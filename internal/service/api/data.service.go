package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/krobus00/hft-service/internal/constant"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/nats-io/nats.go"
	"github.com/shopspring/decimal"
)

type DataService struct {
	authRepo              *repository.APIAuthRepository
	settingRepo           *repository.APISettingRepository
	orderHistoryRepo      *repository.OrderHistoryRepository
	priceReferenceRepo    *repository.PriceReferenceRepository
	marketKlineRepo       *repository.MarketKlineRepository
	symbolMappingRepo     *repository.SymbolMappingRepository
	klineSubscriptionRepo *repository.KlineSubscriptionRepository
	indicatorConfigRepo   *repository.IndicatorConfigRepository
	strategyConfigRepo    *repository.StrategyConfigRepository
	strategyRuleRepo      *repository.StrategyRuleRepository
	js                    nats.JetStreamContext
}

type AuthConfigResponse struct {
	ShowSetupLink bool `json:"show_setup_link"`
}

type FormEnumsResponse map[string][]string

type StrategyMonitorResponse struct {
	Name    string         `json:"name"`
	URL     string         `json:"url"`
	Online  bool           `json:"online"`
	Error   string         `json:"error,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

type strategyMonitorConfig struct {
	Name string
	URL  string
}

var (
	ErrInvalidManualOrder = errors.New("invalid manual order")
	ErrPositionNotRunning = errors.New("position is not running")
)

type ManualOrderResult struct {
	RequestID  string `json:"request_id"`
	StrategyID string `json:"strategy_id"`
	Status     string `json:"status"`
}

func NewDataService(apiDB, marketDB, orderDB *sqlx.DB, js nats.JetStreamContext) *DataService {
	return &DataService{
		authRepo:              repository.NewAPIAuthRepository(apiDB),
		settingRepo:           repository.NewAPISettingRepository(apiDB),
		orderHistoryRepo:      repository.NewOrderHistoryRepository(orderDB),
		priceReferenceRepo:    repository.NewPriceReferenceRepository(orderDB),
		marketKlineRepo:       repository.NewMarketKlineRepository(marketDB),
		symbolMappingRepo:     repository.NewSymbolMappingRepository(marketDB),
		klineSubscriptionRepo: repository.NewKlineSubscriptionRepository(marketDB),
		indicatorConfigRepo:   repository.NewIndicatorConfigRepository(marketDB),
		strategyConfigRepo:    repository.NewStrategyConfigRepository(marketDB),
		strategyRuleRepo:      repository.NewStrategyRuleRepository(marketDB),
		js:                    js,
	}
}

func (s *DataService) GetAuthConfig(ctx context.Context) (*AuthConfigResponse, error) {
	setting, err := s.settingRepo.FindByKey(ctx, "dashboard.show_setup_link")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &AuthConfigResponse{ShowSetupLink: true}, nil
		}
		return nil, err
	}
	return &AuthConfigResponse{ShowSetupLink: settingBoolValue(setting["value"], true)}, nil
}

func (s *DataService) GetDashboardOverview(ctx context.Context) (*DashboardOverviewResponse, error) {
	now := time.Now().UTC()
	summary, recent, err := s.orderHistoryRepo.GetDashboardOverview(ctx, now.Add(-24*time.Hour), 20)
	if err != nil {
		return nil, err
	}
	enabled, total, err := s.strategyConfigRepo.CountEnabled(ctx)
	if err != nil {
		return nil, err
	}
	result := &DashboardOverviewResponse{
		GeneratedAt:       now,
		Orders24h:         summary.Orders24h,
		ProblemOrders24h:  summary.ProblemOrders24h,
		ClosedTrades:      summary.ClosedTrades,
		WinningTrades:     summary.WinningTrades,
		RunningTrades:     summary.RunningTrades,
		RealizedPnL:       summary.RealizedPnL,
		RunningPnL:        summary.RunningPnL,
		EnabledStrategies: enabled,
		TotalStrategies:   total,
		LastPriceAt:       nullTimePtr(summary.LastPriceAt),
		RecentOrders:      make([]OrderHistoryResponse, 0, len(recent)),
	}
	for i := range recent {
		result.RecentOrders = append(result.RecentOrders, *orderHistoryWithMetricsResponse(&recent[i]))
	}
	return result, nil
}

func (s *DataService) GetFormEnums(ctx context.Context) (FormEnumsResponse, error) {
	exchanges, err := s.listExchangeEnums(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := s.authRepo.ListRoleNames(ctx)
	if err != nil {
		return nil, err
	}
	permissions, err := s.authRepo.ListPermissionNames(ctx)
	if err != nil {
		return nil, err
	}

	return FormEnumsResponse{
		"exchange":         exchanges,
		"role":             roles,
		"permission":       permissions,
		"dashboard_page":   {"orders", "orderPnL", "dailyReports", "strategyPerformance", "strategyConfigs", "strategyRules", "strategyMonitors", "marketKlines", "priceReferences", "marketBackfills", "symbolMappings", "klineSubscriptions", "indicatorConfigs", "users", "roles", "permissions", "settings", "dashboardPages"},
		"market_type":      {"spot", "futures"},
		"position_side":    {"BOTH", "LONG", "SHORT"},
		"order_side":       {"BUY", "SELL", "LONG", "SHORT"},
		"order_type":       {"MARKET", "LIMIT"},
		"order_status":     {"NEW", "PARTIALLY_FILLED", "FILLED", "CANCELED", "REJECTED", "EXPIRED"},
		"order_state":      {"running", "closed"},
		"trade_condition":  {"ENTRY", "EXIT", "TAKE_PROFIT", "STOP_LOSS", "TRAILING_STOP", "SIGNAL", "UNKNOWN"},
		"exit_type":        {"", "TAKE_PROFIT", "STOP_LOSS", "TRAILING_STOP"},
		"interval":         {"1m", "3m", "5m", "15m", "30m", "1h", "4h", "1d"},
		"kline_event_type": {"kline"},
		"indicator":        {"ema", "vwap", "macd", "atr", "rsi", "bollinger_bands", "stochastic", "volume_mean"},
		"condition_op":     {"gt", "gte", "lt", "lte", "eq", "neq", "cross_above", "cross_below"},
		"source":           {"dashboard", "api", "strategy"},
		"boolean":          {"true", "false"},
	}, nil
}

func (s *DataService) ListStrategyMonitors(ctx context.Context) ([]StrategyMonitorResponse, error) {
	monitors, err := s.configuredStrategyMonitors(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]StrategyMonitorResponse, 0, len(monitors))
	for _, monitor := range monitors {
		result = append(result, s.fetchStrategyMonitor(ctx, monitor, http.MethodGet, ""))
	}
	return result, nil
}

func (s *DataService) StrategyMonitorAction(ctx context.Context, name, action string) (*StrategyMonitorResponse, error) {
	if action != "reset" && action != "restart" {
		return nil, fmt.Errorf("%w: invalid strategy monitor action", ErrInvalidManualOrder)
	}
	monitors, err := s.configuredStrategyMonitors(ctx)
	if err != nil {
		return nil, err
	}
	for _, monitor := range monitors {
		if monitor.Name == strings.TrimSpace(name) {
			resp := s.fetchStrategyMonitor(ctx, monitor, http.MethodPost, action)
			return &resp, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (s *DataService) fetchStrategyMonitor(ctx context.Context, monitor strategyMonitorConfig, method, action string) StrategyMonitorResponse {
	resp := StrategyMonitorResponse{Name: monitor.Name, URL: monitor.URL}
	target := strings.TrimRight(monitor.URL, "/")
	if action != "" {
		target += "/" + action
	}
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	client := &http.Client{Timeout: 5 * time.Second}
	httpResp, err := client.Do(req)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	defer httpResp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	if err := json.Unmarshal(body, &resp.Payload); err != nil {
		resp.Error = err.Error()
		return resp
	}
	resp.Online = httpResp.StatusCode >= 200 && httpResp.StatusCode < 300
	if !resp.Online && resp.Error == "" {
		resp.Error = httpResp.Status
	}
	return resp
}

func (s *DataService) configuredStrategyMonitors(ctx context.Context) ([]strategyMonitorConfig, error) {
	configs, err := s.strategyConfigRepo.ListMonitors(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]strategyMonitorConfig, 0, len(configs))
	for _, cfg := range configs {
		result = append(result, strategyMonitorConfig{Name: cfg.Strategy, URL: strings.TrimSpace(cfg.MonitorURL)})
	}
	return result, nil
}

func (s *DataService) listExchangeEnums(ctx context.Context) ([]string, error) {
	sources := []func(context.Context) ([]string, error){
		s.orderHistoryRepo.ListExchanges,
		s.marketKlineRepo.ListExchanges,
		s.symbolMappingRepo.ListExchanges,
		s.strategyConfigRepo.ListExchanges,
	}
	groups := make([][]string, 0, len(sources))
	for _, source := range sources {
		items, err := source(ctx)
		if err != nil {
			return nil, err
		}
		groups = append(groups, items)
	}
	return repository.NormalizeExchangeEnums(groups...), nil
}

func (s *DataService) ListOrders(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	resp, err := s.orderHistoryRepo.GetPaginationWithMetrics(ctx, req)
	if err != nil {
		return nil, err
	}
	return orderHistoryPaginationResponse(resp), nil
}

func (s *DataService) GetOrder(ctx context.Context, id string) (*OrderHistoryResponse, error) {
	item, err := s.orderHistoryRepo.FindByIDWithMetrics(ctx, id)
	result, err := ensureFound(item, err)
	if err != nil {
		return nil, err
	}
	return orderHistoryWithMetricsResponse(result), nil
}

func (s *DataService) CreateManualOrder(ctx context.Context, userID string, values map[string]any) (*ManualOrderResult, error) {
	if s.js == nil {
		return nil, errors.New("order publisher is not configured")
	}
	requestID, err := apiutil.NewRandomToken()
	if err != nil {
		return nil, err
	}
	order, err := buildManualEntryOrder(userID, "manual-"+requestID, values)
	if err != nil {
		return nil, err
	}
	if err := util.PublishEvent(s.js, constant.OrderEngineStreamSubjectPlaceOrder, entity.OrderRequestEvent{Data: order}); err != nil {
		return nil, err
	}
	return &ManualOrderResult{RequestID: order.RequestID, StrategyID: "manual", Status: "queued"}, nil
}

func (s *DataService) CloseOrder(ctx context.Context, id string) (*ManualOrderResult, error) {
	if s.js == nil {
		return nil, errors.New("order publisher is not configured")
	}
	entry, err := s.orderHistoryRepo.FindByIDWithMetrics(ctx, id)
	if err != nil {
		return nil, err
	}
	if entry == nil || entry.State != "running" || entry.TradeCondition != string(entity.TradeConditionEntry) {
		return nil, ErrPositionNotRunning
	}
	order, err := buildManualCloseOrder(entry.OrderHistory)
	if err != nil {
		return nil, err
	}
	if order.IsPaperTrading {
		price, err := s.priceReferenceRepo.FindByMarket(ctx, order.Exchange, order.MarketType, order.Symbol)
		if err != nil {
			return nil, err
		}
		if price == nil || !price.Price.IsPositive() {
			return nil, fmt.Errorf("%w: current paper-trading price is unavailable", ErrInvalidManualOrder)
		}
		order.Price = price.Price
	}
	if err := util.PublishEvent(s.js, constant.OrderEngineStreamSubjectPlaceOrder, entity.OrderRequestEvent{Data: order}); err != nil {
		return nil, err
	}
	strategyID := ""
	if order.StrategyID != nil {
		strategyID = *order.StrategyID
	}
	return &ManualOrderResult{RequestID: order.RequestID, StrategyID: strategyID, Status: "queued"}, nil
}

func buildManualEntryOrder(userID, requestID string, values map[string]any) (entity.OrderRequest, error) {
	marketType := entity.NormalizeMarketType(stringValue(values, "market_type", "spot"))
	side := entity.NormalizeOrderSideByMarket(stringValue(values, "side", ""), marketType)
	quantity := decimalValue(values, "quantity", decimal.Zero)
	price := decimalValue(values, "price", decimal.Zero)
	exchange := strings.ToLower(stringValue(values, "exchange", ""))
	symbol := strings.ToUpper(stringValue(values, "symbol", ""))
	isPaperTrading := boolValue(values, "is_paper_trading", false)
	if strings.TrimSpace(userID) == "" || exchange == "" || symbol == "" || side == "" || !quantity.IsPositive() || (isPaperTrading && !price.IsPositive()) {
		return entity.OrderRequest{}, ErrInvalidManualOrder
	}
	strategyID := "manual"
	return entity.OrderRequest{
		RequestID: requestID, UserID: userID, EntryOrderID: requestID,
		Exchange: exchange, MarketType: string(marketType), PositionSide: string(entity.NormalizePositionSide(stringValue(values, "position_side", "BOTH"))),
		Symbol: symbol, Type: entity.OrderTypeMarket, Side: side, Price: price, Quantity: quantity,
		RequestedAt: time.Now().UTC().UnixMilli(), Source: "dashboard", StrategyID: &strategyID, StrategyName: strategyID,
		TradeCondition: string(entity.TradeConditionEntry), OrderReason: "manual_entry",
		NeedNotification: boolValue(values, "need_notification", true), IsPaperTrading: isPaperTrading,
	}, nil
}

func buildManualCloseOrder(entry entity.OrderHistory) (entity.OrderRequest, error) {
	quantity := entry.FilledQuantity
	if quantity.IsZero() {
		quantity = entry.Quantity
	}
	side, ok := closeOrderSide(entry.Side, entity.NormalizeMarketType(entry.MarketType))
	if !ok || !quantity.IsPositive() {
		return entity.OrderRequest{}, ErrInvalidManualOrder
	}
	entryOrderID := strings.TrimSpace(entry.EntryOrderID)
	if entryOrderID == "" {
		return entity.OrderRequest{}, ErrInvalidManualOrder
	}
	strategyID := strings.TrimSpace(entry.StrategyID.String)
	return entity.OrderRequest{
		RequestID: "manual-close-" + entry.ID, UserID: entry.UserID, EntryOrderID: entryOrderID,
		Exchange: entry.Exchange, MarketType: entry.MarketType, PositionSide: entry.PositionSide, Symbol: entry.Symbol,
		Type: entity.OrderTypeMarket, Side: side, Price: decimal.Zero, Quantity: quantity,
		RequestedAt: time.Now().UTC().UnixMilli(), Source: "dashboard", StrategyID: &strategyID, StrategyName: strategyID,
		TradeCondition: string(entity.TradeConditionExit), OrderReason: "manual_close", IsPaperTrading: entry.IsPaperTrading,
	}, nil
}

func (s *DataService) ListPriceReferences(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.priceReferenceRepo.GetPagination(ctx, req)
}

func (s *DataService) GetPriceReference(ctx context.Context, id string) (*entity.PriceReference, error) {
	item, err := s.priceReferenceRepo.FindByID(ctx, id)
	return ensureFound(item, err)
}

func (s *DataService) ListOrderTradePnL(ctx context.Context, filter entity.OrderReportFilter) (*apiutil.PaginationResp, error) {
	return s.orderHistoryRepo.ListTradePnL(ctx, filter)
}

func (s *DataService) ListDailyOrderReports(ctx context.Context, filter entity.OrderReportFilter) (*apiutil.PaginationResp, error) {
	return s.orderHistoryRepo.ListDailyReport(ctx, filter)
}

func (s *DataService) ListStrategyPerformanceReports(ctx context.Context, filter entity.OrderReportFilter) ([]entity.StrategyPerformanceReport, error) {
	return s.orderHistoryRepo.ListStrategyPerformance(ctx, filter)
}

func (s *DataService) ListMarketKlines(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.marketKlineRepo.GetPagination(ctx, req)
}

func (s *DataService) GetMarketKline(ctx context.Context, id string) (*entity.MarketKline, error) {
	item, err := s.marketKlineRepo.FindByID(ctx, id)
	return ensureFound(item, err)
}

func (s *DataService) ListSymbolMappings(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.symbolMappingRepo.GetPagination(ctx, req)
}

func (s *DataService) GetSymbolMapping(ctx context.Context, id string) (*entity.SymbolMapping, error) {
	item, err := s.symbolMappingRepo.FindByID(ctx, id)
	return ensureFound(item, err)
}

func (s *DataService) CreateSymbolMapping(ctx context.Context, values map[string]any) (*entity.SymbolMapping, error) {
	item := mapSymbolMapping(values, nil)
	if err := s.symbolMappingRepo.Create(ctx, item); err != nil {
		return nil, err
	}
	return s.GetSymbolMapping(ctx, item.ID)
}

func (s *DataService) UpdateSymbolMapping(ctx context.Context, id string, values map[string]any) (*entity.SymbolMapping, error) {
	current, err := s.symbolMappingRepo.FindByID(ctx, id)
	if current == nil || err != nil {
		return ensureFound(current, err)
	}
	item := mapSymbolMapping(values, current)
	item.ID = id
	if err := s.symbolMappingRepo.Update(ctx, item); err != nil {
		return nil, err
	}
	return s.GetSymbolMapping(ctx, id)
}

func (s *DataService) DeleteSymbolMapping(ctx context.Context, id string) error {
	return s.symbolMappingRepo.Delete(ctx, id)
}

func (s *DataService) ListKlineSubscriptions(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.klineSubscriptionRepo.GetPagination(ctx, req)
}

func (s *DataService) GetKlineSubscription(ctx context.Context, id string) (*entity.KlineSubscription, error) {
	item, err := s.klineSubscriptionRepo.FindByID(ctx, id)
	return ensureFound(item, err)
}

func (s *DataService) CreateKlineSubscription(ctx context.Context, values map[string]any) (*entity.KlineSubscription, error) {
	item := mapKlineSubscription(values, nil)
	if err := s.klineSubscriptionRepo.Create(ctx, item); err != nil {
		return nil, err
	}
	return s.GetKlineSubscription(ctx, item.ID)
}

func (s *DataService) UpdateKlineSubscription(ctx context.Context, id string, values map[string]any) (*entity.KlineSubscription, error) {
	current, err := s.klineSubscriptionRepo.FindByID(ctx, id)
	if current == nil || err != nil {
		return ensureFound(current, err)
	}
	item := mapKlineSubscription(values, current)
	item.ID = id
	if err := s.klineSubscriptionRepo.Update(ctx, item); err != nil {
		return nil, err
	}
	return s.GetKlineSubscription(ctx, id)
}

func (s *DataService) DeleteKlineSubscription(ctx context.Context, id string) error {
	return s.klineSubscriptionRepo.Delete(ctx, id)
}

func (s *DataService) ListIndicatorConfigs(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.indicatorConfigRepo.GetPagination(ctx, req)
}

func (s *DataService) GetIndicatorConfig(ctx context.Context, id string) (*entity.IndicatorConfig, error) {
	item, err := s.indicatorConfigRepo.FindByID(ctx, id)
	return ensureFound(item, err)
}

func (s *DataService) CreateIndicatorConfig(ctx context.Context, values map[string]any) (*entity.IndicatorConfig, error) {
	item := mapIndicatorConfig(values, nil)
	if err := s.indicatorConfigRepo.Create(ctx, item); err != nil {
		return nil, err
	}
	return s.GetIndicatorConfig(ctx, item.ID)
}

func (s *DataService) UpdateIndicatorConfig(ctx context.Context, id string, values map[string]any) (*entity.IndicatorConfig, error) {
	current, err := s.indicatorConfigRepo.FindByID(ctx, id)
	if current == nil || err != nil {
		_, err := ensureFound(current, err)
		return nil, err
	}
	item := mapIndicatorConfig(values, current)
	item.ID = current.ID
	if err := s.indicatorConfigRepo.Update(ctx, item); err != nil {
		return nil, err
	}
	return s.GetIndicatorConfig(ctx, id)
}

func (s *DataService) DeleteIndicatorConfig(ctx context.Context, id string) error {
	return s.indicatorConfigRepo.Delete(ctx, id)
}

func (s *DataService) ListStrategyConfigs(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	resp, err := s.strategyConfigRepo.GetPagination(ctx, req)
	if err != nil {
		return nil, err
	}
	return strategyConfigPaginationResponse(resp), nil
}

func (s *DataService) GetStrategyConfig(ctx context.Context, id string) (*StrategyConfigResponse, error) {
	item, err := s.strategyConfigRepo.FindByID(ctx, id)
	result, err := ensureFound(item, err)
	if err != nil {
		return nil, err
	}
	return strategyConfigResponse(result), nil
}

func (s *DataService) CreateStrategyConfig(ctx context.Context, values map[string]any) (*StrategyConfigResponse, error) {
	item := mapStrategyConfig(values, nil)
	if err := s.strategyConfigRepo.Create(ctx, item); err != nil {
		return nil, err
	}
	return s.GetStrategyConfig(ctx, item.ID)
}

func (s *DataService) UpdateStrategyConfig(ctx context.Context, id string, values map[string]any) (*StrategyConfigResponse, error) {
	current, err := s.strategyConfigRepo.FindByID(ctx, id)
	if current == nil || err != nil {
		_, err := ensureFound(current, err)
		return nil, err
	}
	wasEnabled := current.Enabled
	item := mapStrategyConfig(values, current)
	item.ID = current.ID
	if err := s.strategyConfigRepo.Update(ctx, item); err != nil {
		return nil, err
	}
	if wasEnabled && !item.Enabled {
		if err := s.closeOpenTradesForDisabledStrategy(ctx, item); err != nil {
			return nil, err
		}
	}
	return s.GetStrategyConfig(ctx, id)
}

func (s *DataService) DeleteStrategyConfig(ctx context.Context, id string) error {
	return s.strategyConfigRepo.Delete(ctx, id)
}

func (s *DataService) ListStrategyRules(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.strategyRuleRepo.GetPagination(ctx, req)
}

func (s *DataService) GetStrategyRule(ctx context.Context, id string) (*entity.StrategyRule, error) {
	item, err := s.strategyRuleRepo.FindByID(ctx, id)
	return ensureFound(item, err)
}

func (s *DataService) CreateStrategyRule(ctx context.Context, values map[string]any) (*entity.StrategyRule, error) {
	item := mapStrategyRule(values, nil)
	if err := s.strategyRuleRepo.Create(ctx, item); err != nil {
		return nil, err
	}
	return s.GetStrategyRule(ctx, item.ID)
}

func (s *DataService) UpdateStrategyRule(ctx context.Context, id string, values map[string]any) (*entity.StrategyRule, error) {
	current, err := s.strategyRuleRepo.FindByID(ctx, id)
	if current == nil || err != nil {
		_, err := ensureFound(current, err)
		return nil, err
	}
	item := mapStrategyRule(values, current)
	item.ID = current.ID
	if err := s.strategyRuleRepo.Update(ctx, item); err != nil {
		return nil, err
	}
	return s.GetStrategyRule(ctx, id)
}

func (s *DataService) DeleteStrategyRule(ctx context.Context, id string) error {
	return s.strategyRuleRepo.Delete(ctx, id)
}

func (s *DataService) closeOpenTradesForDisabledStrategy(ctx context.Context, config *entity.StrategyConfig) error {
	if s.js == nil {
		return errors.New("order close publisher is not configured")
	}

	openTrades, err := s.orderHistoryRepo.ListOpenEntriesByStrategyConfig(ctx, *config)
	if err != nil {
		return err
	}
	for _, trade := range openTrades {
		order, ok := buildDisableStrategyCloseOrder(*config, trade.OrderHistory)
		if !ok {
			continue
		}
		if err := util.PublishEvent(s.js, constant.OrderEngineStreamSubjectPlaceOrder, entity.OrderRequestEvent{Data: order}); err != nil {
			return err
		}
	}
	return nil
}

func buildDisableStrategyCloseOrder(config entity.StrategyConfig, entry entity.OrderHistory) (entity.OrderRequest, bool) {
	quantity := entry.FilledQuantity
	if quantity.IsZero() {
		quantity = entry.Quantity
	}
	if quantity.IsZero() {
		return entity.OrderRequest{}, false
	}
	entryOrderID := strings.TrimSpace(entry.EntryOrderID)
	if entryOrderID == "" {
		entryOrderID = strings.TrimSpace(entry.OrderID)
	}
	if entryOrderID == "" {
		entryOrderID = strings.TrimSpace(entry.ID)
	}

	side, ok := closeOrderSide(entry.Side, entity.NormalizeMarketType(entry.MarketType))
	if !ok {
		return entity.OrderRequest{}, false
	}

	strategyID := config.Strategy
	requestID := fmt.Sprintf("disable-strategy-%s-%s-%d", config.ID, entry.ID, time.Now().UTC().UnixMilli())
	clientOrderID := disableStrategyClientOrderID(config.ID, entry.ID)
	internalPayload, _ := json.Marshal(map[string]string{
		"source":         "disable_strategy",
		"entry_order_id": entryOrderID,
		"entry_id":       entry.ID,
	})

	return entity.OrderRequest{
		RequestID:        requestID,
		UserID:           entry.UserID,
		OrderID:          &clientOrderID,
		EntryOrderID:     entryOrderID,
		Exchange:         entry.Exchange,
		MarketType:       entry.MarketType,
		PositionSide:     entry.PositionSide,
		Symbol:           entry.Symbol,
		Type:             entity.OrderTypeMarket,
		Side:             side,
		Price:            decimal.Zero,
		Quantity:         quantity,
		RequestedAt:      time.Now().UTC().UnixMilli(),
		Source:           "dashboard",
		StrategyID:       &strategyID,
		StrategyName:     config.Strategy,
		Interval:         config.Interval,
		Internal:         string(internalPayload),
		TradeCondition:   string(entity.TradeConditionExit),
		OrderReason:      "strategy_disabled",
		ExitType:         "",
		NeedNotification: config.NeedNotification,
		IsPaperTrading:   entry.IsPaperTrading,
	}, true
}

func disableStrategyClientOrderID(configID, entryID string) string {
	now := strconv.FormatInt(time.Now().UTC().UnixMilli(), 36)
	return "ds" + compactAlnum(configID, 8) + compactAlnum(entryID, 10) + now
}

func compactAlnum(value string, limit int) string {
	cleaned := strings.NewReplacer("-", "", "_", "").Replace(strings.TrimSpace(value))
	if len(cleaned) <= limit {
		return cleaned
	}
	return cleaned[:limit]
}

func closeOrderSide(entrySide entity.OrderSide, marketType entity.MarketType) (entity.OrderSide, bool) {
	switch marketType {
	case entity.MarketTypeFutures:
		switch entrySide {
		case entity.OrderSideLong, entity.OrderSideBuy:
			return entity.OrderSideShort, true
		case entity.OrderSideShort, entity.OrderSideSell:
			return entity.OrderSideLong, true
		default:
			return "", false
		}
	default:
		switch entrySide {
		case entity.OrderSideBuy, entity.OrderSideLong:
			return entity.OrderSideSell, true
		case entity.OrderSideSell, entity.OrderSideShort:
			return entity.OrderSideBuy, true
		default:
			return "", false
		}
	}
}

func (s *DataService) ListSettings(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.settingRepo.GetPagination(ctx, req)
}

func (s *DataService) GetSetting(ctx context.Context, id string) (map[string]any, error) {
	return s.settingRepo.FindByID(ctx, id)
}

func (s *DataService) CreateSetting(ctx context.Context, values map[string]any) (map[string]any, error) {
	return s.settingRepo.Create(ctx, values)
}

func (s *DataService) UpdateSetting(ctx context.Context, id string, values map[string]any) (map[string]any, error) {
	return s.settingRepo.Update(ctx, id, values)
}

func (s *DataService) DeleteSetting(ctx context.Context, id string) error {
	return s.settingRepo.Delete(ctx, id)
}

func (s *DataService) ListUsers(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	resp, err := s.authRepo.GetUsersPagination(ctx, req)
	if err != nil {
		return nil, err
	}
	return apiUserPaginationResponse(resp), nil
}

func (s *DataService) GetUser(ctx context.Context, id string) (*APIUserResponse, error) {
	user, err := s.authRepo.FindUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	roles, err := s.authRepo.ListRoleNamesByUserID(ctx, id)
	if err != nil {
		return nil, err
	}
	result := apiUserResponse(user)
	result.Roles = roles
	return result, nil
}

func (s *DataService) CreateUser(ctx context.Context, values map[string]any) (*APIUserResponse, error) {
	password := stringValue(values, "password", "")
	passwordHash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}
	user := &entity.APIUser{
		Email:        strings.ToLower(stringValue(values, "email", "")),
		Name:         stringValue(values, "name", ""),
		PasswordHash: passwordHash,
		Active:       boolValue(values, "active", true),
	}
	if err := s.authRepo.CreateUser(ctx, user, stringSliceValue(values, "roles")); err != nil {
		return nil, err
	}
	return s.GetUser(ctx, user.ID)
}

func (s *DataService) UpdateUser(ctx context.Context, id string, values map[string]any) (*APIUserResponse, error) {
	current, err := s.authRepo.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	current.Email = strings.ToLower(stringValue(values, "email", current.Email))
	current.Name = stringValue(values, "name", current.Name)
	current.Active = boolValue(values, "active", current.Active)
	if password := stringValue(values, "password", ""); password != "" {
		current.PasswordHash, err = hashPassword(password)
		if err != nil {
			return nil, err
		}
	} else {
		current.PasswordHash = ""
	}
	var roles []string
	if _, ok := values["roles"]; ok {
		roles = stringSliceValue(values, "roles")
	} else {
		roles = nil
	}
	if err := s.authRepo.UpdateUser(ctx, current, roles); err != nil {
		return nil, err
	}
	return s.GetUser(ctx, id)
}

func (s *DataService) DeleteUser(ctx context.Context, id string) error {
	return s.authRepo.DeleteUser(ctx, id)
}

func (s *DataService) ListRoles(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.authRepo.GetRolesPagination(ctx, req)
}

func (s *DataService) GetRole(ctx context.Context, id string) (*APIRoleResponse, error) {
	role, err := s.authRepo.FindRoleByID(ctx, id)
	if err != nil {
		return nil, err
	}
	permissions, err := s.authRepo.ListPermissionNamesByRoleID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &APIRoleResponse{APIRole: *role, Permissions: permissions}, nil
}

func (s *DataService) CreateRole(ctx context.Context, values map[string]any) (*APIRoleResponse, error) {
	role := &entity.APIRole{
		Name:        stringValue(values, "name", ""),
		Description: stringValue(values, "description", ""),
	}
	if err := s.authRepo.CreateRole(ctx, role, stringSliceValue(values, "permissions")); err != nil {
		return nil, err
	}
	return s.GetRole(ctx, role.ID)
}

func (s *DataService) UpdateRole(ctx context.Context, id string, values map[string]any) (*APIRoleResponse, error) {
	current, err := s.authRepo.FindRoleByID(ctx, id)
	if err != nil {
		return nil, err
	}
	current.Name = stringValue(values, "name", current.Name)
	current.Description = stringValue(values, "description", current.Description)
	var permissions []string
	if _, ok := values["permissions"]; ok {
		permissions = stringSliceValue(values, "permissions")
	} else {
		permissions = nil
	}
	if err := s.authRepo.UpdateRole(ctx, current, permissions); err != nil {
		return nil, err
	}
	return s.GetRole(ctx, id)
}

func (s *DataService) DeleteRole(ctx context.Context, id string) error {
	return s.authRepo.DeleteRole(ctx, id)
}

func (s *DataService) ListPermissions(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.authRepo.GetPermissionsPagination(ctx, req)
}

func (s *DataService) GetPermission(ctx context.Context, id string) (*entity.APIPermission, error) {
	return s.authRepo.FindPermissionByID(ctx, id)
}

func (s *DataService) CreatePermission(ctx context.Context, values map[string]any) (*entity.APIPermission, error) {
	permission := &entity.APIPermission{
		Name:        stringValue(values, "name", ""),
		Description: stringValue(values, "description", ""),
	}
	if err := s.authRepo.CreatePermission(ctx, permission); err != nil {
		return nil, err
	}
	return s.GetPermission(ctx, permission.ID)
}

func (s *DataService) UpdatePermission(ctx context.Context, id string, values map[string]any) (*entity.APIPermission, error) {
	current, err := s.authRepo.FindPermissionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	current.Name = stringValue(values, "name", current.Name)
	current.Description = stringValue(values, "description", current.Description)
	if err := s.authRepo.UpdatePermission(ctx, current); err != nil {
		return nil, err
	}
	return s.GetPermission(ctx, id)
}

func (s *DataService) DeletePermission(ctx context.Context, id string) error {
	return s.authRepo.DeletePermission(ctx, id)
}

func (s *DataService) ListVisibleDashboardPages(ctx context.Context) ([]entity.APIDashboardPage, error) {
	return s.authRepo.ListVisibleDashboardPages(ctx)
}

func (s *DataService) ListDashboardPages(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.authRepo.GetDashboardPagesPagination(ctx, req)
}

func (s *DataService) GetDashboardPage(ctx context.Context, id string) (*entity.APIDashboardPage, error) {
	return s.authRepo.FindDashboardPageByID(ctx, id)
}

func (s *DataService) CreateDashboardPage(ctx context.Context, values map[string]any) (*entity.APIDashboardPage, error) {
	page := mapDashboardPage(values, nil)
	if err := s.authRepo.CreateDashboardPage(ctx, page); err != nil {
		return nil, err
	}
	return s.GetDashboardPage(ctx, page.ID)
}

func (s *DataService) UpdateDashboardPage(ctx context.Context, id string, values map[string]any) (*entity.APIDashboardPage, error) {
	current, err := s.authRepo.FindDashboardPageByID(ctx, id)
	if err != nil {
		return nil, err
	}
	page := mapDashboardPage(values, current)
	page.ID = id
	if err := s.authRepo.UpdateDashboardPage(ctx, page); err != nil {
		return nil, err
	}
	return s.GetDashboardPage(ctx, id)
}

func (s *DataService) DeleteDashboardPage(ctx context.Context, id string) error {
	return s.authRepo.DeleteDashboardPage(ctx, id)
}

func ensureFound[T any](item *T, err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, sql.ErrNoRows
	}
	return item, nil
}

func mapDashboardPage(values map[string]any, current *entity.APIDashboardPage) *entity.APIDashboardPage {
	item := &entity.APIDashboardPage{Visible: true}
	if current != nil {
		item = current
	}
	item.ResourceKey = stringValue(values, "resource_key", item.ResourceKey)
	item.ParentKey = stringValue(values, "parent_key", item.ParentKey)
	item.Label = stringValue(values, "label", item.Label)
	item.Description = stringValue(values, "description", item.Description)
	item.ShortDescription = stringValue(values, "short_description", item.ShortDescription)
	item.Icon = stringValue(values, "icon", item.Icon)
	item.Path = stringValue(values, "path", item.Path)
	item.ReadPermission = stringValue(values, "read_permission", item.ReadPermission)
	item.WritePermission = stringValue(values, "write_permission", item.WritePermission)
	item.SortOrder = intValue(values, "sort_order", item.SortOrder)
	item.Visible = boolValue(values, "visible", item.Visible)
	return item
}

func mapSymbolMapping(values map[string]any, current *entity.SymbolMapping) *entity.SymbolMapping {
	now := time.Now().UTC()
	item := &entity.SymbolMapping{CreatedAt: now, UpdatedAt: now}
	if current != nil {
		item = current
		item.UpdatedAt = now
	}
	item.Exchange = stringValue(values, "exchange", item.Exchange)
	item.MarketType = stringValue(values, "market_type", item.MarketType)
	item.Symbol = stringValue(values, "symbol", item.Symbol)
	item.KlineSymbol = stringValue(values, "kline_symbol", item.KlineSymbol)
	item.OrderSymbol = stringValue(values, "order_symbol", item.OrderSymbol)
	return item
}

func mapKlineSubscription(values map[string]any, current *entity.KlineSubscription) *entity.KlineSubscription {
	now := time.Now().UTC()
	item := &entity.KlineSubscription{Payload: "{}", MaxKlineData: 1000, CreatedAt: now, UpdatedAt: now}
	if current != nil {
		item = current
		item.UpdatedAt = now
	}
	item.Exchange = stringValue(values, "exchange", item.Exchange)
	item.MarketType = stringValue(values, "market_type", item.MarketType)
	item.Symbol = stringValue(values, "symbol", item.Symbol)
	item.Interval = stringValue(values, "interval", item.Interval)
	item.Payload = jsonStringValue(values, "payload", item.Payload)
	item.MaxKlineData = intValue(values, "max_kline_data", item.MaxKlineData)
	return item
}

func mapIndicatorConfig(values map[string]any, current *entity.IndicatorConfig) *entity.IndicatorConfig {
	now := time.Now().UTC()
	item := &entity.IndicatorConfig{Params: "{}", Enabled: true, CreatedAt: now, UpdatedAt: now}
	if current != nil {
		item = current
		item.UpdatedAt = now
	}
	item.Exchange = stringValue(values, "exchange", item.Exchange)
	item.MarketType = stringValue(values, "market_type", item.MarketType)
	item.Symbol = stringValue(values, "symbol", item.Symbol)
	item.Interval = stringValue(values, "interval", item.Interval)
	item.Indicator = strings.ToLower(stringValue(values, "indicator", item.Indicator))
	item.OutputName = stringValue(values, "output_name", item.OutputName)
	item.Params = jsonStringValue(values, "params", item.Params)
	item.Enabled = boolValue(values, "enabled", item.Enabled)
	return item
}

func mapStrategyConfig(values map[string]any, current *entity.StrategyConfig) *entity.StrategyConfig {
	now := time.Now().UTC()
	item := &entity.StrategyConfig{
		CreatedAt:                now,
		UpdatedAt:                now,
		PositionSide:             "BOTH",
		Source:                   "api",
		Enabled:                  true,
		NeedNotification:         true,
		IsPaperTrading:           true,
		OrderType:                "MARKET",
		OrderQty:                 decimal.NewFromInt(1),
		LimitSlippagePct:         decimal.Zero,
		CooldownBars:             2,
		SLCooldownBars:           3,
		MaxConsecutiveStopLosses: 2,
		SLPauseBars:              10,
		TakeProfitPct:            decimal.RequireFromString("0.25"),
		StopLossPct:              decimal.RequireFromString("0.15"),
		TrailingStopPct:          decimal.RequireFromString("0.12"),
		TrailingStopTriggerPct:   decimal.Zero,
		MaxHoldBars:              24,
		MaxPositions:             1,
		EnableIntrabarRiskExit:   true,
	}
	if current != nil {
		item = current
		item.UpdatedAt = now
	}

	item.Strategy = stringValue(values, "strategy", item.Strategy)
	item.Exchange = stringValue(values, "exchange", item.Exchange)
	item.MarketType = stringValue(values, "market_type", item.MarketType)
	item.Symbol = stringValue(values, "symbol", item.Symbol)
	item.Interval = stringValue(values, "interval", item.Interval)
	item.UserID = nullStringValue(values, "user_id", item.UserID)
	item.PositionSide = stringValue(values, "position_side", item.PositionSide)
	item.Source = stringValue(values, "source", item.Source)
	item.Enabled = boolValue(values, "enabled", item.Enabled)
	item.MonitorURL = stringValue(values, "monitor_url", item.MonitorURL)
	item.NeedNotification = boolValue(values, "need_notification", item.NeedNotification)
	item.IsPaperTrading = boolValue(values, "is_paper_trading", item.IsPaperTrading)
	item.OrderType = stringValue(values, "order_type", item.OrderType)
	item.OrderQty = decimalValue(values, "order_qty", item.OrderQty)
	item.LimitSlippagePct = decimalValue(values, "limit_slippage_pct", item.LimitSlippagePct)
	item.CooldownBars = intValue(values, "cooldown_bars", item.CooldownBars)
	item.SLCooldownBars = intValue(values, "sl_cooldown_bars", item.SLCooldownBars)
	item.MaxConsecutiveStopLosses = intValue(values, "max_consecutive_stop_losses", item.MaxConsecutiveStopLosses)
	item.SLPauseBars = intValue(values, "sl_pause_bars", item.SLPauseBars)
	item.TakeProfitPct = decimalValue(values, "take_profit_pct", item.TakeProfitPct)
	item.StopLossPct = decimalValue(values, "stop_loss_pct", item.StopLossPct)
	item.TrailingStopPct = decimalValue(values, "trailing_stop_pct", item.TrailingStopPct)
	item.TrailingStopTriggerPct = decimalValue(values, "trailing_stop_trigger_pct", item.TrailingStopTriggerPct)
	item.MaxHoldBars = intValue(values, "max_hold_bars", item.MaxHoldBars)
	item.MaxPositions = intValue(values, "max_positions", item.MaxPositions)
	item.EnableIntrabarRiskExit = boolValue(values, "enable_intrabar_risk_exit", item.EnableIntrabarRiskExit)
	return item
}

func mapStrategyRule(values map[string]any, current *entity.StrategyRule) *entity.StrategyRule {
	now := time.Now().UTC()
	item := &entity.StrategyRule{
		Conditions:     `{"all":[]}`,
		TradeCondition: string(entity.TradeConditionEntry),
		Enabled:        true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if current != nil {
		item = current
		item.UpdatedAt = now
	}
	item.StrategyConfigID = stringValue(values, "strategy_config_id", item.StrategyConfigID)
	item.Name = stringValue(values, "name", item.Name)
	item.Side = strings.ToUpper(stringValue(values, "side", item.Side))
	item.TradeCondition = string(entity.NormalizeTradeCondition(stringValue(values, "trade_condition", item.TradeCondition)))
	item.ExitType = string(entity.NormalizeExitType(stringValue(values, "exit_type", item.ExitType)))
	item.OrderReason = stringValue(values, "order_reason", item.OrderReason)
	item.Conditions = jsonStringValue(values, "conditions", item.Conditions)
	item.Enabled = boolValue(values, "enabled", item.Enabled)
	return item
}

func stringValue(values map[string]any, key string, fallback string) string {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func jsonStringValue(values map[string]any, key string, fallback string) string {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fallback
		}
		return string(b)
	}
}

func intValue(values map[string]any, key string, fallback int) int {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		i, err := v.Int64()
		if err == nil {
			return int(i)
		}
	default:
		i, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(v)))
		if err == nil {
			return i
		}
	}
	return fallback
}

func boolValue(values map[string]any, key string, fallback bool) bool {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	switch v := value.(type) {
	case bool:
		return v
	default:
		parsed, err := strconv.ParseBool(strings.TrimSpace(fmt.Sprint(v)))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func settingBoolValue(value any, fallback bool) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		raw := strings.TrimSpace(v)
		var parsed bool
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			return parsed
		}
		if parsed, err := strconv.ParseBool(raw); err == nil {
			return parsed
		}
	case map[string]any:
		if nested, ok := v["enabled"]; ok {
			return settingBoolValue(nested, fallback)
		}
		if nested, ok := v["show_setup_link"]; ok {
			return settingBoolValue(nested, fallback)
		}
	}
	return fallback
}

func decimalValue(values map[string]any, key string, fallback decimal.Decimal) decimal.Decimal {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	parsed, err := decimal.NewFromString(strings.TrimSpace(fmt.Sprint(value)))
	if err != nil {
		return fallback
	}
	return parsed
}

func nullStringValue(values map[string]any, key string, fallback sql.NullString) sql.NullString {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	raw := strings.TrimSpace(fmt.Sprint(value))
	return sql.NullString{String: raw, Valid: raw != ""}
}

func stringSliceValue(values map[string]any, key string) []string {
	value, ok := values[key]
	if !ok {
		return []string{}
	}
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if raw := strings.TrimSpace(fmt.Sprint(item)); raw != "" {
				result = append(result, raw)
			}
		}
		return result
	case string:
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, item := range parts {
			if raw := strings.TrimSpace(item); raw != "" {
				result = append(result, raw)
			}
		}
		return result
	default:
		return []string{}
	}
}
