package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"strings"
	"sync/atomic"
	"time"

	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/constant"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/nats-io/nats.go"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

const consumerName = "GO_STRATEGY"
const strategyCacheSyncInterval = 30 * time.Second
const strategyWorkerCount = 8
const strategyWorkerQueueSize = 64

type Service struct {
	js                 nats.JetStreamContext
	strategyConfigRepo *repository.StrategyConfigRepository
	ruleRepo           *repository.StrategyRuleRepository
	stateRepo          *repository.StrategyStateRepository
	cache              atomic.Pointer[strategyCacheSnapshot]
}

type cachedStrategySet struct {
	rules    []entity.StrategyRule
	configBy map[string]entity.StrategyConfig
}

type strategyCacheSnapshot map[string]cachedStrategySet

type ruleConditions struct {
	All []condition `json:"all"`
	Any []condition `json:"any"`
}

type condition struct {
	Left  string  `json:"left"`
	Op    string  `json:"op"`
	Right string  `json:"right"`
	Value float64 `json:"value"`
}

func NewService(js nats.JetStreamContext, strategyConfigRepo *repository.StrategyConfigRepository, ruleRepo *repository.StrategyRuleRepository, stateRepo *repository.StrategyStateRepository) *Service {
	service := &Service{js: js, strategyConfigRepo: strategyConfigRepo, ruleRepo: ruleRepo, stateRepo: stateRepo}
	snapshot := strategyCacheSnapshot{}
	service.cache.Store(&snapshot)
	return service
}

func (s *Service) StartPeriodicSync(ctx context.Context) {
	ticker := time.NewTicker(strategyCacheSyncInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.refreshStrategyCache(ctx)
			}
		}
	}()
}

func (s *Service) Subscribe(ctx context.Context) error {
	if err := ensureStream(ctx, s.js); err != nil {
		return err
	}
	workers := s.startWorkers(ctx)
	_, err := s.js.QueueSubscribe(
		constant.KlineIndicatorStreamSubjectAll,
		consumerName,
		func(msg *nats.Msg) {
			queue := workers[workerIndex(msg.Subject, len(workers))]
			select {
			case queue <- msg:
			case <-ctx.Done():
				_ = msg.Nak()
			default:
				if err := msg.NakWithDelay(time.Second); err != nil {
					logrus.WithError(err).Warn("failed to requeue strategy message")
				}
			}
		},
		nats.ManualAck(),
		nats.Durable(consumerName),
		nats.DeliverNew(),
		nats.MaxAckPending(strategyWorkerCount*strategyWorkerQueueSize),
	)
	return err
}

func (s *Service) startWorkers(ctx context.Context) []chan *nats.Msg {
	workers := make([]chan *nats.Msg, strategyWorkerCount)
	for i := range workers {
		queue := make(chan *nats.Msg, strategyWorkerQueueSize)
		workers[i] = queue
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case msg := <-queue:
					if err := util.ProcessWithTimeout(config.Env.NatsJetstream.TimeoutHandler["insert_kline"], config.Env.NatsJetstream.MaxRetries, msg, s.handleKline); err != nil {
						logrus.WithError(err).Error("failed to process strategy rule")
					}
				}
			}
		}()
	}
	return workers
}

func workerIndex(subject string, workerCount int) int {
	if workerCount <= 1 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(subject))
	return int(h.Sum32() % uint32(workerCount))
}

func ensureStream(ctx context.Context, js nats.JetStreamContext) error {
	cfg := &nats.StreamConfig{
		Name:      constant.KlineIndicatorStreamName,
		Subjects:  []string{constant.KlineIndicatorStreamSubjectAll},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		MaxAge:    5 * time.Minute,
		Replicas:  1,
	}
	stream, err := js.StreamInfo(constant.KlineIndicatorStreamName, nats.Context(ctx))
	if err != nil && !errors.Is(err, nats.ErrStreamNotFound) {
		return err
	}
	if stream == nil {
		_, err = js.AddStream(cfg, nats.Context(ctx))
		return err
	}
	_, err = js.UpdateStream(cfg, nats.Context(ctx))
	return err
}

func (s *Service) handleKline(ctx context.Context, msg *nats.Msg) error {
	var event entity.MarketKlineIndicatorEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		return err
	}
	if !event.Data.IsClosed {
		return nil
	}

	strategies, err := s.strategySetForPair(ctx, event.Data.Exchange, event.Data.MarketType, event.Data.Symbol, event.Data.Interval)
	if err != nil {
		return err
	}

	values := valuesForEvent(event)
	prev, err := s.previousValues(ctx, event.Data)
	if err != nil {
		return err
	}
	for _, rule := range strategies.rules {
		cfg := strategies.configBy[rule.StrategyConfigID]
		if !ruleMatches(rule, values, prev) {
			continue
		}
		order, ok := buildOrder(cfg, rule, event)
		if !ok {
			continue
		}
		if err := util.PublishEvent(s.js, constant.OrderEngineStreamSubjectPlaceOrder, entity.OrderRequestEvent{Data: order}); err != nil {
			return err
		}
	}
	return s.setPreviousValues(ctx, event.Data, values)
}

func (s *Service) strategySetForPair(ctx context.Context, exchange, marketType, symbol, interval string) (cachedStrategySet, error) {
	key := strategyPairKey(exchange, marketType, symbol, interval)
	cached, ok := s.cacheSnapshot()[key]
	if ok {
		return cached, nil
	}
	return s.syncStrategySet(ctx, key, exchange, marketType, symbol, interval)
}

func (s *Service) syncStrategySet(ctx context.Context, key, exchange, marketType, symbol, interval string) (cachedStrategySet, error) {
	configs, err := s.strategyConfigRepo.ListEnabledByPair(ctx, exchange, marketType, symbol, interval)
	if err != nil {
		return cachedStrategySet{}, err
	}
	ids := make([]string, 0, len(configs))
	configByID := make(map[string]entity.StrategyConfig, len(configs))
	for _, cfg := range configs {
		ids = append(ids, cfg.ID)
		configByID[cfg.ID] = cfg
	}
	rules, err := s.ruleRepo.ListEnabledByStrategyConfigIDs(ctx, ids)
	if err != nil {
		return cachedStrategySet{}, err
	}
	cached := cachedStrategySet{rules: rules, configBy: configByID}
	s.storeStrategySet(key, cached)
	return cached, nil
}

func (s *Service) refreshStrategyCache(ctx context.Context) {
	snapshot := s.cacheSnapshot()
	keys := make([]string, 0, len(snapshot))
	for key := range snapshot {
		keys = append(keys, key)
	}
	for _, key := range keys {
		exchange, marketType, symbol, interval, ok := splitStrategyPairKey(key)
		if !ok {
			continue
		}
		if _, err := s.syncStrategySet(ctx, key, exchange, marketType, symbol, interval); err != nil {
			logrus.WithError(err).WithField("pair", key).Warn("failed to sync strategy cache")
		}
	}
}

func (s *Service) cacheSnapshot() strategyCacheSnapshot {
	snapshot := s.cache.Load()
	if snapshot == nil {
		return strategyCacheSnapshot{}
	}
	return *snapshot
}

func (s *Service) storeStrategySet(key string, cached cachedStrategySet) {
	for {
		currentPtr := s.cache.Load()
		current := strategyCacheSnapshot{}
		if currentPtr != nil {
			current = *currentPtr
		}
		next := make(strategyCacheSnapshot, len(current)+1)
		for existingKey, existingValue := range current {
			next[existingKey] = existingValue
		}
		next[key] = cached
		if s.cache.CompareAndSwap(currentPtr, &next) {
			return
		}
	}
}

func strategyPairKey(exchange, marketType, symbol, interval string) string {
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(exchange)),
		strings.ToLower(strings.TrimSpace(marketType)),
		strings.TrimSpace(symbol),
		strings.TrimSpace(interval),
	}, "|")
}

func splitStrategyPairKey(key string) (exchange, marketType, symbol, interval string, ok bool) {
	parts := strings.Split(key, "|")
	if len(parts) != 4 {
		return "", "", "", "", false
	}
	return parts[0], parts[1], parts[2], parts[3], true
}

func valuesForEvent(event entity.MarketKlineIndicatorEvent) map[string]float64 {
	values := make(map[string]float64, len(event.Indicators)+4)
	values["close"] = decimalFloat(event.Data.ClosePrice)
	values["high"] = decimalFloat(event.Data.HighPrice)
	values["low"] = decimalFloat(event.Data.LowPrice)
	values["volume"] = decimalFloat(event.Data.QuoteVolume)
	for k, v := range event.Indicators {
		values["indicators."+k] = v
	}
	return values
}

func decimalFloat(value decimal.Decimal) float64 {
	f, _ := value.Float64()
	return f
}

func (s *Service) previousValues(ctx context.Context, k entity.MarketKline) (map[string]float64, error) {
	raw, err := s.stateRepo.FindValues(ctx, k.Exchange, k.MarketType, k.Symbol, k.Interval)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil, err
	}
	values := map[string]float64{}
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func (s *Service) setPreviousValues(ctx context.Context, k entity.MarketKline, values map[string]float64) error {
	payload, err := json.Marshal(values)
	if err != nil {
		return err
	}
	return s.stateRepo.UpsertValues(ctx, k.Exchange, k.MarketType, k.Symbol, k.Interval, string(payload))
}

func ruleMatches(rule entity.StrategyRule, values, prev map[string]float64) bool {
	var cfg ruleConditions
	if err := json.Unmarshal([]byte(rule.Conditions), &cfg); err != nil {
		return false
	}
	for _, c := range cfg.All {
		if !conditionMatches(c, values, prev) {
			return false
		}
	}
	if len(cfg.Any) == 0 {
		return true
	}
	for _, c := range cfg.Any {
		if conditionMatches(c, values, prev) {
			return true
		}
	}
	return false
}

func conditionMatches(c condition, values, prev map[string]float64) bool {
	left, ok := values[c.Left]
	if !ok {
		return false
	}
	right := c.Value
	if strings.TrimSpace(c.Right) != "" {
		var ok bool
		right, ok = values[c.Right]
		if !ok {
			return false
		}
	}
	switch strings.ToLower(strings.TrimSpace(c.Op)) {
	case "gt":
		return left > right
	case "gte":
		return left >= right
	case "lt":
		return left < right
	case "lte":
		return left <= right
	case "eq":
		return left == right
	case "neq":
		return left != right
	case "cross_above":
		if prev == nil {
			return false
		}
		prevLeft, okLeft := prev[c.Left]
		prevRight, okRight := prev[c.Right]
		return okLeft && okRight && prevLeft <= prevRight && left > right
	case "cross_below":
		if prev == nil {
			return false
		}
		prevLeft, okLeft := prev[c.Left]
		prevRight, okRight := prev[c.Right]
		return okLeft && okRight && prevLeft >= prevRight && left < right
	default:
		return false
	}
}

func buildOrder(cfg entity.StrategyConfig, rule entity.StrategyRule, event entity.MarketKlineIndicatorEvent) (entity.OrderRequest, bool) {
	marketType := entity.NormalizeMarketType(cfg.MarketType)
	side := entity.NormalizeOrderSideByMarket(rule.Side, marketType)
	if side == "" || strings.TrimSpace(cfg.ID) == "" || strings.TrimSpace(cfg.UserID.String) == "" || !cfg.OrderQty.IsPositive() {
		return entity.OrderRequest{}, false
	}
	requestToken, err := apiutil.NewRandomToken()
	if err != nil {
		return entity.OrderRequest{}, false
	}
	requestID := "go-strategy-" + requestToken
	strategyID := cfg.Strategy
	internal, _ := json.Marshal(map[string]string{"strategy_rule_id": rule.ID, "strategy_config_id": cfg.ID})
	price := event.Data.ClosePrice
	if cfg.OrderType == string(entity.OrderTypeMarket) {
		price = decimal.Zero
	}
	return entity.OrderRequest{
		RequestID:        requestID,
		UserID:           cfg.UserID.String,
		EntryOrderID:     entryOrderID(requestID, rule.TradeCondition),
		Exchange:         cfg.Exchange,
		MarketType:       cfg.MarketType,
		PositionSide:     cfg.PositionSide,
		Symbol:           cfg.Symbol,
		Type:             entity.OrderType(cfg.OrderType),
		Side:             side,
		Price:            price,
		Quantity:         cfg.OrderQty,
		RequestedAt:      time.Now().UTC().UnixMilli(),
		Source:           "go-strategy",
		StrategyID:       &strategyID,
		StrategyName:     cfg.Strategy,
		Interval:         cfg.Interval,
		Internal:         string(internal),
		TradeCondition:   rule.TradeCondition,
		OrderReason:      rule.OrderReason,
		ExitType:         rule.ExitType,
		NeedNotification: cfg.NeedNotification,
		IsPaperTrading:   cfg.IsPaperTrading,
	}, true
}

func entryOrderID(requestID string, tradeCondition string) string {
	if entity.NormalizeTradeCondition(tradeCondition) == entity.TradeConditionEntry {
		return requestID
	}
	return ""
}
