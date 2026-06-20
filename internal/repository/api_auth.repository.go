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

func (r *APIAuthRepository) ListPermissionNamesByRoleID(ctx context.Context, roleID string) ([]string, error) {
	permissions := []string{}
	err := r.db.SelectContext(ctx, &permissions, `
SELECT p.name
FROM permissions p
JOIN role_permissions rp ON rp.permission_id = p.id
WHERE rp.role_id = $1
ORDER BY p.name`, roleID)
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

func (r *APIAuthRepository) UpdateUserProfile(ctx context.Context, userID, name, passwordHash string) error {
	builder := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Update("users").
		Set("name", name).
		Set("updated_at", sq.Expr("NOW()")).
		Where(sq.Eq{"id": userID})
	if passwordHash != "" {
		builder = builder.Set("password_hash", passwordHash)
	}
	query, args, err := builder.ToSql()
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
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

func (r *APIAuthRepository) FindPermissionByID(ctx context.Context, id string) (*entity.APIPermission, error) {
	var permission entity.APIPermission
	err := r.db.GetContext(ctx, &permission, "SELECT id, name, description, created_at FROM permissions WHERE id = $1 LIMIT 1", id)
	if err != nil {
		return nil, err
	}
	return &permission, nil
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

func (r *APIAuthRepository) CreatePermission(ctx context.Context, permission *entity.APIPermission) error {
	query, args, err := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Insert("permissions").
		Columns("name", "description").
		Values(permission.Name, permission.Description).
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return err
	}
	return r.db.QueryRowxContext(ctx, query, args...).Scan(&permission.ID, &permission.CreatedAt)
}

func (r *APIAuthRepository) UpdatePermission(ctx context.Context, permission *entity.APIPermission) error {
	result, err := r.db.ExecContext(ctx, "UPDATE permissions SET name = $1, description = $2 WHERE id = $3", permission.Name, permission.Description, permission.ID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *APIAuthRepository) DeletePermission(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM permissions WHERE id = $1", id)
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

func (r *APIAuthRepository) GetPermissionsPagination(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	model := &entity.APIPermission{}
	baseSelect := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("id", "name", "description", "created_at").
		From("permissions")
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

	items := []entity.APIPermission{}
	if err := r.db.SelectContext(ctx, &items, selectQuery, selectArgs...); err != nil {
		return nil, err
	}

	return apiutil.NewPaginationResp(req.Paginate.Page, req.Paginate.Limit, total, items), nil
}

func (r *APIAuthRepository) FindDashboardPageByID(ctx context.Context, id string) (*entity.APIDashboardPage, error) {
	var page entity.APIDashboardPage
	err := r.db.GetContext(ctx, &page, `
SELECT id, resource_key, parent_key, label, description, short_description, icon, path, read_permission, write_permission, sort_order, visible, created_at, updated_at
FROM dashboard_pages
WHERE id = $1
LIMIT 1`, id)
	if err != nil {
		return nil, err
	}
	return &page, nil
}

func (r *APIAuthRepository) ListVisibleDashboardPages(ctx context.Context) ([]entity.APIDashboardPage, error) {
	items := []entity.APIDashboardPage{}
	err := r.db.SelectContext(ctx, &items, `
SELECT id, resource_key, parent_key, label, description, short_description, icon, path, read_permission, write_permission, sort_order, visible, created_at, updated_at
FROM dashboard_pages
WHERE visible = TRUE
ORDER BY sort_order ASC, label ASC`)
	return items, err
}

func (r *APIAuthRepository) CreateDashboardPage(ctx context.Context, page *entity.APIDashboardPage) error {
	query, args, err := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Insert("dashboard_pages").
		Columns("resource_key", "parent_key", "label", "description", "short_description", "icon", "path", "read_permission", "write_permission", "sort_order", "visible").
		Values(page.ResourceKey, page.ParentKey, page.Label, page.Description, page.ShortDescription, page.Icon, page.Path, page.ReadPermission, page.WritePermission, page.SortOrder, page.Visible).
		Suffix("RETURNING id, created_at, updated_at").
		ToSql()
	if err != nil {
		return err
	}
	return r.db.QueryRowxContext(ctx, query, args...).Scan(&page.ID, &page.CreatedAt, &page.UpdatedAt)
}

func (r *APIAuthRepository) UpdateDashboardPage(ctx context.Context, page *entity.APIDashboardPage) error {
	query, args, err := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Update("dashboard_pages").
		Set("resource_key", page.ResourceKey).
		Set("parent_key", page.ParentKey).
		Set("label", page.Label).
		Set("description", page.Description).
		Set("short_description", page.ShortDescription).
		Set("icon", page.Icon).
		Set("path", page.Path).
		Set("read_permission", page.ReadPermission).
		Set("write_permission", page.WritePermission).
		Set("sort_order", page.SortOrder).
		Set("visible", page.Visible).
		Set("updated_at", sq.Expr("NOW()")).
		Where(sq.Eq{"id": page.ID}).
		ToSql()
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *APIAuthRepository) DeleteDashboardPage(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM dashboard_pages WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *APIAuthRepository) GetDashboardPagesPagination(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	model := &entity.APIDashboardPage{}
	baseSelect := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("id", "resource_key", "parent_key", "label", "description", "short_description", "icon", "path", "read_permission", "write_permission", "sort_order", "visible", "created_at", "updated_at").
		From("dashboard_pages")
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

	items := []entity.APIDashboardPage{}
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
