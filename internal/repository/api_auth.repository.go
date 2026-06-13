package repository

import (
	"context"
	"database/sql"

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

func (r *APIAuthRepository) ListRoleNames(ctx context.Context) ([]string, error) {
	items := []string{}
	err := r.db.SelectContext(ctx, &items, "SELECT name FROM roles ORDER BY name")
	return items, err
}

func (r *APIAuthRepository) ListPermissionNames(ctx context.Context) ([]string, error) {
	items := []string{}
	err := r.db.SelectContext(ctx, &items, "SELECT name FROM permissions ORDER BY name")
	return items, err
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

func (r *APIAuthRepository) UpdateUser(ctx context.Context, user *entity.APIUser, roleNames []string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	builder := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Update("users").
		Set("email", user.Email).
		Set("name", user.Name).
		Set("active", user.Active).
		Set("updated_at", sq.Expr("NOW()")).
		Where(sq.Eq{"id": user.ID})
	if user.PasswordHash != "" {
		builder = builder.Set("password_hash", user.PasswordHash)
	}
	query, args, err := builder.ToSql()
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}

	if roleNames != nil {
		if _, err := tx.ExecContext(ctx, "DELETE FROM user_roles WHERE user_id = $1", user.ID); err != nil {
			return err
		}
		for _, roleName := range roleNames {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO user_roles (user_id, role_id)
SELECT $1, id FROM roles WHERE name = $2
ON CONFLICT DO NOTHING`, user.ID, roleName); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (r *APIAuthRepository) DeleteUser(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *APIAuthRepository) CreateRole(ctx context.Context, role *entity.APIRole, permissionNames []string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query, args, err := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Insert("roles").
		Columns("name", "description").
		Values(role.Name, role.Description).
		Suffix("RETURNING id, created_at, updated_at").
		ToSql()
	if err != nil {
		return err
	}
	if err := tx.QueryRowxContext(ctx, query, args...).Scan(&role.ID, &role.CreatedAt, &role.UpdatedAt); err != nil {
		return err
	}
	if err := replaceRolePermissions(ctx, tx, role.ID, permissionNames); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *APIAuthRepository) UpdateRole(ctx context.Context, role *entity.APIRole, permissionNames []string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, "UPDATE roles SET name = $1, description = $2, updated_at = NOW() WHERE id = $3", role.Name, role.Description, role.ID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	if permissionNames != nil {
		if err := replaceRolePermissions(ctx, tx, role.ID, permissionNames); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *APIAuthRepository) DeleteRole(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM roles WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func replaceRolePermissions(ctx context.Context, tx *sqlx.Tx, roleID string, permissionNames []string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM role_permissions WHERE role_id = $1", roleID); err != nil {
		return err
	}
	for _, permissionName := range permissionNames {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO role_permissions (role_id, permission_id)
SELECT $1, id FROM permissions WHERE name = $2
ON CONFLICT DO NOTHING`, roleID, permissionName); err != nil {
			return err
		}
	}
	return nil
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
