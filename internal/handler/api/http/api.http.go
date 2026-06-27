package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/constant"
	"github.com/krobus00/hft-service/internal/entity"
	apiservice "github.com/krobus00/hft-service/internal/service/api"
)

type contextKey string

const claimsContextKey contextKey = "claims"

type Handler struct {
	authService     *apiservice.AuthService
	dataService     *apiservice.DataService
	backfillService *apiservice.BackfillService
	authConfig      config.DashboardAuthConfig
}

func NewAPIHTTPHandler(authService *apiservice.AuthService, dataService *apiservice.DataService, backfillService *apiservice.BackfillService, authConfig config.DashboardAuthConfig) *Handler {
	return &Handler{authService: authService, dataService: dataService, backfillService: backfillService, authConfig: authConfig}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/auth/setup", h.Setup)
	mux.HandleFunc("/api/v1/auth/login", h.Login)
	mux.HandleFunc("/api/v1/auth/config", h.AuthConfig)
	mux.HandleFunc("/api/v1/auth/refresh", h.Refresh)
	mux.HandleFunc("/api/v1/auth/logout", h.withAuth(h.Logout))
	mux.HandleFunc("/api/v1/auth/me", h.withAuth(h.Me))
	mux.HandleFunc("/api/v1/auth/profile", h.withAuth(h.Profile))
	mux.HandleFunc("/api/v1/form/enums", h.withAuth(h.FormEnums))
	mux.HandleFunc("/api/v1/dashboard/pages/menu", h.withAuth(h.DashboardPageMenu))
	mux.HandleFunc("/api/v1/dashboard/overview", h.withPermission(constant.PermissionOrderRead, h.DashboardOverview))

	mux.HandleFunc("/api/v1/order-reports/trades", h.withPermission(constant.PermissionOrderReportRead, h.OrderTradePnL))
	mux.HandleFunc("/api/v1/order-reports/daily", h.withPermission(constant.PermissionOrderReportRead, h.DailyOrderReports))
	mux.HandleFunc("/api/v1/order-reports/strategy-performance", h.withPermission(constant.PermissionOrderReportRead, h.StrategyPerformanceReports))
	mux.HandleFunc("/api/v1/orders", h.withPermission(constant.PermissionOrderRead, h.Orders))
	mux.HandleFunc("/api/v1/orders/", h.withPermission(constant.PermissionOrderRead, h.OrderByID))
	mux.HandleFunc("/api/v1/market/backfills", h.withPermission(constant.PermissionMarketRead, h.MarketBackfills))
	mux.HandleFunc("/api/v1/market/backfills/", h.withPermission(constant.PermissionMarketRead, h.MarketBackfillByID))
	mux.HandleFunc("/api/v1/market/klines", h.withPermission(constant.PermissionMarketRead, h.MarketKlines))
	mux.HandleFunc("/api/v1/market/klines/", h.withPermission(constant.PermissionMarketRead, h.MarketKlineByID))
	mux.HandleFunc("/api/v1/market/price-references", h.withPermission(constant.PermissionMarketRead, h.PriceReferences))
	mux.HandleFunc("/api/v1/market/price-references/", h.withPermission(constant.PermissionMarketRead, h.PriceReferenceByID))
	mux.HandleFunc("/api/v1/market/symbol-mappings", h.withPermission(constant.PermissionMarketRead, h.SymbolMappings))
	mux.HandleFunc("/api/v1/market/symbol-mappings/", h.withPermission(constant.PermissionMarketRead, h.SymbolMappingByID))
	mux.HandleFunc("/api/v1/market/kline-subscriptions", h.withPermission(constant.PermissionMarketRead, h.KlineSubscriptions))
	mux.HandleFunc("/api/v1/market/kline-subscriptions/", h.withPermission(constant.PermissionMarketRead, h.KlineSubscriptionByID))
	mux.HandleFunc("/api/v1/market/indicator-results/recalculate-missing", h.withPermission(constant.PermissionMarketRead, h.RecalculateMissingIndicators))
	mux.HandleFunc("/api/v1/market/indicator-results/recalculate-missing/", h.withPermission(constant.PermissionMarketRead, h.RecalculateMissingIndicatorJob))
	mux.HandleFunc("/api/v1/market/indicator-configs", h.withPermission(constant.PermissionMarketRead, h.IndicatorConfigs))
	mux.HandleFunc("/api/v1/market/indicator-configs/", h.withPermission(constant.PermissionMarketRead, h.IndicatorConfigByID))
	mux.HandleFunc("/api/v1/strategy/monitors", h.withPermission(constant.PermissionStrategyConfigRead, h.StrategyMonitors))
	mux.HandleFunc("/api/v1/strategy/monitors/", h.withPermission(constant.PermissionStrategyConfigRead, h.StrategyMonitorByName))
	mux.HandleFunc("/api/v1/strategy/configs", h.withPermission(constant.PermissionStrategyConfigRead, h.StrategyConfigs))
	mux.HandleFunc("/api/v1/strategy/configs/", h.withPermission(constant.PermissionStrategyConfigRead, h.StrategyConfigByID))
	mux.HandleFunc("/api/v1/strategy/rules", h.withPermission(constant.PermissionStrategyConfigRead, h.StrategyRules))
	mux.HandleFunc("/api/v1/strategy/rules/", h.withPermission(constant.PermissionStrategyConfigRead, h.StrategyRuleByID))
	mux.HandleFunc("/api/v1/settings", h.withPermission(constant.PermissionSettingsRead, h.Settings))
	mux.HandleFunc("/api/v1/settings/", h.withPermission(constant.PermissionSettingsRead, h.SettingByID))
	mux.HandleFunc("/api/v1/users", h.withPermission(constant.PermissionUserRead, h.Users))
	mux.HandleFunc("/api/v1/users/", h.withPermission(constant.PermissionUserRead, h.UserByID))
	mux.HandleFunc("/api/v1/roles", h.withPermission(constant.PermissionUserRead, h.Roles))
	mux.HandleFunc("/api/v1/roles/", h.withPermission(constant.PermissionUserRead, h.RoleByID))
	mux.HandleFunc("/api/v1/permissions", h.withPermission(constant.PermissionPermissionRead, h.Permissions))
	mux.HandleFunc("/api/v1/permissions/", h.withPermission(constant.PermissionPermissionRead, h.PermissionByID))
	mux.HandleFunc("/api/v1/dashboard/pages", h.withPermission(constant.PermissionDashboardPageRead, h.DashboardPages))
	mux.HandleFunc("/api/v1/dashboard/pages/", h.withPermission(constant.PermissionDashboardPageRead, h.DashboardPageByID))
}

type setupRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type profileRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

type marketBackfillRequest struct {
	Exchange   string `json:"exchange"`
	MarketType string `json:"market_type"`
	Symbol     string `json:"symbol"`
	Interval   string `json:"interval"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
}

func (h *Handler) Setup(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req setupRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.Password) == "" {
		apiutil.WriteError(w, http.StatusBadRequest, constant.BadRequestStatusCode, "email and password are required")
		return
	}
	result, err := h.authService.Setup(r.Context(), req.Email, req.Name, req.Password)
	writeAuthResult(w, result, err)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req loginRequest
	if !decodeBody(w, r, &req) {
		return
	}
	result, err := h.authService.Login(r.Context(), req.Email, req.Password)
	writeAuthResult(w, result, err)
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req refreshRequest
	if !decodeBody(w, r, &req) {
		return
	}
	result, err := h.authService.Refresh(r.Context(), req.RefreshToken)
	writeAuthResult(w, result, err)
}

func (h *Handler) AuthConfig(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	result, err := h.dataService.GetAuthConfig(r.Context())
	writeDataResult(w, result, err)
}

func (h *Handler) FormEnums(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	result, err := h.dataService.GetFormEnums(r.Context())
	writeDataResult(w, result, err)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req refreshRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.authService.Logout(r.Context(), req.RefreshToken); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, constant.BadRequestStatusCode, err.Error())
		return
	}
	apiutil.WriteSuccess(w, map[string]string{"status": "logged_out"})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	claims := claimsFromContext(r.Context())
	if claims == nil || strings.TrimSpace(claims.Subject) == "" {
		apiutil.WriteError(w, http.StatusUnauthorized, constant.UnauthorizedStatusCode, "invalid authorization token")
		return
	}
	user, err := h.authService.Me(r.Context(), claims.Subject)
	writeDataResult(w, user, err)
}

func (h *Handler) Profile(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPatch) {
		return
	}
	claims := claimsFromContext(r.Context())
	if claims == nil || strings.TrimSpace(claims.Subject) == "" {
		apiutil.WriteError(w, http.StatusUnauthorized, constant.UnauthorizedStatusCode, "invalid authorization token")
		return
	}
	var req profileRequest
	if !decodeBody(w, r, &req) {
		return
	}
	user, err := h.authService.UpdateProfile(r.Context(), claims.Subject, req.Name, req.Password)
	writeDataResult(w, user, err)
}

func (h *Handler) DashboardPageMenu(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	result, err := h.dataService.ListVisibleDashboardPages(r.Context())
	writeDataResult(w, result, err)
}

func (h *Handler) DashboardOverview(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	result, err := h.dataService.GetDashboardOverview(r.Context())
	writeDataResult(w, result, err)
}

func (h *Handler) Orders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		req, ok := parsePagination(w, r, &entity.OrderHistory{})
		if !ok {
			return
		}
		result, err := h.dataService.ListOrders(r.Context(), req)
		writeDataResult(w, result, err)
	case http.MethodPost:
		if !requirePermission(w, r, constant.PermissionOrderWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		claims := claimsFromContext(r.Context())
		result, err := h.dataService.CreateManualOrder(r.Context(), claims.Subject, body)
		writeOrderActionResult(w, result, err)
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) OrderTradePnL(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	filter, ok := parseOrderReportFilter(w, r)
	if !ok {
		return
	}
	result, err := h.dataService.ListOrderTradePnL(r.Context(), filter)
	writeDataResult(w, result, err)
}

func (h *Handler) DailyOrderReports(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	filter, ok := parseOrderReportFilter(w, r)
	if !ok {
		return
	}
	result, err := h.dataService.ListDailyOrderReports(r.Context(), filter)
	writeDataResult(w, result, err)
}

func (h *Handler) StrategyPerformanceReports(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	filter, ok := parseOrderReportFilter(w, r)
	if !ok {
		return
	}
	result, err := h.dataService.ListStrategyPerformanceReports(r.Context(), filter)
	writeDataResult(w, result, err)
}

func (h *Handler) OrderByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "/api/v1/orders/")
	if !ok {
		return
	}
	if strings.HasSuffix(id, "/close") {
		if !requireMethod(w, r, http.MethodPost) || !requirePermission(w, r, constant.PermissionOrderWrite) {
			return
		}
		result, err := h.dataService.CloseOrder(r.Context(), strings.TrimSuffix(id, "/close"))
		writeOrderActionResult(w, result, err)
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	result, err := h.dataService.GetOrder(r.Context(), id)
	writeDataResult(w, result, err)
}

func writeOrderActionResult(w http.ResponseWriter, result *apiservice.ManualOrderResult, err error) {
	if err == nil {
		apiutil.WriteSuccess(w, result)
		return
	}
	if errors.Is(err, apiservice.ErrInvalidManualOrder) || errors.Is(err, apiservice.ErrPositionNotRunning) {
		apiutil.WriteError(w, http.StatusBadRequest, constant.BadRequestStatusCode, err.Error())
		return
	}
	writeDataResult(w, nil, err)
}

func (h *Handler) MarketKlines(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	req, ok := parsePagination(w, r, &entity.MarketKline{})
	if !ok {
		return
	}
	result, err := h.dataService.ListMarketKlines(r.Context(), req)
	writeDataResult(w, result, err)
}

func (h *Handler) MarketKlineByID(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	id, ok := pathID(w, r, "/api/v1/market/klines/")
	if !ok {
		return
	}
	result, err := h.dataService.GetMarketKline(r.Context(), id)
	writeDataResult(w, result, err)
}

func (h *Handler) PriceReferences(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	req, ok := parsePagination(w, r, &entity.PriceReference{})
	if !ok {
		return
	}
	result, err := h.dataService.ListPriceReferences(r.Context(), req)
	writeDataResult(w, result, err)
}

func (h *Handler) PriceReferenceByID(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	id, ok := pathID(w, r, "/api/v1/market/price-references/")
	if !ok {
		return
	}
	result, err := h.dataService.GetPriceReference(r.Context(), id)
	writeDataResult(w, result, err)
}

func (h *Handler) MarketBackfills(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !requirePermission(w, r, constant.PermissionMarketConfigWrite) {
		return
	}
	var body marketBackfillRequest
	if !decodeBody(w, r, &body) {
		return
	}
	req, ok := parseMarketBackfillRequest(w, body)
	if !ok {
		return
	}
	job, err := h.backfillService.Start(req)
	if err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, constant.BadRequestStatusCode, err.Error())
		return
	}
	apiutil.WriteSuccess(w, job)
}

func (h *Handler) MarketBackfillByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "/api/v1/market/backfills/")
	if !ok {
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	wait := parseWaitDuration(r)
	var job *apiservice.BackfillJob
	var found bool
	if wait > 0 {
		job, found = h.backfillService.Wait(r.Context(), id, wait)
	} else {
		job, found = h.backfillService.Get(id)
	}
	if !found {
		apiutil.WriteError(w, http.StatusNotFound, constant.NotFoundStatusCode, "backfill job not found")
		return
	}
	apiutil.WriteSuccess(w, job)
}

func (h *Handler) SymbolMappings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		req, ok := parsePagination(w, r, &entity.SymbolMapping{})
		if !ok {
			return
		}
		result, err := h.dataService.ListSymbolMappings(r.Context(), req)
		writeDataResult(w, result, err)
	case http.MethodPost:
		if !requirePermission(w, r, constant.PermissionMarketConfigWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.CreateSymbolMapping(r.Context(), body)
		writeDataResult(w, result, err)
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) SymbolMappingByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "/api/v1/market/symbol-mappings/")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := h.dataService.GetSymbolMapping(r.Context(), id)
		writeDataResult(w, result, err)
	case http.MethodPut, http.MethodPatch:
		if !requirePermission(w, r, constant.PermissionMarketConfigWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.UpdateSymbolMapping(r.Context(), id, body)
		writeDataResult(w, result, err)
	case http.MethodDelete:
		if !requirePermission(w, r, constant.PermissionMarketConfigWrite) {
			return
		}
		writeDeleteResult(w, h.dataService.DeleteSymbolMapping(r.Context(), id))
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) KlineSubscriptions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		req, ok := parsePagination(w, r, &entity.KlineSubscription{})
		if !ok {
			return
		}
		result, err := h.dataService.ListKlineSubscriptions(r.Context(), req)
		writeDataResult(w, result, err)
	case http.MethodPost:
		if !requirePermission(w, r, constant.PermissionMarketConfigWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.CreateKlineSubscription(r.Context(), body)
		writeDataResult(w, result, err)
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) KlineSubscriptionByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "/api/v1/market/kline-subscriptions/")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := h.dataService.GetKlineSubscription(r.Context(), id)
		writeDataResult(w, result, err)
	case http.MethodPut, http.MethodPatch:
		if !requirePermission(w, r, constant.PermissionMarketConfigWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.UpdateKlineSubscription(r.Context(), id, body)
		writeDataResult(w, result, err)
	case http.MethodDelete:
		if !requirePermission(w, r, constant.PermissionMarketConfigWrite) {
			return
		}
		writeDeleteResult(w, h.dataService.DeleteKlineSubscription(r.Context(), id))
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) RecalculateMissingIndicators(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !requirePermission(w, r, constant.PermissionMarketConfigWrite) {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	apiutil.WriteSuccess(w, h.dataService.StartMissingIndicatorRecalculation(limit))
}

func (h *Handler) RecalculateMissingIndicatorJob(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "/api/v1/market/indicator-results/recalculate-missing/")
	if !ok {
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	wait := parseWaitDuration(r)
	var job *apiservice.IndicatorRecalculateJob
	var found bool
	if wait > 0 {
		job, found = h.dataService.WaitMissingIndicatorRecalculation(r.Context(), id, wait)
	} else {
		job, found = h.dataService.GetMissingIndicatorRecalculation(id)
	}
	if !found {
		apiutil.WriteError(w, http.StatusNotFound, constant.NotFoundStatusCode, "indicator recalculation job not found")
		return
	}
	apiutil.WriteSuccess(w, job)
}

func (h *Handler) IndicatorConfigs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		req, ok := parsePagination(w, r, &entity.IndicatorConfig{})
		if !ok {
			return
		}
		result, err := h.dataService.ListIndicatorConfigs(r.Context(), req)
		writeDataResult(w, result, err)
	case http.MethodPost:
		if !requirePermission(w, r, constant.PermissionMarketConfigWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.CreateIndicatorConfig(r.Context(), body)
		writeDataResult(w, result, err)
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) IndicatorConfigByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "/api/v1/market/indicator-configs/")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := h.dataService.GetIndicatorConfig(r.Context(), id)
		writeDataResult(w, result, err)
	case http.MethodPut, http.MethodPatch:
		if !requirePermission(w, r, constant.PermissionMarketConfigWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.UpdateIndicatorConfig(r.Context(), id, body)
		writeDataResult(w, result, err)
	case http.MethodDelete:
		if !requirePermission(w, r, constant.PermissionMarketConfigWrite) {
			return
		}
		writeDeleteResult(w, h.dataService.DeleteIndicatorConfig(r.Context(), id))
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) StrategyConfigs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		req, ok := parsePagination(w, r, &entity.StrategyConfig{})
		if !ok {
			return
		}
		result, err := h.dataService.ListStrategyConfigs(r.Context(), req)
		writeDataResult(w, result, err)
	case http.MethodPost:
		if !requirePermission(w, r, constant.PermissionStrategyConfigWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.CreateStrategyConfig(r.Context(), body)
		writeDataResult(w, result, err)
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) StrategyConfigByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "/api/v1/strategy/configs/")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := h.dataService.GetStrategyConfig(r.Context(), id)
		writeDataResult(w, result, err)
	case http.MethodPut, http.MethodPatch:
		if !requirePermission(w, r, constant.PermissionStrategyConfigWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.UpdateStrategyConfig(r.Context(), id, body)
		writeDataResult(w, result, err)
	case http.MethodDelete:
		if !requirePermission(w, r, constant.PermissionStrategyConfigWrite) {
			return
		}
		writeDeleteResult(w, h.dataService.DeleteStrategyConfig(r.Context(), id))
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) StrategyRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		req, ok := parsePagination(w, r, &entity.StrategyRule{})
		if !ok {
			return
		}
		result, err := h.dataService.ListStrategyRules(r.Context(), req)
		writeDataResult(w, result, err)
	case http.MethodPost:
		if !requirePermission(w, r, constant.PermissionStrategyConfigWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.CreateStrategyRule(r.Context(), body)
		writeDataResult(w, result, err)
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) StrategyRuleByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "/api/v1/strategy/rules/")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := h.dataService.GetStrategyRule(r.Context(), id)
		writeDataResult(w, result, err)
	case http.MethodPut, http.MethodPatch:
		if !requirePermission(w, r, constant.PermissionStrategyConfigWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.UpdateStrategyRule(r.Context(), id, body)
		writeDataResult(w, result, err)
	case http.MethodDelete:
		if !requirePermission(w, r, constant.PermissionStrategyConfigWrite) {
			return
		}
		writeDeleteResult(w, h.dataService.DeleteStrategyRule(r.Context(), id))
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) StrategyMonitors(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	result, err := h.dataService.ListStrategyMonitors(r.Context())
	writeDataResult(w, result, err)
}

func (h *Handler) StrategyMonitorByName(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/strategy/monitors/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || (parts[1] != "reset" && parts[1] != "restart") {
		apiutil.WriteError(w, http.StatusNotFound, constant.NotFoundStatusCode, "not found")
		return
	}
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !requirePermission(w, r, constant.PermissionStrategyConfigWrite) {
		return
	}
	result, err := h.dataService.StrategyMonitorAction(r.Context(), parts[0], parts[1])
	writeDataResult(w, result, err)
}

func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		req, ok := parsePagination(w, r, &entity.APISetting{})
		if !ok {
			return
		}
		result, err := h.dataService.ListSettings(r.Context(), req)
		writeDataResult(w, result, err)
	case http.MethodPost:
		if !requirePermission(w, r, constant.PermissionSettingsWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.CreateSetting(r.Context(), body)
		writeDataResult(w, result, err)
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) SettingByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "/api/v1/settings/")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := h.dataService.GetSetting(r.Context(), id)
		writeDataResult(w, result, err)
	case http.MethodPut, http.MethodPatch:
		if !requirePermission(w, r, constant.PermissionSettingsWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.UpdateSetting(r.Context(), id, body)
		writeDataResult(w, result, err)
	case http.MethodDelete:
		if !requirePermission(w, r, constant.PermissionSettingsWrite) {
			return
		}
		writeDeleteResult(w, h.dataService.DeleteSetting(r.Context(), id))
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) Users(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		req, ok := parsePagination(w, r, &entity.APIUser{})
		if !ok {
			return
		}
		result, err := h.dataService.ListUsers(r.Context(), req)
		writeDataResult(w, result, err)
	case http.MethodPost:
		if !requirePermission(w, r, constant.PermissionUserWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.CreateUser(r.Context(), body)
		writeDataResult(w, result, err)
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) UserByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "/api/v1/users/")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := h.dataService.GetUser(r.Context(), id)
		writeDataResult(w, result, err)
	case http.MethodPut, http.MethodPatch:
		if !requirePermission(w, r, constant.PermissionUserWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.UpdateUser(r.Context(), id, body)
		writeDataResult(w, result, err)
	case http.MethodDelete:
		if !requirePermission(w, r, constant.PermissionUserWrite) {
			return
		}
		writeDeleteResult(w, h.dataService.DeleteUser(r.Context(), id))
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) Roles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		req, ok := parsePagination(w, r, &entity.APIRole{})
		if !ok {
			return
		}
		result, err := h.dataService.ListRoles(r.Context(), req)
		writeDataResult(w, result, err)
	case http.MethodPost:
		if !requirePermission(w, r, constant.PermissionUserWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.CreateRole(r.Context(), body)
		writeDataResult(w, result, err)
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) RoleByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "/api/v1/roles/")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := h.dataService.GetRole(r.Context(), id)
		writeDataResult(w, result, err)
	case http.MethodPut, http.MethodPatch:
		if !requirePermission(w, r, constant.PermissionUserWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.UpdateRole(r.Context(), id, body)
		writeDataResult(w, result, err)
	case http.MethodDelete:
		if !requirePermission(w, r, constant.PermissionUserWrite) {
			return
		}
		writeDeleteResult(w, h.dataService.DeleteRole(r.Context(), id))
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) Permissions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		req, ok := parsePagination(w, r, &entity.APIPermission{})
		if !ok {
			return
		}
		result, err := h.dataService.ListPermissions(r.Context(), req)
		writeDataResult(w, result, err)
	case http.MethodPost:
		if !requirePermission(w, r, constant.PermissionPermissionWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.CreatePermission(r.Context(), body)
		writeDataResult(w, result, err)
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) PermissionByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "/api/v1/permissions/")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := h.dataService.GetPermission(r.Context(), id)
		writeDataResult(w, result, err)
	case http.MethodPut, http.MethodPatch:
		if !requirePermission(w, r, constant.PermissionPermissionWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.UpdatePermission(r.Context(), id, body)
		writeDataResult(w, result, err)
	case http.MethodDelete:
		if !requirePermission(w, r, constant.PermissionPermissionWrite) {
			return
		}
		writeDeleteResult(w, h.dataService.DeletePermission(r.Context(), id))
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) DashboardPages(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		req, ok := parsePagination(w, r, &entity.APIDashboardPage{})
		if !ok {
			return
		}
		result, err := h.dataService.ListDashboardPages(r.Context(), req)
		writeDataResult(w, result, err)
	case http.MethodPost:
		if !requirePermission(w, r, constant.PermissionDashboardPageWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.CreateDashboardPage(r.Context(), body)
		writeDataResult(w, result, err)
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) DashboardPageByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "/api/v1/dashboard/pages/")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := h.dataService.GetDashboardPage(r.Context(), id)
		writeDataResult(w, result, err)
	case http.MethodPut, http.MethodPatch:
		if !requirePermission(w, r, constant.PermissionDashboardPageWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.UpdateDashboardPage(r.Context(), id, body)
		writeDataResult(w, result, err)
	case http.MethodDelete:
		if !requirePermission(w, r, constant.PermissionDashboardPageWrite) {
			return
		}
		writeDeleteResult(w, h.dataService.DeleteDashboardPage(r.Context(), id))
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimSpace(r.Header.Get("Authorization"))
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))
		if raw == "" {
			apiutil.WriteError(w, http.StatusUnauthorized, constant.UnauthorizedStatusCode, "authorization token is required")
			return
		}
		claims, err := apiutil.VerifyToken(raw, h.authConfig.TokenSecret)
		if err != nil {
			apiutil.WriteError(w, http.StatusUnauthorized, constant.UnauthorizedStatusCode, "invalid authorization token")
			return
		}
		ctx := context.WithValue(r.Context(), claimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func (h *Handler) withPermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return h.withAuth(func(w http.ResponseWriter, r *http.Request) {
		claims := claimsFromContext(r.Context())
		if claims == nil || strings.TrimSpace(claims.Subject) == "" {
			apiutil.WriteError(w, http.StatusUnauthorized, constant.UnauthorizedStatusCode, "invalid authorization token")
			return
		}

		user, err := h.authService.Me(r.Context(), claims.Subject)
		if err != nil {
			writeAuthAccessError(w, err)
			return
		}

		claims.Roles = user.Roles
		claims.Permissions = user.Permissions
		ctx := context.WithValue(r.Context(), claimsContextKey, claims)
		r = r.WithContext(ctx)

		if !hasPermission(claims, permission) {
			apiutil.WriteError(w, http.StatusForbidden, constant.ForbiddenStatusCode, "permission denied")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func claimsFromContext(ctx context.Context) *apiutil.TokenClaims {
	claims, _ := ctx.Value(claimsContextKey).(*apiutil.TokenClaims)
	return claims
}

func hasPermission(claims *apiutil.TokenClaims, permission string) bool {
	if claims == nil {
		return false
	}
	for _, role := range claims.Roles {
		if role == "admin" {
			return true
		}
	}
	for _, v := range claims.Permissions {
		if v == permission {
			return true
		}
	}
	return false
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, constant.BadRequestStatusCode, "invalid json body")
		return false
	}
	return true
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
		return false
	}
	return true
}

func parsePagination(w http.ResponseWriter, r *http.Request, model apiutil.PaginationEntity) (*apiutil.PaginationReq, bool) {
	var req apiutil.PaginationReq
	if err := req.ParseFromQueryParam(apiutil.NewPaginationQueryParamReq(r.URL.Query()), model); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, constant.BadRequestStatusCode, "invalid query parameter")
		return nil, false
	}
	return &req, true
}

func parseOrderReportFilter(w http.ResponseWriter, r *http.Request) (entity.OrderReportFilter, bool) {
	values := r.URL.Query()
	filter := entity.OrderReportFilter{
		StrategyID: strings.TrimSpace(values.Get("strategy_id")),
		Symbol:     strings.TrimSpace(values.Get("symbol")),
		Page:       parsePositiveInt64(values.Get("page"), 1),
		Limit:      parsePositiveInt64(values.Get("limit"), 50),
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	if raw := strings.TrimSpace(values.Get("start_time")); raw != "" {
		parsed, err := parseReportTime(raw)
		if err != nil {
			apiutil.WriteError(w, http.StatusBadRequest, constant.BadRequestStatusCode, "invalid start_time")
			return filter, false
		}
		filter.StartTime = &parsed
	}
	if raw := strings.TrimSpace(values.Get("end_time")); raw != "" {
		parsed, err := parseReportTime(raw)
		if err != nil {
			apiutil.WriteError(w, http.StatusBadRequest, constant.BadRequestStatusCode, "invalid end_time")
			return filter, false
		}
		filter.EndTime = &parsed
	}
	return filter, true
}

func parseReportTime(raw string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04",
		"2006-01-02 15:04:05.999 -0700",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02",
	}
	var lastErr error
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return parsed, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

func parsePositiveInt64(raw string, fallback int64) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func parseMarketBackfillRequest(w http.ResponseWriter, body marketBackfillRequest) (apiservice.BackfillRequest, bool) {
	startTime, err := time.Parse(time.RFC3339, strings.TrimSpace(body.StartTime))
	if err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, constant.BadRequestStatusCode, "start_time must be RFC3339")
		return apiservice.BackfillRequest{}, false
	}
	endTime, err := time.Parse(time.RFC3339, strings.TrimSpace(body.EndTime))
	if err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, constant.BadRequestStatusCode, "end_time must be RFC3339")
		return apiservice.BackfillRequest{}, false
	}
	return apiservice.BackfillRequest{
		Exchange:   body.Exchange,
		MarketType: body.MarketType,
		Symbol:     body.Symbol,
		Interval:   body.Interval,
		StartTime:  startTime,
		EndTime:    endTime,
	}, true
}

func parseWaitDuration(r *http.Request) time.Duration {
	raw := strings.TrimSpace(r.URL.Query().Get("wait"))
	if raw == "" {
		return 0
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0
	}
	if seconds > 30 {
		seconds = 30
	}
	return time.Duration(seconds) * time.Second
}

func pathID(w http.ResponseWriter, r *http.Request, prefix string) (string, bool) {
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, prefix))
	if id == "" {
		apiutil.WriteError(w, http.StatusBadRequest, constant.BadRequestStatusCode, "id is required")
		return "", false
	}
	return id, true
}

func requirePermission(w http.ResponseWriter, r *http.Request, permission string) bool {
	if !hasPermission(claimsFromContext(r.Context()), permission) {
		apiutil.WriteError(w, http.StatusForbidden, constant.ForbiddenStatusCode, "permission denied")
		return false
	}
	return true
}

func writeDeleteResult(w http.ResponseWriter, err error) {
	if err != nil {
		writeDataResult(w, nil, err)
		return
	}
	apiutil.WriteSuccess(w, map[string]string{"status": "deleted"})
}

func writeAuthResult(w http.ResponseWriter, result *apiservice.AuthResult, err error) {
	if err == nil {
		apiutil.WriteSuccess(w, result)
		return
	}
	switch {
	case errors.Is(err, apiservice.ErrInvalidCredential), errors.Is(err, apiservice.ErrInvalidRefresh):
		apiutil.WriteError(w, http.StatusUnauthorized, constant.UnauthorizedStatusCode, err.Error())
	case errors.Is(err, apiservice.ErrInactiveUser), errors.Is(err, apiservice.ErrSetupAlreadyDone):
		apiutil.WriteError(w, http.StatusConflict, constant.ConflictStatusCode, err.Error())
	default:
		apiutil.WriteError(w, http.StatusInternalServerError, constant.InternalStatusCode, "internal server error")
	}
}

func writeAuthAccessError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		apiutil.WriteError(w, http.StatusUnauthorized, constant.UnauthorizedStatusCode, "invalid authorization token")
	case errors.Is(err, apiservice.ErrInactiveUser):
		apiutil.WriteError(w, http.StatusUnauthorized, constant.UnauthorizedStatusCode, err.Error())
	default:
		apiutil.WriteError(w, http.StatusInternalServerError, constant.InternalStatusCode, "internal server error")
	}
}

func writeDataResult(w http.ResponseWriter, result any, err error) {
	if err == nil {
		apiutil.WriteSuccess(w, result)
		return
	}
	switch {
	case errors.Is(err, sql.ErrNoRows):
		apiutil.WriteError(w, http.StatusNotFound, constant.NotFoundStatusCode, "data not found")
	default:
		apiutil.WriteError(w, http.StatusInternalServerError, constant.InternalStatusCode, "internal server error")
	}
}
