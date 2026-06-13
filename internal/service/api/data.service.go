package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/shopspring/decimal"
)

type DataService struct {
	authRepo              *repository.APIAuthRepository
	settingRepo           *repository.APISettingRepository
	orderHistoryRepo      *repository.OrderHistoryRepository
	marketKlineRepo       *repository.MarketKlineRepository
	symbolMappingRepo     *repository.SymbolMappingRepository
	klineSubscriptionRepo *repository.KlineSubscriptionRepository
	strategyConfigRepo    *repository.StrategyConfigRepository
}

func NewDataService(apiDB, marketDB, orderDB *sqlx.DB) *DataService {
	return &DataService{
		authRepo:              repository.NewAPIAuthRepository(apiDB),
		settingRepo:           repository.NewAPISettingRepository(apiDB),
		orderHistoryRepo:      repository.NewOrderHistoryRepository(orderDB),
		marketKlineRepo:       repository.NewMarketKlineRepository(marketDB),
		symbolMappingRepo:     repository.NewSymbolMappingRepository(marketDB),
		klineSubscriptionRepo: repository.NewKlineSubscriptionRepository(marketDB),
		strategyConfigRepo:    repository.NewStrategyConfigRepository(marketDB),
	}
}

func (s *DataService) ListOrders(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.orderHistoryRepo.GetPagination(ctx, req)
}

func (s *DataService) GetOrder(ctx context.Context, id string) (any, error) {
	item, err := s.orderHistoryRepo.FindByID(ctx, id)
	return ensureFound(item, err)
}

func (s *DataService) ListMarketKlines(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.marketKlineRepo.GetPagination(ctx, req)
}

func (s *DataService) GetMarketKline(ctx context.Context, openTime string) (any, error) {
	item, err := s.marketKlineRepo.FindByOpenTime(ctx, openTime)
	return ensureFound(item, err)
}

func (s *DataService) ListSymbolMappings(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.symbolMappingRepo.GetPagination(ctx, req)
}

func (s *DataService) GetSymbolMapping(ctx context.Context, id string) (any, error) {
	item, err := s.symbolMappingRepo.FindByID(ctx, id)
	return ensureFound(item, err)
}

func (s *DataService) CreateSymbolMapping(ctx context.Context, values map[string]any) (any, error) {
	item := mapSymbolMapping(values, nil)
	if err := s.symbolMappingRepo.Create(ctx, item); err != nil {
		return nil, err
	}
	return s.GetSymbolMapping(ctx, item.ID)
}

func (s *DataService) UpdateSymbolMapping(ctx context.Context, id string, values map[string]any) (any, error) {
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

func (s *DataService) GetKlineSubscription(ctx context.Context, id string) (any, error) {
	item, err := s.klineSubscriptionRepo.FindByID(ctx, id)
	return ensureFound(item, err)
}

func (s *DataService) CreateKlineSubscription(ctx context.Context, values map[string]any) (any, error) {
	item := mapKlineSubscription(values, nil)
	if err := s.klineSubscriptionRepo.Create(ctx, item); err != nil {
		return nil, err
	}
	return s.GetKlineSubscription(ctx, item.ID)
}

func (s *DataService) UpdateKlineSubscription(ctx context.Context, id string, values map[string]any) (any, error) {
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

func (s *DataService) ListStrategyConfigs(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.strategyConfigRepo.GetPagination(ctx, req)
}

func (s *DataService) GetStrategyConfig(ctx context.Context, id string) (any, error) {
	item, err := s.strategyConfigRepo.FindByID(ctx, id)
	return ensureFound(item, err)
}

func (s *DataService) CreateStrategyConfig(ctx context.Context, values map[string]any) (any, error) {
	item := mapStrategyConfig(values, nil)
	if err := s.strategyConfigRepo.Create(ctx, item); err != nil {
		return nil, err
	}
	return s.GetStrategyConfig(ctx, strconv.FormatInt(item.ID, 10))
}

func (s *DataService) UpdateStrategyConfig(ctx context.Context, id string, values map[string]any) (any, error) {
	current, err := s.strategyConfigRepo.FindByID(ctx, id)
	if current == nil || err != nil {
		return ensureFound(current, err)
	}
	item := mapStrategyConfig(values, current)
	item.ID = current.ID
	if err := s.strategyConfigRepo.Update(ctx, item); err != nil {
		return nil, err
	}
	return s.GetStrategyConfig(ctx, id)
}

func (s *DataService) DeleteStrategyConfig(ctx context.Context, id string) error {
	return s.strategyConfigRepo.Delete(ctx, id)
}

func (s *DataService) ListSettings(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.settingRepo.GetPagination(ctx, req)
}

func (s *DataService) GetSetting(ctx context.Context, id string) (any, error) {
	return s.settingRepo.FindByID(ctx, id)
}

func (s *DataService) CreateSetting(ctx context.Context, values map[string]any) (any, error) {
	return s.settingRepo.Create(ctx, values)
}

func (s *DataService) UpdateSetting(ctx context.Context, id string, values map[string]any) (any, error) {
	return s.settingRepo.Update(ctx, id, values)
}

func (s *DataService) DeleteSetting(ctx context.Context, id string) error {
	return s.settingRepo.Delete(ctx, id)
}

func (s *DataService) ListUsers(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.authRepo.GetUsersPagination(ctx, req)
}

func (s *DataService) GetUser(ctx context.Context, id string) (any, error) {
	return s.authRepo.FindUserByID(ctx, id)
}

func (s *DataService) ListRoles(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	return s.authRepo.GetRolesPagination(ctx, req)
}

func (s *DataService) GetRole(ctx context.Context, id string) (any, error) {
	return s.authRepo.FindRoleByID(ctx, id)
}

func ensureFound[T any](item *T, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, sql.ErrNoRows
	}
	return item, nil
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

func mapStrategyConfig(values map[string]any, current *entity.StrategyConfig) *entity.StrategyConfig {
	now := time.Now().UTC()
	item := &entity.StrategyConfig{
		CreatedAt:                now,
		UpdatedAt:                now,
		PositionSide:             "BOTH",
		Source:                   "api",
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
