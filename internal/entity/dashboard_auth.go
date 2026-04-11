package entity

import (
	"database/sql"
	"time"
)

type DashboardRole string

const (
	DashboardRoleAdmin DashboardRole = "admin"
	DashboardRoleUser  DashboardRole = "user"
)

type DashboardUser struct {
	ID           string    `db:"id" json:"id"`
	Username     string    `db:"username" json:"username"`
	PasswordHash string    `db:"password_hash" json:"-"`
	Role         string    `db:"role" json:"role"`
	IsActive     bool      `db:"is_active" json:"is_active"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}

func (DashboardUser) TableName() string {
	return "dashboard_users"
}

type DashboardRefreshToken struct {
	ID        string       `db:"id" json:"id"`
	UserID    string       `db:"user_id" json:"user_id"`
	TokenHash string       `db:"token_hash" json:"-"`
	ExpiresAt time.Time    `db:"expires_at" json:"expires_at"`
	RevokedAt sql.NullTime `db:"revoked_at" json:"revoked_at"`
	CreatedAt time.Time    `db:"created_at" json:"created_at"`
	UpdatedAt time.Time    `db:"updated_at" json:"updated_at"`
}

func (DashboardRefreshToken) TableName() string {
	return "dashboard_refresh_tokens"
}
