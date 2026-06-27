package indicator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/cinar/indicator/v2/momentum"
	"github.com/cinar/indicator/v2/trend"
	"github.com/cinar/indicator/v2/volatility"
	"github.com/cinar/indicator/v2/volume"
	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/constant"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

const (
	consumerName    = "KLINE_INDICATOR"
	configCacheTTL  = 10 * time.Second
	defaultLookback = 300
)

type Service struct {
	js         nats.JetStreamContext
	klineRepo  *repository.MarketKlineRepository
	configRepo *repository.IndicatorConfigRepository
	resultRepo *repository.IndicatorResultRepository
	mu         sync.Mutex
	cache      map[string]cachedConfigs
}

type cachedConfigs struct {
	until   time.Time
	configs []entity.IndicatorConfig
}

func NewService(js nats.JetStreamContext, klineRepo *repository.MarketKlineRepository, configRepo *repository.IndicatorConfigRepository, resultRepo *repository.IndicatorResultRepository) *Service {
	return &Service{js: js, klineRepo: klineRepo, configRepo: configRepo, resultRepo: resultRepo, cache: make(map[string]cachedConfigs)}
}

func (s *Service) Subscribe(ctx context.Context) error {
	if err := ensureStream(ctx, s.js, constant.KlineIndicatorStreamName, constant.KlineIndicatorStreamSubjectAll); err != nil {
		return err
	}
	if err := ensureStream(ctx, s.js, constant.KlineStreamName, constant.KlineStreamSubjectAll); err != nil {
		return err
	}

	_, err := s.js.QueueSubscribe(
		constant.KlineStreamSubjectAll,
		consumerName,
		func(msg *nats.Msg) {
			if err := util.ProcessWithTimeout(config.Env.NatsJetstream.TimeoutHandler["insert_kline"], config.Env.NatsJetstream.MaxRetries, msg, s.handleKline); err != nil {
				logrus.WithError(err).Error("failed to calculate kline indicators")
			}
		},
		nats.ManualAck(),
		nats.Durable(consumerName),
		nats.DeliverNew(),
	)
	return err
}

func ensureStream(ctx context.Context, js nats.JetStreamContext, name string, subject string) error {
	cfg := &nats.StreamConfig{
		Name:      name,
		Subjects:  []string{subject},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		MaxAge:    5 * time.Minute,
		Replicas:  1,
	}

	stream, err := js.StreamInfo(name, nats.Context(ctx))
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
	var event entity.MarketKlineEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		return err
	}
	if err := s.klineRepo.Create(ctx, &event.Data); err != nil {
		return err
	}

	indicators, err := s.calculate(ctx, event.Data)
	if err != nil {
		return err
	}
	if err := s.resultRepo.Upsert(ctx, event.Data.ID, indicators); err != nil {
		return err
	}
	subject := constant.GetKlineIndicatorStreamSubject(event.Data.Exchange, event.Data.Symbol, event.Data.Interval)
	return util.PublishEvent(s.js, subject, entity.MarketKlineIndicatorEvent{
		Data:       event.Data,
		Indicators: indicators,
	})
}

func (s *Service) calculate(ctx context.Context, k entity.MarketKline) (map[string]float64, error) {
	cfgs, err := s.configsForPair(ctx, k)
	if err != nil {
		return nil, err
	}
	lookback := maxLookback(cfgs)
	rows, err := s.klineRepo.ListRecentClosedBefore(ctx, k.Exchange, k.MarketType, k.Symbol, k.Interval, k.CloseTime, lookback)
	if err != nil {
		return nil, err
	}
	if k.IsClosed {
		rows = append(rows, k)
	}
	return calculateLatest(ctx, rows, cfgs), nil
}

func (s *Service) configsForPair(ctx context.Context, k entity.MarketKline) ([]entity.IndicatorConfig, error) {
	key := pairKey(k)
	now := time.Now().UTC()

	s.mu.Lock()
	cached, ok := s.cache[key]
	if ok && now.Before(cached.until) {
		cfgs := cached.configs
		s.mu.Unlock()
		return cfgs, nil
	}
	s.mu.Unlock()

	cfgs, err := s.configRepo.ListForPair(ctx, k.Exchange, k.MarketType, k.Symbol, k.Interval)
	if err != nil {
		return nil, err
	}
	if len(cfgs) == 0 {
		cfgs = defaultConfigs(k)
	}

	s.mu.Lock()
	s.cache[key] = cachedConfigs{until: now.Add(configCacheTTL), configs: cfgs}
	s.mu.Unlock()
	return cfgs, nil
}

func pairKey(k entity.MarketKline) string {
	return strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(k.Exchange)),
		strings.ToLower(strings.TrimSpace(k.MarketType)),
		strings.ToUpper(strings.TrimSpace(k.Symbol)),
		strings.TrimSpace(k.Interval),
	}, "|")
}

func defaultConfigs(k entity.MarketKline) []entity.IndicatorConfig {
	base := []struct {
		indicator string
		name      string
		params    string
	}{
		{"ema", "ema_21", `{"period":21}`},
		{"ema", "ema_50", `{"period":50}`},
		{"ema", "ema_55", `{"period":55}`},
		{"ema", "ema_200", `{"period":200}`},
		{"vwap", "vwap_100", `{"period":100}`},
		{"vwap", "vwap_120", `{"period":120}`},
		{"vwap", "vwap_200", `{"period":200}`},
		{"macd", "macd_12_26_9", `{"fast":12,"slow":26,"signal":9}`},
		{"atr", "atr_10", `{"period":10}`},
		{"atr", "atr_14", `{"period":14}`},
		{"rsi", "rsi_14", `{"period":14}`},
		{"bollinger_bands", "bb_20_2", `{"period":20}`},
		{"stochastic", "stoch_14_3", `{"period":14,"signal":3}`},
		{"volume_mean", "volume_mean_20", `{"period":20}`},
	}
	cfgs := make([]entity.IndicatorConfig, 0, len(base))
	for _, item := range base {
		cfgs = append(cfgs, entity.IndicatorConfig{
			Exchange:   k.Exchange,
			MarketType: k.MarketType,
			Symbol:     k.Symbol,
			Interval:   k.Interval,
			Indicator:  item.indicator,
			OutputName: item.name,
			Params:     item.params,
			Enabled:    true,
		})
	}
	return cfgs
}

func calculateLatest(ctx context.Context, rows []entity.MarketKline, cfgs []entity.IndicatorConfig) map[string]float64 {
	out := map[string]float64{}
	if len(rows) == 0 {
		return out
	}
	series := newSeries(rows)
	for _, cfg := range cfgs {
		if !cfg.Enabled {
			continue
		}
		name := strings.TrimSpace(cfg.OutputName)
		if name == "" {
			continue
		}
		params := parseParams(cfg.Params)
		switch strings.ToLower(strings.TrimSpace(cfg.Indicator)) {
		case "ema":
			setLatest(out, name, trend.NewEmaWithPeriod[float64](positiveParam(params, "period", 20)).ComputeWithContext(ctx, floats(series.close)))
		case "vwap":
			setLatest(out, name, volume.NewVwapWithPeriod[float64](positiveParam(params, "period", 14)).ComputeWithContext(ctx, floats(series.close), floats(series.volume)))
		case "macd":
			macd, signal := trend.NewMacdWithPeriod[float64](
				positiveParam(params, "fast", 12),
				positiveParam(params, "slow", 26),
				positiveParam(params, "signal", 9),
			).ComputeWithContext(ctx, floats(series.close))
			line, okLine, sig, okSignal := latest2(macd, signal)
			if okLine && okSignal {
				out[name] = line
				out[name+"_signal"] = sig
				out[name+"_hist"] = line - sig
			}
		case "atr":
			if v, ok := latest(volatility.NewAtrWithPeriod[float64](positiveParam(params, "period", 14)).ComputeWithContext(ctx, floats(series.high), floats(series.low), floats(series.close))); ok {
				out[name] = v
				closePx := series.close[len(series.close)-1]
				if closePx > 0 {
					out[name+"_pct"] = v / closePx * 100
				}
			}
		case "rsi":
			setLatest(out, name, momentum.NewRsiWithPeriod[float64](positiveParam(params, "period", 14)).ComputeWithContext(ctx, floats(series.close)))
		case "bollinger_bands":
			upper, mid, lower := volatility.NewBollingerBandsWithPeriod[float64](positiveParam(params, "period", 20)).ComputeWithContext(ctx, floats(series.close))
			up, okUp, m, okMid, lo, okLow := latest3(upper, mid, lower)
			if okUp && okMid && okLow {
				out[name+"_upper"] = up
				out[name+"_mid"] = m
				out[name+"_lower"] = lo
			}
		case "stochastic":
			so := momentum.NewStochasticOscillator[float64]()
			period := positiveParam(params, "period", 14)
			signal := positiveParam(params, "signal", 3)
			so.Max = trend.NewMovingMaxWithPeriod[float64](period)
			so.Min = trend.NewMovingMinWithPeriod[float64](period)
			so.Sma = trend.NewSmaWithPeriod[float64](signal)
			k, d := so.ComputeWithContext(ctx, floats(series.high), floats(series.low), floats(series.close))
			kv, okK, dv, okD := latest2(k, d)
			if okK && okD {
				out[name+"_k"] = kv
				out[name+"_d"] = dv
			}
		case "volume_mean":
			setLatest(out, name, trend.NewSmaWithPeriod[float64](positiveParam(params, "period", 20)).ComputeWithContext(ctx, floats(series.volume)))
		}
	}
	return out
}

type priceSeries struct {
	close  []float64
	high   []float64
	low    []float64
	volume []float64
}

func newSeries(rows []entity.MarketKline) priceSeries {
	s := priceSeries{
		close:  make([]float64, 0, len(rows)),
		high:   make([]float64, 0, len(rows)),
		low:    make([]float64, 0, len(rows)),
		volume: make([]float64, 0, len(rows)),
	}
	for _, row := range rows {
		closePx, _ := row.ClosePrice.Float64()
		highPx, _ := row.HighPrice.Float64()
		lowPx, _ := row.LowPrice.Float64()
		volumePx, _ := row.QuoteVolume.Float64()
		s.close = append(s.close, closePx)
		s.high = append(s.high, highPx)
		s.low = append(s.low, lowPx)
		s.volume = append(s.volume, volumePx)
	}
	return s
}

func floats(values []float64) <-chan float64 {
	ch := make(chan float64, len(values))
	for _, value := range values {
		ch <- value
	}
	close(ch)
	return ch
}

func latest(ch <-chan float64) (float64, bool) {
	var value float64
	ok := false
	for item := range ch {
		if !math.IsNaN(item) && !math.IsInf(item, 0) {
			value = item
			ok = true
		}
	}
	return value, ok
}

func latest2(a, b <-chan float64) (float64, bool, float64, bool) {
	type result struct {
		index int
		value float64
		ok    bool
	}
	done := make(chan result, 2)
	go func() {
		v, ok := latest(a)
		done <- result{index: 0, value: v, ok: ok}
	}()
	go func() {
		v, ok := latest(b)
		done <- result{index: 1, value: v, ok: ok}
	}()
	var av, bv float64
	var aok, bok bool
	for i := 0; i < 2; i++ {
		res := <-done
		if res.index == 0 {
			av, aok = res.value, res.ok
		} else {
			bv, bok = res.value, res.ok
		}
	}
	return av, aok, bv, bok
}

func latest3(a, b, c <-chan float64) (float64, bool, float64, bool, float64, bool) {
	type result struct {
		index int
		value float64
		ok    bool
	}
	done := make(chan result, 3)
	for index, ch := range []<-chan float64{a, b, c} {
		go func(index int, ch <-chan float64) {
			v, ok := latest(ch)
			done <- result{index: index, value: v, ok: ok}
		}(index, ch)
	}
	var values [3]float64
	var oks [3]bool
	for i := 0; i < 3; i++ {
		res := <-done
		values[res.index], oks[res.index] = res.value, res.ok
	}
	return values[0], oks[0], values[1], oks[1], values[2], oks[2]
}

func setLatest(out map[string]float64, name string, ch <-chan float64) {
	if v, ok := latest(ch); ok {
		out[name] = v
	}
}

func parseParams(raw string) map[string]any {
	params := map[string]any{}
	_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &params)
	return params
}

func positiveParam(params map[string]any, key string, fallback int) int {
	value, ok := params[key]
	if !ok {
		return fallback
	}
	var parsed int
	switch v := value.(type) {
	case float64:
		parsed = int(v)
	case int:
		parsed = v
	default:
		_, _ = fmt.Sscan(strings.TrimSpace(fmt.Sprint(v)), &parsed)
	}
	if parsed <= 0 {
		return fallback
	}
	return parsed
}

func maxLookback(cfgs []entity.IndicatorConfig) int {
	maximum := defaultLookback
	for _, cfg := range cfgs {
		params := parseParams(cfg.Params)
		switch strings.ToLower(strings.TrimSpace(cfg.Indicator)) {
		case "macd":
			needed := positiveParam(params, "slow", 26) + positiveParam(params, "signal", 9)
			if needed > maximum {
				maximum = needed
			}
		case "stochastic":
			needed := positiveParam(params, "period", 14) + positiveParam(params, "signal", 3)
			if needed > maximum {
				maximum = needed
			}
		default:
			period := positiveParam(params, "period", 0)
			if period > maximum {
				maximum = period
			}
		}
	}
	return maximum
}
