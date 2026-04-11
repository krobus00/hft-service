package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/krobus00/hft-service/internal/entity"
)

type DashboardAuthRepository struct {
	db *sqlx.DB
}

type DashboardUserListFilter struct {
	Search   string
	Page     int
	PageSize int
}

type DashboardUserPatch struct {
	Username     *string
	PasswordHash *string
	Role         *string
	IsActive     *bool
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

func (r *DashboardAuthRepository) ListUsers(ctx context.Context, filter DashboardUserListFilter) ([]entity.DashboardUser, int64, error) {
	search := strings.TrimSpace(filter.Search)
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 10
	}

	whereClause := ""
	args := []any{}
	if search != "" {
		whereClause = " WHERE username ILIKE $1"
		args = append(args, "%"+search+"%")
	}

	countQuery := "SELECT COUNT(*) FROM dashboard_users" + whereClause
	var total int64
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	listQuery := "SELECT * FROM dashboard_users" + whereClause + fmt.Sprintf(" ORDER BY created_at DESC OFFSET $%d LIMIT $%d", len(args)+1, len(args)+2)
	args = append(args, offset, filter.PageSize)

	items := make([]entity.DashboardUser, 0)
	if err := r.db.SelectContext(ctx, &items, listQuery, args...); err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *DashboardAuthRepository) CreateUser(ctx context.Context, user *entity.DashboardUser) error {
	queryBuilder := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Insert(user.TableName()).
		Columns(
			"username",
			"password_hash",
			"role",
			"is_active",
			"created_at",
			"updated_at",
		).
		Values(
			user.Username,
			user.PasswordHash,
			user.Role,
			user.IsActive,
			user.CreatedAt,
			user.UpdatedAt,
		).
		Suffix("RETURNING id")

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return err
	}

	return r.db.QueryRowContext(ctx, query, args...).Scan(&user.ID)
}

func (r *DashboardAuthRepository) UpdateUser(ctx context.Context, userID string, patch DashboardUserPatch, updatedAt time.Time) error {
	queryBuilder := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Update("dashboard_users").
		Set("updated_at", updatedAt).
		Where(sq.Eq{"id": userID})

	if patch.Username != nil {
		queryBuilder = queryBuilder.Set("username", *patch.Username)
	}

	if patch.PasswordHash != nil {
		queryBuilder = queryBuilder.Set("password_hash", *patch.PasswordHash)
	}

	if patch.Role != nil {
		queryBuilder = queryBuilder.Set("role", *patch.Role)
	}

	if patch.IsActive != nil {
		queryBuilder = queryBuilder.Set("is_active", *patch.IsActive)
	}

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *DashboardAuthRepository) DeleteUser(ctx context.Context, userID string) error {
	queryBuilder := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Delete("dashboard_users").
		Where(sq.Eq{"id": userID})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	return err
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
