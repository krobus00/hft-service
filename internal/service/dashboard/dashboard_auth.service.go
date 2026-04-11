package dashboard

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrDashboardInvalidCredential = errors.New("invalid credentials")
	ErrDashboardUserInactive      = errors.New("user inactive")
	ErrDashboardTokenInvalid      = errors.New("invalid token")
	ErrDashboardTokenExpired      = errors.New("token expired")
	ErrDashboardTokenTypeInvalid  = errors.New("invalid token type")
	ErrDashboardForbiddenRole     = errors.New("forbidden role")
)

type DashboardAuthService struct {
	repo *repository.DashboardAuthRepository
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
}

var (
	fallbackSecret     string
	fallbackSecretOnce sync.Once
)

type TokenClaims struct {
	Sub  string `json:"sub"`
	Role string `json:"role"`
	Type string `json:"type"`
	Iss  string `json:"iss"`
	Exp  int64  `json:"exp"`
	Iat  int64  `json:"iat"`
}

func NewDashboardAuthService(repo *repository.DashboardAuthRepository) *DashboardAuthService {
	return &DashboardAuthService{repo: repo}
}

func (s *DashboardAuthService) Login(ctx context.Context, username string, password string) (*entity.DashboardUser, *TokenPair, error) {
	user, err := s.repo.GetUserByUsername(ctx, strings.TrimSpace(username))
	if err != nil {
		return nil, nil, ErrDashboardInvalidCredential
	}

	if !user.IsActive {
		return nil, nil, ErrDashboardUserInactive
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, nil, ErrDashboardInvalidCredential
	}

	pair, err := s.issueTokenPair(ctx, user)
	if err != nil {
		return nil, nil, err
	}

	return user, pair, nil
}

func (s *DashboardAuthService) Refresh(ctx context.Context, refreshToken string) (*entity.DashboardUser, *TokenPair, error) {
	claims, err := validateSignedToken(refreshToken, resolveDashboardSecret())
	if err != nil {
		return nil, nil, err
	}

	if claims.Type != "refresh" {
		return nil, nil, ErrDashboardTokenTypeInvalid
	}

	now := time.Now().UTC()
	if now.Unix() >= claims.Exp {
		return nil, nil, ErrDashboardTokenExpired
	}

	tokenHash := hashToken(refreshToken)
	storedToken, err := s.repo.GetActiveRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		return nil, nil, ErrDashboardTokenInvalid
	}

	if storedToken.ExpiresAt.Before(now) {
		_ = s.repo.RevokeRefreshToken(ctx, storedToken.ID, now)
		return nil, nil, ErrDashboardTokenExpired
	}

	user, err := s.repo.GetUserByID(ctx, claims.Sub)
	if err != nil {
		return nil, nil, ErrDashboardTokenInvalid
	}

	if !user.IsActive {
		return nil, nil, ErrDashboardUserInactive
	}

	if err := s.repo.RevokeRefreshToken(ctx, storedToken.ID, now); err != nil {
		return nil, nil, err
	}

	pair, err := s.issueTokenPair(ctx, user)
	if err != nil {
		return nil, nil, err
	}

	return user, pair, nil
}

func (s *DashboardAuthService) ValidateAccessToken(accessToken string) (*TokenClaims, error) {
	claims, err := validateSignedToken(accessToken, resolveDashboardSecret())
	if err != nil {
		return nil, err
	}

	if claims.Type != "access" {
		return nil, ErrDashboardTokenTypeInvalid
	}

	if time.Now().UTC().Unix() >= claims.Exp {
		return nil, ErrDashboardTokenExpired
	}

	return claims, nil
}

func (s *DashboardAuthService) AuthorizeRole(claims *TokenClaims, roles ...string) error {
	if claims == nil {
		return ErrDashboardTokenInvalid
	}

	if len(roles) == 0 {
		return nil
	}

	for _, role := range roles {
		if strings.EqualFold(strings.TrimSpace(role), strings.TrimSpace(claims.Role)) {
			return nil
		}
	}

	return ErrDashboardForbiddenRole
}

func (s *DashboardAuthService) issueTokenPair(ctx context.Context, user *entity.DashboardUser) (*TokenPair, error) {
	now := time.Now().UTC()
	accessTTL := resolveAccessTTL()
	refreshTTL := resolveRefreshTTL()

	accessClaims := TokenClaims{
		Sub:  user.ID,
		Role: user.Role,
		Type: "access",
		Iss:  resolveDashboardIssuer(),
		Exp:  now.Add(accessTTL).Unix(),
		Iat:  now.Unix(),
	}

	refreshClaims := TokenClaims{
		Sub:  user.ID,
		Role: user.Role,
		Type: "refresh",
		Iss:  resolveDashboardIssuer(),
		Exp:  now.Add(refreshTTL).Unix(),
		Iat:  now.Unix(),
	}

	accessToken, err := signToken(accessClaims, resolveDashboardSecret())
	if err != nil {
		return nil, err
	}

	refreshToken, err := signToken(refreshClaims, resolveDashboardSecret())
	if err != nil {
		return nil, err
	}

	err = s.repo.CreateRefreshToken(ctx, &entity.DashboardRefreshToken{
		UserID:    user.ID,
		TokenHash: hashToken(refreshToken),
		ExpiresAt: now.Add(refreshTTL),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		AccessTTL:    accessTTL,
		RefreshTTL:   refreshTTL,
	}, nil
}

func signToken(claims TokenClaims, secret string) (string, error) {
	headerPart := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	payloadPart := base64.RawURLEncoding.EncodeToString(payloadBytes)
	unsigned := fmt.Sprintf("%s.%s", headerPart, payloadPart)

	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(unsigned))
	sig := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	return fmt.Sprintf("%s.%s", unsigned, sig), nil
}

func validateSignedToken(token string, secret string) (*TokenClaims, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return nil, ErrDashboardTokenInvalid
	}

	unsigned := parts[0] + "." + parts[1]
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(unsigned))
	expectedSig := h.Sum(nil)

	receivedSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrDashboardTokenInvalid
	}

	if subtle.ConstantTimeCompare(expectedSig, receivedSig) != 1 {
		return nil, ErrDashboardTokenInvalid
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrDashboardTokenInvalid
	}

	var claims TokenClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, ErrDashboardTokenInvalid
	}

	if strings.TrimSpace(claims.Sub) == "" || strings.TrimSpace(claims.Type) == "" {
		return nil, ErrDashboardTokenInvalid
	}

	return &claims, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func resolveDashboardSecret() string {
	if config.Env != nil {
		secret := strings.TrimSpace(config.Env.DashboardAuth.TokenSecret)
		if secret != "" {
			return secret
		}
	}

	fallbackSecretOnce.Do(func() {
		bytes := make([]byte, 32)
		_, _ = rand.Read(bytes)
		fallbackSecret = hex.EncodeToString(bytes)
	})

	return fallbackSecret
}

func resolveDashboardIssuer() string {
	if config.Env != nil && strings.TrimSpace(config.Env.DashboardAuth.Issuer) != "" {
		return strings.TrimSpace(config.Env.DashboardAuth.Issuer)
	}

	return "hft-dashboard"
}

func resolveAccessTTL() time.Duration {
	if config.Env != nil && config.Env.DashboardAuth.AccessTokenTTL > 0 {
		return config.Env.DashboardAuth.AccessTokenTTL
	}

	return 15 * time.Minute
}

func resolveRefreshTTL() time.Duration {
	if config.Env != nil && config.Env.DashboardAuth.RefreshTokenTTL > 0 {
		return config.Env.DashboardAuth.RefreshTokenTTL
	}

	return 7 * 24 * time.Hour
}
