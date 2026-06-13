package api

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredential = errors.New("invalid credential")
	ErrInactiveUser      = errors.New("user is inactive")
	ErrSetupAlreadyDone  = errors.New("setup already done")
	ErrInvalidRefresh    = errors.New("invalid refresh token")
)

type AuthService struct {
	repo  *repository.APIAuthRepository
	cache *AuthCache
	cfg   config.DashboardAuthConfig
}

type AuthTokens struct {
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	TokenType             string `json:"token_type"`
	ExpiresIn             int64  `json:"expires_in"`
	RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
}

type AuthUser struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	Name        string   `json:"name"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

type AuthResult struct {
	User   AuthUser   `json:"user"`
	Tokens AuthTokens `json:"tokens"`
}

func NewAuthService(repo *repository.APIAuthRepository, cfg config.DashboardAuthConfig, cache ...*AuthCache) *AuthService {
	if cfg.Issuer == "" {
		cfg.Issuer = "hft-dashboard"
	}
	if cfg.AccessTokenTTL <= 0 {
		cfg.AccessTokenTTL = 15 * time.Minute
	}
	if cfg.RefreshTokenTTL <= 0 {
		cfg.RefreshTokenTTL = 7 * 24 * time.Hour
	}
	var authCache *AuthCache
	if len(cache) > 0 {
		authCache = cache[0]
	}
	return &AuthService{repo: repo, cache: authCache, cfg: cfg}
}

func (s *AuthService) Setup(ctx context.Context, email, name, password string) (*AuthResult, error) {
	total, err := s.repo.CountUsers(ctx)
	if err != nil {
		return nil, err
	}
	if total > 0 {
		return nil, ErrSetupAlreadyDone
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}

	user := &entity.APIUser{
		Email:        strings.ToLower(strings.TrimSpace(email)),
		Name:         strings.TrimSpace(name),
		PasswordHash: passwordHash,
		Active:       true,
	}
	if err := s.repo.CreateUser(ctx, user, []string{"admin"}); err != nil {
		return nil, err
	}

	result, err := s.issue(ctx, user)
	if err != nil {
		return nil, err
	}
	_ = s.cache.SetUser(ctx, &result.User)
	return result, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	user, err := s.repo.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidCredential
		}
		return nil, err
	}
	if !user.Active {
		return nil, ErrInactiveUser
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredential
	}
	if err := s.repo.UpdateLastLogin(ctx, user.ID); err != nil {
		return nil, err
	}
	_ = s.cache.DeleteUser(ctx, user.ID)
	result, err := s.issue(ctx, user)
	if err != nil {
		return nil, err
	}
	_ = s.cache.SetUser(ctx, &result.User)
	return result, nil
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*AuthResult, error) {
	tokenHash := apiutil.HashToken(strings.TrimSpace(refreshToken))
	session, err := s.cache.GetRefreshSession(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, errAuthCacheMiss) {
			return nil, ErrInvalidRefresh
		}
		return nil, err
	}
	if session.User.ID == "" {
		return nil, ErrInvalidRefresh
	}

	_ = s.cache.DeleteRefreshSession(ctx, tokenHash)
	result, err := s.issueFromAuthUser(ctx, &session.User)
	if err != nil {
		return nil, err
	}
	_ = s.cache.SetUser(ctx, &result.User)
	return result, nil
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return ErrInvalidRefresh
	}
	return s.cache.DeleteRefreshSession(ctx, apiutil.HashToken(refreshToken))
}

func (s *AuthService) Me(ctx context.Context, userID string) (*AuthUser, error) {
	if cachedUser, err := s.cache.GetUser(ctx, strings.TrimSpace(userID)); err == nil {
		return cachedUser, nil
	}

	user, err := s.repo.GetUserByID(ctx, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	if !user.Active {
		return nil, ErrInactiveUser
	}

	roles, err := s.repo.ListRoleNamesByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	permissions, err := s.repo.ListPermissionNamesByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	authUser := &AuthUser{
		ID:          user.ID,
		Email:       user.Email,
		Name:        user.Name,
		Roles:       roles,
		Permissions: permissions,
	}
	_ = s.cache.SetUser(ctx, authUser)
	return authUser, nil
}

func (s *AuthService) issue(ctx context.Context, user *entity.APIUser) (*AuthResult, error) {
	roles, err := s.repo.ListRoleNamesByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	permissions, err := s.repo.ListPermissionNamesByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	accessExpiresAt := now.Add(s.cfg.AccessTokenTTL)
	accessToken, err := apiutil.SignToken(apiutil.TokenClaims{
		Subject:     user.ID,
		Issuer:      s.cfg.Issuer,
		IssuedAt:    now.Unix(),
		ExpiresAt:   accessExpiresAt.Unix(),
		Roles:       roles,
		Permissions: permissions,
	}, s.cfg.TokenSecret)
	if err != nil {
		return nil, err
	}

	refreshToken, err := apiutil.NewRandomToken()
	if err != nil {
		return nil, err
	}

	authUser := AuthUser{
		ID:          user.ID,
		Email:       user.Email,
		Name:        user.Name,
		Roles:       roles,
		Permissions: permissions,
	}
	if err := s.cache.SetRefreshSession(ctx, apiutil.HashToken(refreshToken), &RefreshSession{User: authUser}, s.cfg.RefreshTokenTTL); err != nil {
		return nil, err
	}

	return &AuthResult{
		User: authUser,
		Tokens: AuthTokens{
			AccessToken:           accessToken,
			RefreshToken:          refreshToken,
			TokenType:             "Bearer",
			ExpiresIn:             int64(s.cfg.AccessTokenTTL.Seconds()),
			RefreshTokenExpiresIn: int64(s.cfg.RefreshTokenTTL.Seconds()),
		},
	}, nil
}

func (s *AuthService) issueFromAuthUser(ctx context.Context, user *AuthUser) (*AuthResult, error) {
	now := time.Now()
	accessExpiresAt := now.Add(s.cfg.AccessTokenTTL)
	accessToken, err := apiutil.SignToken(apiutil.TokenClaims{
		Subject:     user.ID,
		Issuer:      s.cfg.Issuer,
		IssuedAt:    now.Unix(),
		ExpiresAt:   accessExpiresAt.Unix(),
		Roles:       user.Roles,
		Permissions: user.Permissions,
	}, s.cfg.TokenSecret)
	if err != nil {
		return nil, err
	}

	refreshToken, err := apiutil.NewRandomToken()
	if err != nil {
		return nil, err
	}
	if err := s.cache.SetRefreshSession(ctx, apiutil.HashToken(refreshToken), &RefreshSession{User: *user}, s.cfg.RefreshTokenTTL); err != nil {
		return nil, err
	}

	return &AuthResult{
		User: *user,
		Tokens: AuthTokens{
			AccessToken:           accessToken,
			RefreshToken:          refreshToken,
			TokenType:             "Bearer",
			ExpiresIn:             int64(s.cfg.AccessTokenTTL.Seconds()),
			RefreshTokenExpiresIn: int64(s.cfg.RefreshTokenTTL.Seconds()),
		},
	}, nil
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
