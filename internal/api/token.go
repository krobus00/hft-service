package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrInvalidToken = errors.New("invalid token")

type TokenClaims struct {
	Subject     string   `json:"sub"`
	Issuer      string   `json:"iss"`
	ExpiresAt   int64    `json:"exp"`
	IssuedAt    int64    `json:"iat"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

func SignToken(claims TokenClaims, secret string) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := sign(encodedPayload, secret)
	return encodedPayload + "." + signature, nil
}

func VerifyToken(rawToken string, secret string) (*TokenClaims, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 2 {
		return nil, ErrInvalidToken
	}

	expected := sign(parts[0], secret)
	if hmac.Equal([]byte(expected), []byte(parts[1])) == false {
		return nil, ErrInvalidToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidToken
	}
	if claims.ExpiresAt <= time.Now().Unix() {
		return nil, ErrInvalidToken
	}

	return &claims, nil
}

func NewRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func sign(payload string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
