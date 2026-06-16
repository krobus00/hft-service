package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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

type AuthConfigResponse struct {
	ShowSetupLink bool `json:"show_setup_link"`
}

type FormEnumsResponse map[string][]string

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
		"dashboard_page":   {"orders", "orderPnL", "dailyReports", "strategyPerformance", "marketKlines", "marketBackfills", "symbolMappings", "klineSubscriptions", "strategyConfigs", "settings", "users", "roles", "permissions", "dashboardPages"},
		"market_type":      {"spot", "futures"},
		"position_side":    {"BOTH", "LONG", "SHORT"},
		"order_side":       {"BUY", "SELL", "LONG", "SHORT"},
		"order_type":       {"MARKET", "LIMIT"},
		"order_status":     {"NEW", "PARTIALLY_FILLED", "FILLED", "CANCELED", "REJECTED", "EXPIRED"},
		"trade_condition":  {"ENTRY", "EXIT", "TAKE_PROFIT", "STOP_LOSS", "TRAILING_STOP", "SIGNAL", "UNKNOWN"},
		"exit_type":        {"", "TAKE_PROFIT", "STOP_LOSS", "TRAILING_STOP"},
		"interval":         {"1m", "3m", "5m", "15m", "30m", "1h", "4h", "1d"},
		"kline_event_type": {"kline"},
		"source":           {"dashboard", "api", "strategy"},
	}, nil
}

func (s *DataService) listExchangeEnums(ctx context.Context) ([]string, error) {
	sources := []func(context.Context) ([]string, error){
		s.orderHistoryRepo.ListExchanges,
		s.marketKlineRepo.ListExchanges,
		s.symbolMappingRepo.ListExchanges,
		s.strategyConfigRepo.ListExchanges,
	}
	seen := map[string]struct{}{}
	for _, source := range sources {
		items, err := source(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			value := strings.TrimSpace(item)
			if value != "" {
				seen[value] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for item := range seen {
		result = append(result, item)
	}
	sort.Strings(result)
	return result, nil
}

func (s *DataService) ListOrders(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	resp, err := s.orderHistoryRepo.GetPagination(ctx, req)
	if err != nil {
		return nil, err
	}
	return orderHistoryPaginationResponse(resp), nil
}

func (s *DataService) GetOrder(ctx context.Context, id string) (*OrderHistoryResponse, error) {
	item, err := s.orderHistoryRepo.FindByID(ctx, id)
	result, err := ensureFound(item, err)
	if err != nil {
		return nil, err
	}
	return orderHistoryResponse(result), nil
}

func (s *DataService) CreateOrder(ctx context.Context, values map[string]any) (*OrderHistoryResponse, error) {
	item := mapOrderHistory(values, nil)
	if err := s.orderHistoryRepo.Create(ctx, item); err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, item.ID)
}

func (s *DataService) UpdateOrder(ctx context.Context, id string, values map[string]any) (*OrderHistoryResponse, error) {
	current, err := s.orderHistoryRepo.FindByID(ctx, id)
	if current == nil || err != nil {
		_, err := ensureFound(current, err)
		return nil, err
	}
	item := mapOrderHistory(values, current)
	item.ID = id
	if err := s.orderHistoryRepo.Update(ctx, item); err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, id)
}

func (s *DataService) DeleteOrder(ctx context.Context, id string) error {
	return s.orderHistoryRepo.Delete(ctx, id)
}

func (s *DataService) ListOrderTradePnL(ctx context.Context, filter entity.OrderReportFilter) (*apiutil.PaginationResp, error) {
	items, total, err := s.orderHistoryRepo.ListTradePnL(ctx, filter)
	if err != nil {
		return nil, err
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return apiutil.NewPaginationResp(page, limit, total, items), nil
}

func (s *DataService) ListDailyOrderReports(ctx context.Context, filter entity.OrderReportFilter) ([]entity.DailyOrderReport, error) {
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

func (s *DataService) CreateMarketKline(ctx context.Context, values map[string]any) (*entity.MarketKline, error) {
	item := mapMarketKline(values, nil)
	if err := s.marketKlineRepo.Create(ctx, item); err != nil {
		return nil, err
	}
	return s.GetMarketKline(ctx, item.ID)
}

func (s *DataService) UpdateMarketKline(ctx context.Context, id string, values map[string]any) (*entity.MarketKline, error) {
	current, err := s.marketKlineRepo.FindByID(ctx, id)
	if current == nil || err != nil {
		return ensureFound(current, err)
	}
	item := mapMarketKline(values, current)
	item.ID = id
	if err := s.marketKlineRepo.Update(ctx, item); err != nil {
		return nil, err
	}
	return s.GetMarketKline(ctx, id)
}

func (s *DataService) DeleteMarketKline(ctx context.Context, id string) error {
	return s.marketKlineRepo.Delete(ctx, id)
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
	return apiUserResponse(user), nil
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

func (s *DataService) GetRole(ctx context.Context, id string) (*entity.APIRole, error) {
	return s.authRepo.FindRoleByID(ctx, id)
}

func (s *DataService) CreateRole(ctx context.Context, values map[string]any) (*entity.APIRole, error) {
	role := &entity.APIRole{
		Name:        stringValue(values, "name", ""),
		Description: stringValue(values, "description", ""),
	}
	if err := s.authRepo.CreateRole(ctx, role, stringSliceValue(values, "permissions")); err != nil {
		return nil, err
	}
	return s.GetRole(ctx, role.ID)
}

func (s *DataService) UpdateRole(ctx context.Context, id string, values map[string]any) (*entity.APIRole, error) {
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

func mapOrderHistory(values map[string]any, current *entity.OrderHistory) *entity.OrderHistory {
	now := time.Now().UTC()
	item := &entity.OrderHistory{
		CreatedAt:      now,
		UpdatedAt:      now,
		MarketType:     "spot",
		PositionSide:   "BOTH",
		TradeCondition: "UNKNOWN",
	}
	if current != nil {
		item = current
		item.UpdatedAt = now
	}
	item.RequestID = stringValue(values, "request_id", item.RequestID)
	item.UserID = stringValue(values, "user_id", item.UserID)
	item.Exchange = stringValue(values, "exchange", item.Exchange)
	item.MarketType = stringValue(values, "market_type", item.MarketType)
	item.PositionSide = stringValue(values, "position_side", item.PositionSide)
	item.Symbol = stringValue(values, "symbol", item.Symbol)
	item.OrderID = stringValue(values, "order_id", item.OrderID)
	item.EntryOrderID = stringValue(values, "entry_order_id", item.EntryOrderID)
	item.ClientOrderID = nullStringValue(values, "client_order_id", item.ClientOrderID)
	item.Side = entity.OrderSide(stringValue(values, "side", string(item.Side)))
	item.Type = entity.OrderType(stringValue(values, "type", string(item.Type)))
	item.Price = decimalPtrValue(values, "price", item.Price)
	item.Quantity = decimalValue(values, "quantity", item.Quantity)
	item.FilledQuantity = decimalValue(values, "filled_quantity", item.FilledQuantity)
	item.AvgFillPrice = decimalPtrValue(values, "avg_fill_price", item.AvgFillPrice)
	item.Status = stringValue(values, "status", item.Status)
	item.Leverage = decimalPtrValue(values, "leverage", item.Leverage)
	item.Fee = decimalPtrValue(values, "fee", item.Fee)
	item.RealizedPnl = decimalPtrValue(values, "realized_pnl", item.RealizedPnl)
	item.CreatedAtExchange = nullTimeValue(values, "created_at_exchange", item.CreatedAtExchange)
	item.SentAt = nullTimeValue(values, "sent_at", item.SentAt)
	item.AcknowledgedAt = nullTimeValue(values, "acknowledged_at", item.AcknowledgedAt)
	item.FilledAt = nullTimeValue(values, "filled_at", item.FilledAt)
	item.StrategyID = nullStringValue(values, "strategy_id", item.StrategyID)
	item.TradeCondition = stringValue(values, "trade_condition", item.TradeCondition)
	item.OrderReason = stringValue(values, "order_reason", item.OrderReason)
	item.ExitType = stringValue(values, "exit_type", item.ExitType)
	item.ErrorMessage = nullStringValue(values, "error_message", item.ErrorMessage)
	item.IsPaperTrading = boolValue(values, "is_paper_trading", item.IsPaperTrading)
	return item
}

func mapMarketKline(values map[string]any, current *entity.MarketKline) *entity.MarketKline {
	now := time.Now().UTC()
	item := &entity.MarketKline{
		CreatedAt:  now,
		UpdatedAt:  now,
		MarketType: "spot",
		EventTime:  now,
		OpenTime:   now,
		CloseTime:  now,
	}
	if current != nil {
		item = current
		item.UpdatedAt = now
	}
	item.ID = stringValue(values, "id", item.ID)
	item.Exchange = stringValue(values, "exchange", item.Exchange)
	item.MarketType = stringValue(values, "market_type", item.MarketType)
	item.EventType = stringValue(values, "event_type", item.EventType)
	item.EventTime = timeValue(values, "event_time", item.EventTime)
	item.Symbol = stringValue(values, "symbol", item.Symbol)
	item.Interval = stringValue(values, "interval", item.Interval)
	item.OpenTime = timeValue(values, "open_time", item.OpenTime)
	item.CloseTime = timeValue(values, "close_time", item.CloseTime)
	item.OpenPrice = decimalValue(values, "open_price", item.OpenPrice)
	item.HighPrice = decimalValue(values, "high_price", item.HighPrice)
	item.LowPrice = decimalValue(values, "low_price", item.LowPrice)
	item.ClosePrice = decimalValue(values, "close_price", item.ClosePrice)
	item.BaseVolume = decimalValue(values, "base_volume", item.BaseVolume)
	item.QuoteVolume = decimalValue(values, "quote_volume", item.QuoteVolume)
	item.TakerBaseVolume = decimalValue(values, "taker_base_volume", item.TakerBaseVolume)
	item.TakerQuoteVolume = decimalValue(values, "taker_quote_volume", item.TakerQuoteVolume)
	item.TradeCount = int32(intValue(values, "trade_count", int(item.TradeCount)))
	item.IsClosed = boolValue(values, "is_closed", item.IsClosed)
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

func decimalPtrValue(values map[string]any, key string, fallback *decimal.Decimal) *decimal.Decimal {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	raw := strings.TrimSpace(fmt.Sprint(value))
	if raw == "" || raw == "<nil>" {
		return nil
	}
	parsed, err := decimal.NewFromString(raw)
	if err != nil {
		return fallback
	}
	return &parsed
}

func nullStringValue(values map[string]any, key string, fallback sql.NullString) sql.NullString {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	raw := strings.TrimSpace(fmt.Sprint(value))
	return sql.NullString{String: raw, Valid: raw != ""}
}

func nullTimeValue(values map[string]any, key string, fallback sql.NullTime) sql.NullTime {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	raw := strings.TrimSpace(fmt.Sprint(value))
	if raw == "" || raw == "<nil>" {
		return sql.NullTime{}
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return fallback
	}
	return sql.NullTime{Time: parsed, Valid: true}
}

func timeValue(values map[string]any, key string, fallback time.Time) time.Time {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	raw := strings.TrimSpace(fmt.Sprint(value))
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return fallback
	}
	return parsed
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
