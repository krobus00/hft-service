package repository

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/krobus00/hft-service/internal/entity"
)

type APIAuthRepository struct {
	db *sqlx.DB
}

func NewAPIAuthRepository(db *sqlx.DB) *APIAuthRepository {
	return &APIAuthRepository{db: db}
}

func (r *APIAuthRepository) CountUsers(ctx context.Context) (int64, error) {
	var total int64
	err := r.db.GetContext(ctx, &total, "SELECT COUNT(1) FROM users")
	return total, err
}

func (r *APIAuthRepository) CreateUser(ctx context.Context, user *entity.APIUser, roleNames []string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query, args, err := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Insert("users").
		Columns("email", "name", "password_hash", "active").
		Values(user.Email, user.Name, user.PasswordHash, user.Active).
		Suffix("RETURNING id, created_at, updated_at").
		ToSql()
	if err != nil {
		return err
	}
	if err := tx.QueryRowxContext(ctx, query, args...).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt); err != nil {
		return err
	}

	for _, roleName := range roleNames {
		_, err := tx.ExecContext(ctx, `
INSERT INTO user_roles (user_id, role_id)
SELECT $1, id FROM roles WHERE name = $2
ON CONFLICT DO NOTHING`, user.ID, roleName)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *APIAuthRepository) GetUserByEmail(ctx context.Context, email string) (*entity.APIUser, error) {
	var user entity.APIUser
	err := r.db.GetContext(ctx, &user, "SELECT * FROM users WHERE email = $1 LIMIT 1", email)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *APIAuthRepository) GetUserByID(ctx context.Context, id string) (*entity.APIUser, error) {
	var user entity.APIUser
	err := r.db.GetContext(ctx, &user, "SELECT * FROM users WHERE id = $1 LIMIT 1", id)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *APIAuthRepository) ListRoleNamesByUserID(ctx context.Context, userID string) ([]string, error) {
	roles := []string{}
	err := r.db.SelectContext(ctx, &roles, `
SELECT r.name
FROM roles r
JOIN user_roles ur ON ur.role_id = r.id
WHERE ur.user_id = $1
ORDER BY r.name`, userID)
	return roles, err
}

func (r *APIAuthRepository) ListPermissionNamesByUserID(ctx context.Context, userID string) ([]string, error) {
	permissions := []string{}
	err := r.db.SelectContext(ctx, &permissions, `
SELECT DISTINCT p.name
FROM permissions p
JOIN role_permissions rp ON rp.permission_id = p.id
JOIN user_roles ur ON ur.role_id = rp.role_id
WHERE ur.user_id = $1
ORDER BY p.name`, userID)
	return permissions, err
}

func (r *APIAuthRepository) UpdateLastLogin(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE users SET last_login_at = NOW(), updated_at = NOW() WHERE id = $1", userID)
	return err
}

func (r *APIAuthRepository) FindUserByID(ctx context.Context, id string) (*entity.APIUser, error) {
	var user entity.APIUser
	err := r.db.GetContext(ctx, &user, "SELECT id, email, name, active, last_login_at, created_at, updated_at FROM users WHERE id = $1 LIMIT 1", id)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *APIAuthRepository) FindRoleByID(ctx context.Context, id string) (*entity.APIRole, error) {
	var role entity.APIRole
	err := r.db.GetContext(ctx, &role, "SELECT id, name, description, created_at, updated_at FROM roles WHERE id = $1 LIMIT 1", id)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *APIAuthRepository) GetUsersPagination(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	model := &entity.APIUser{}
	baseSelect := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("id", "email", "name", "active", "last_login_at", "created_at", "updated_at").
		From("users")
	baseSelect = req.ApplyFilter(baseSelect, model)

	countBuilder := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("COUNT(*)").
		FromSelect(baseSelect, "count_query")
	countQuery, countArgs, err := countBuilder.ToSql()
	if err != nil {
		return nil, err
	}

	var total int64
	if err := r.db.GetContext(ctx, &total, countQuery, countArgs...); err != nil {
		return nil, err
	}

	selectBuilder := baseSelect.OrderBy(req.Sort.Field + " " + req.Sort.Direction).
		Limit(uint64(req.Paginate.Limit)).
		Offset(uint64(req.Paginate.Offset))
	selectQuery, selectArgs, err := selectBuilder.ToSql()
	if err != nil {
		return nil, err
	}

	items := []entity.APIUser{}
	if err := r.db.SelectContext(ctx, &items, selectQuery, selectArgs...); err != nil {
		return nil, err
	}

	return apiutil.NewPaginationResp(req.Paginate.Page, req.Paginate.Limit, total, items), nil
}

func (r *APIAuthRepository) GetRolesPagination(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	model := &entity.APIRole{}
	baseSelect := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("id", "name", "description", "created_at", "updated_at").
		From("roles")
	baseSelect = req.ApplyFilter(baseSelect, model)

	countBuilder := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("COUNT(*)").
		FromSelect(baseSelect, "count_query")
	countQuery, countArgs, err := countBuilder.ToSql()
	if err != nil {
		return nil, err
	}

	var total int64
	if err := r.db.GetContext(ctx, &total, countQuery, countArgs...); err != nil {
		return nil, err
	}

	selectBuilder := baseSelect.OrderBy(req.Sort.Field + " " + req.Sort.Direction).
		Limit(uint64(req.Paginate.Limit)).
		Offset(uint64(req.Paginate.Offset))
	selectQuery, selectArgs, err := selectBuilder.ToSql()
	if err != nil {
		return nil, err
	}

	items := []entity.APIRole{}
	if err := r.db.SelectContext(ctx, &items, selectQuery, selectArgs...); err != nil {
		return nil, err
	}

	return apiutil.NewPaginationResp(req.Paginate.Page, req.Paginate.Limit, total, items), nil
}
