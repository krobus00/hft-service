package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/constant"
	"github.com/krobus00/hft-service/internal/entity"
	apiservice "github.com/krobus00/hft-service/internal/service/api"
)

type contextKey string

const claimsContextKey contextKey = "claims"

type Handler struct {
	authService *apiservice.AuthService
	dataService *apiservice.DataService
	authConfig  config.DashboardAuthConfig
}

func NewAPIHTTPHandler(authService *apiservice.AuthService, dataService *apiservice.DataService, authConfig config.DashboardAuthConfig) *Handler {
	return &Handler{authService: authService, dataService: dataService, authConfig: authConfig}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/auth/setup", h.Setup)
	mux.HandleFunc("/api/v1/auth/login", h.Login)
	mux.HandleFunc("/api/v1/auth/config", h.AuthConfig)
	mux.HandleFunc("/api/v1/auth/refresh", h.Refresh)
	mux.HandleFunc("/api/v1/auth/logout", h.withAuth(h.Logout))
	mux.HandleFunc("/api/v1/auth/me", h.withAuth(h.Me))
	mux.HandleFunc("/api/v1/form/enums", h.withAuth(h.FormEnums))

	mux.HandleFunc("/api/v1/orders", h.withPermission(constant.PermissionOrderRead, h.Orders))
	mux.HandleFunc("/api/v1/orders/", h.withPermission(constant.PermissionOrderRead, h.OrderByID))
	mux.HandleFunc("/api/v1/market/klines", h.withPermission(constant.PermissionMarketRead, h.MarketKlines))
	mux.HandleFunc("/api/v1/market/klines/", h.withPermission(constant.PermissionMarketRead, h.MarketKlineByID))
	mux.HandleFunc("/api/v1/market/symbol-mappings", h.withPermission(constant.PermissionMarketRead, h.SymbolMappings))
	mux.HandleFunc("/api/v1/market/symbol-mappings/", h.withPermission(constant.PermissionMarketRead, h.SymbolMappingByID))
	mux.HandleFunc("/api/v1/market/kline-subscriptions", h.withPermission(constant.PermissionMarketRead, h.KlineSubscriptions))
	mux.HandleFunc("/api/v1/market/kline-subscriptions/", h.withPermission(constant.PermissionMarketRead, h.KlineSubscriptionByID))
	mux.HandleFunc("/api/v1/strategy/configs", h.withPermission(constant.PermissionStrategyConfigRead, h.StrategyConfigs))
	mux.HandleFunc("/api/v1/strategy/configs/", h.withPermission(constant.PermissionStrategyConfigRead, h.StrategyConfigByID))
	mux.HandleFunc("/api/v1/settings", h.withPermission(constant.PermissionSettingsRead, h.Settings))
	mux.HandleFunc("/api/v1/settings/", h.withPermission(constant.PermissionSettingsRead, h.SettingByID))
	mux.HandleFunc("/api/v1/users", h.withPermission(constant.PermissionUserRead, h.Users))
	mux.HandleFunc("/api/v1/users/", h.withPermission(constant.PermissionUserRead, h.UserByID))
	mux.HandleFunc("/api/v1/roles", h.withPermission(constant.PermissionUserRead, h.Roles))
	mux.HandleFunc("/api/v1/roles/", h.withPermission(constant.PermissionUserRead, h.RoleByID))
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
		result, err := h.dataService.CreateOrder(r.Context(), body)
		writeDataResult(w, result, err)
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) OrderByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "/api/v1/orders/")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := h.dataService.GetOrder(r.Context(), id)
		writeDataResult(w, result, err)
	case http.MethodPut, http.MethodPatch:
		if !requirePermission(w, r, constant.PermissionOrderWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.UpdateOrder(r.Context(), id, body)
		writeDataResult(w, result, err)
	case http.MethodDelete:
		if !requirePermission(w, r, constant.PermissionOrderWrite) {
			return
		}
		writeDeleteResult(w, h.dataService.DeleteOrder(r.Context(), id))
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) MarketKlines(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		req, ok := parsePagination(w, r, &entity.MarketKline{})
		if !ok {
			return
		}
		result, err := h.dataService.ListMarketKlines(r.Context(), req)
		writeDataResult(w, result, err)
	case http.MethodPost:
		if !requirePermission(w, r, constant.PermissionMarketWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.CreateMarketKline(r.Context(), body)
		writeDataResult(w, result, err)
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
}

func (h *Handler) MarketKlineByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "/api/v1/market/klines/")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := h.dataService.GetMarketKline(r.Context(), id)
		writeDataResult(w, result, err)
	case http.MethodPut, http.MethodPatch:
		if !requirePermission(w, r, constant.PermissionMarketWrite) {
			return
		}
		var body map[string]any
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := h.dataService.UpdateMarketKline(r.Context(), id, body)
		writeDataResult(w, result, err)
	case http.MethodDelete:
		if !requirePermission(w, r, constant.PermissionMarketWrite) {
			return
		}
		writeDeleteResult(w, h.dataService.DeleteMarketKline(r.Context(), id))
	default:
		apiutil.WriteError(w, http.StatusMethodNotAllowed, constant.BadRequestStatusCode, "method not allowed")
	}
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
