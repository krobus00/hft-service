package repository

import (
	"context"
	"database/sql"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/krobus00/hft-service/internal/entity"
)

type DashboardAuthRepository struct {
	db *sqlx.DB
}

func NewDashboardAuthRepository(db *sqlx.DB) *DashboardAuthRepository {
	return &DashboardAuthRepository{db: db}
}

func (r *DashboardAuthRepository) GetUserByUsername(ctx context.Context, username string) (*entity.DashboardUser, error) {
	var user entity.DashboardUser
	err := r.db.GetContext(ctx, &user, "SELECT * FROM dashboard_users WHERE username = $1", username)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *DashboardAuthRepository) GetUserByID(ctx context.Context, userID string) (*entity.DashboardUser, error) {
	var user entity.DashboardUser
	err := r.db.GetContext(ctx, &user, "SELECT * FROM dashboard_users WHERE id = $1", userID)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *DashboardAuthRepository) CreateRefreshToken(ctx context.Context, token *entity.DashboardRefreshToken) error {
	queryBuilder := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Insert(token.TableName()).
		Columns(
			"user_id",
			"token_hash",
			"expires_at",
			"created_at",
			"updated_at",
		).
		Values(
			token.UserID,
			token.TokenHash,
			token.ExpiresAt,
			token.CreatedAt,
			token.UpdatedAt,
		).
		Suffix("RETURNING id")

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return err
	}

	err = r.db.QueryRowContext(ctx, query, args...).Scan(&token.ID)
	return err
}

func (r *DashboardAuthRepository) GetActiveRefreshTokenByHash(ctx context.Context, tokenHash string) (*entity.DashboardRefreshToken, error) {
	var token entity.DashboardRefreshToken
	err := r.db.GetContext(
		ctx,
		&token,
		"SELECT * FROM dashboard_refresh_tokens WHERE token_hash = $1 AND revoked_at IS NULL",
		tokenHash,
	)
	if err != nil {
		return nil, err
	}

	return &token, nil
}

func (r *DashboardAuthRepository) RevokeRefreshToken(ctx context.Context, tokenID string, revokedAt time.Time) error {
	queryBuilder := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Update("dashboard_refresh_tokens").
		Set("revoked_at", sql.NullTime{Time: revokedAt, Valid: true}).
		Set("updated_at", revokedAt).
		Where(sq.Eq{"id": tokenID})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	return err
}
