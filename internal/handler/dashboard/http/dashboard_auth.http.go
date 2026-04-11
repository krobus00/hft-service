package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/krobus00/hft-service/internal/service/dashboard"
)

type DashboardAuthHandler struct {
	authService *dashboard.DashboardAuthService
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type authResponse struct {
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	TokenType             string `json:"token_type"`
	AccessTokenExpiresIn  int64  `json:"access_token_expires_in"`
	RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
	Role                  string `json:"role"`
	UserID                string `json:"user_id"`
	Username              string `json:"username"`
}

func NewDashboardAuthHTTPHandler(authService *dashboard.DashboardAuthService) *DashboardAuthHandler {
	return &DashboardAuthHandler{authService: authService}
}

func (h *DashboardAuthHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/dashboard/v1/auth/login", h.Login)
	mux.HandleFunc("/dashboard/v1/auth/refresh", h.Refresh)
	mux.HandleFunc("/dashboard/v1/auth/me", h.Me)
	mux.HandleFunc("/dashboard/v1/admin/ping", h.AdminPing)
}

func (h *DashboardAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	defer r.Body.Close()

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	user, pair, err := h.authService.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		handleAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, authResponse{
		AccessToken:           pair.AccessToken,
		RefreshToken:          pair.RefreshToken,
		TokenType:             "Bearer",
		AccessTokenExpiresIn:  int64(pair.AccessTTL.Seconds()),
		RefreshTokenExpiresIn: int64(pair.RefreshTTL.Seconds()),
		Role:                  user.Role,
		UserID:                user.ID,
		Username:              user.Username,
	})
}

func (h *DashboardAuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	defer r.Body.Close()

	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	user, pair, err := h.authService.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		handleAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, authResponse{
		AccessToken:           pair.AccessToken,
		RefreshToken:          pair.RefreshToken,
		TokenType:             "Bearer",
		AccessTokenExpiresIn:  int64(pair.AccessTTL.Seconds()),
		RefreshTokenExpiresIn: int64(pair.RefreshTTL.Seconds()),
		Role:                  user.Role,
		UserID:                user.ID,
		Username:              user.Username,
	})
}

func (h *DashboardAuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	claims, err := h.authenticateRequest(r)
	if err != nil {
		handleAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": claims.Sub,
		"role":    claims.Role,
	})
}

func (h *DashboardAuthHandler) AdminPing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	claims, err := h.authenticateRequest(r)
	if err != nil {
		handleAuthError(w, err)
		return
	}

	if err := h.authService.AuthorizeRole(claims, "admin"); err != nil {
		handleAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "dashboard admin authenticated",
		"role":    claims.Role,
	})
}

func (h *DashboardAuthHandler) authenticateRequest(r *http.Request) (*dashboard.TokenClaims, error) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		return nil, dashboard.ErrDashboardTokenInvalid
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, dashboard.ErrDashboardTokenInvalid
	}

	return h.authService.ValidateAccessToken(parts[1])
}

func handleAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, dashboard.ErrDashboardInvalidCredential), errors.Is(err, dashboard.ErrDashboardTokenInvalid), errors.Is(err, dashboard.ErrDashboardTokenTypeInvalid), errors.Is(err, dashboard.ErrDashboardTokenExpired):
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
	case errors.Is(err, dashboard.ErrDashboardUserInactive):
		writeJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})
	case errors.Is(err, dashboard.ErrDashboardForbiddenRole):
		writeJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
