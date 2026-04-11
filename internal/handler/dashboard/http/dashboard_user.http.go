package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/infrastructure"
	"github.com/krobus00/hft-service/internal/service/dashboard"
)

type DashboardUserHandler struct {
	authService *dashboard.DashboardAuthService
	userService *dashboard.DashboardUserService
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
	IsActive *bool  `json:"is_active"`
}

type updateUserRequest struct {
	Username *string `json:"username"`
	Password *string `json:"password"`
	Role     *string `json:"role"`
	IsActive *bool   `json:"is_active"`
}

type dashboardUserResponse struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	IsActive  bool   `json:"is_active"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

func NewDashboardUserHTTPHandler(authService *dashboard.DashboardAuthService, userService *dashboard.DashboardUserService) *DashboardUserHandler {
	return &DashboardUserHandler{authService: authService, userService: userService}
}

func (h *DashboardUserHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/dashboard/v1/users", h.Users)
	mux.HandleFunc("/dashboard/v1/users/", h.UserByID)
}

func (h *DashboardUserHandler) Users(w http.ResponseWriter, r *http.Request) {
	claims, err := h.authorizeAdmin(r)
	if err != nil {
		handleDashboardUserError(w, err)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.listUsers(w, r)
	case http.MethodPost:
		h.createUser(w, r, claims)
	default:
		infrastructure.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (h *DashboardUserHandler) UserByID(w http.ResponseWriter, r *http.Request) {
	claims, err := h.authorizeAdmin(r)
	if err != nil {
		handleDashboardUserError(w, err)
		return
	}

	userID := strings.TrimPrefix(r.URL.Path, "/dashboard/v1/users/")
	if strings.TrimSpace(userID) == "" {
		infrastructure.WriteError(w, http.StatusBadRequest, "INVALID_USER_ID", "invalid user id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getUser(w, r, userID)
	case http.MethodPatch:
		h.updateUser(w, r, claims.Sub, userID)
	case http.MethodDelete:
		h.deleteUser(w, r, claims.Sub, userID)
	default:
		infrastructure.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (h *DashboardUserHandler) listUsers(w http.ResponseWriter, r *http.Request) {
	pagination := infrastructure.ParsePaginationRequest(r)
	search := strings.TrimSpace(r.URL.Query().Get("search"))

	users, total, err := h.userService.ListUsers(r.Context(), dashboard.ListUsersInput{
		Search:   search,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	})
	if err != nil {
		handleDashboardUserError(w, err)
		return
	}

	items := make([]dashboardUserResponse, 0, len(users))
	for _, user := range users {
		items = append(items, mapDashboardUserResponse(user))
	}

	infrastructure.WriteSuccess(w, http.StatusOK, infrastructure.PaginatedData[dashboardUserResponse]{
		Items:      items,
		Pagination: infrastructure.NewPaginationResponse(pagination, total),
	}, "users fetched")
}

func (h *DashboardUserHandler) createUser(w http.ResponseWriter, r *http.Request, _ *dashboard.TokenClaims) {
	defer r.Body.Close()

	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		infrastructure.WriteError(w, http.StatusBadRequest, "INVALID_JSON_BODY", "invalid json body")
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	user, err := h.userService.CreateUser(r.Context(), dashboard.CreateUserInput{
		Username: req.Username,
		Password: req.Password,
		Role:     req.Role,
		IsActive: isActive,
	})
	if err != nil {
		handleDashboardUserError(w, err)
		return
	}

	infrastructure.WriteSuccess(w, http.StatusCreated, mapDashboardUserResponse(*user), "user created")
}

func (h *DashboardUserHandler) getUser(w http.ResponseWriter, r *http.Request, userID string) {
	user, err := h.userService.GetUserByID(r.Context(), userID)
	if err != nil {
		handleDashboardUserError(w, err)
		return
	}

	infrastructure.WriteSuccess(w, http.StatusOK, mapDashboardUserResponse(*user), "user fetched")
}

func (h *DashboardUserHandler) updateUser(w http.ResponseWriter, r *http.Request, actorUserID string, userID string) {
	defer r.Body.Close()

	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		infrastructure.WriteError(w, http.StatusBadRequest, "INVALID_JSON_BODY", "invalid json body")
		return
	}

	user, err := h.userService.UpdateUser(r.Context(), actorUserID, userID, dashboard.UpdateUserInput{
		Username: req.Username,
		Password: req.Password,
		Role:     req.Role,
		IsActive: req.IsActive,
	})
	if err != nil {
		handleDashboardUserError(w, err)
		return
	}

	infrastructure.WriteSuccess(w, http.StatusOK, mapDashboardUserResponse(*user), "user updated")
}

func (h *DashboardUserHandler) deleteUser(w http.ResponseWriter, r *http.Request, actorUserID string, userID string) {
	if err := h.userService.DeleteUser(r.Context(), actorUserID, userID); err != nil {
		handleDashboardUserError(w, err)
		return
	}

	infrastructure.WriteSuccess(w, http.StatusOK, map[string]any{"id": userID}, "user deleted")
}

func (h *DashboardUserHandler) authorizeAdmin(r *http.Request) (*dashboard.TokenClaims, error) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		return nil, dashboard.ErrDashboardTokenInvalid
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, dashboard.ErrDashboardTokenInvalid
	}

	claims, err := h.authService.ValidateAccessToken(parts[1])
	if err != nil {
		return nil, err
	}

	if err := h.authService.AuthorizeRole(claims, string(entity.DashboardRoleAdmin)); err != nil {
		return nil, err
	}

	return claims, nil
}

func mapDashboardUserResponse(user entity.DashboardUser) dashboardUserResponse {
	return dashboardUserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		IsActive:  user.IsActive,
		CreatedAt: user.CreatedAt.UnixMilli(),
		UpdatedAt: user.UpdatedAt.UnixMilli(),
	}
}

func handleDashboardUserError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, dashboard.ErrDashboardTokenInvalid), errors.Is(err, dashboard.ErrDashboardTokenTypeInvalid), errors.Is(err, dashboard.ErrDashboardTokenExpired):
		infrastructure.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
	case errors.Is(err, dashboard.ErrDashboardForbiddenRole):
		infrastructure.WriteError(w, http.StatusForbidden, "FORBIDDEN_ROLE", err.Error())
	case errors.Is(err, dashboard.ErrDashboardUserNotFound):
		infrastructure.WriteError(w, http.StatusNotFound, "USER_NOT_FOUND", err.Error())
	case errors.Is(err, dashboard.ErrDashboardUserConflict):
		infrastructure.WriteError(w, http.StatusConflict, "USER_CONFLICT", err.Error())
	case errors.Is(err, dashboard.ErrDashboardUserInvalidInput):
		infrastructure.WriteError(w, http.StatusBadRequest, "INVALID_USER_INPUT", err.Error())
	case errors.Is(err, dashboard.ErrDashboardUserCannotDelete), errors.Is(err, dashboard.ErrDashboardUserCannotDisable):
		infrastructure.WriteError(w, http.StatusBadRequest, "INVALID_USER_OPERATION", err.Error())
	default:
		infrastructure.WriteError(w, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "internal server error")
	}
}
