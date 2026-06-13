package repository

import (
	"context"
	"database/sql"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/krobus00/hft-service/internal/entity"
)

type APISettingRepository struct {
	db *sqlx.DB
}

func NewAPISettingRepository(db *sqlx.DB) *APISettingRepository {
	return &APISettingRepository{db: db}
}

func (r *APISettingRepository) FindByID(ctx context.Context, id string) (map[string]any, error) {
	query := `SELECT id, key, value, created_at, updated_at FROM settings WHERE id = $1 LIMIT 1`
	items, err := selectMaps(ctx, r.db, query, id)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, sql.ErrNoRows
	}
	return items[0], nil
}

func (r *APISettingRepository) FindByKey(ctx context.Context, key string) (map[string]any, error) {
	query := `SELECT id, key, value, created_at, updated_at FROM settings WHERE key = $1 LIMIT 1`
	items, err := selectMaps(ctx, r.db, query, key)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, sql.ErrNoRows
	}
	return items[0], nil
}

func (r *APISettingRepository) Create(ctx context.Context, values map[string]any) (map[string]any, error) {
	queryBuilder := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Insert("settings").
		Columns("key", "value").
		Values(normalizeSettingKey(values["key"]), normalizeConfigValue(values["value"])).
		Suffix("RETURNING id, key, value, created_at, updated_at")

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, err
	}
	items, err := selectMaps(ctx, r.db, query, args...)
	if err != nil {
		return nil, err
	}
	return items[0], nil
}

func (r *APISettingRepository) Update(ctx context.Context, id string, values map[string]any) (map[string]any, error) {
	queryBuilder := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).Update("settings")
	if value, ok := values["key"]; ok {
		queryBuilder = queryBuilder.Set("key", normalizeSettingKey(value))
	}
	if value, ok := values["value"]; ok {
		queryBuilder = queryBuilder.Set("value", normalizeConfigValue(value))
	}
	queryBuilder = queryBuilder.Set("updated_at", sq.Expr("NOW()")).
		Where(sq.Eq{"id": id}).
		Suffix("RETURNING id, key, value, created_at, updated_at")

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, err
	}
	items, err := selectMaps(ctx, r.db, query, args...)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, sql.ErrNoRows
	}
	return items[0], nil
}

func (r *APISettingRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM settings WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *APISettingRepository) GetPagination(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	model := &entity.APISetting{}
	baseSelect := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("id", "key", "value", "created_at", "updated_at").
		From("settings")
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

	items, err := selectMaps(ctx, r.db, selectQuery, selectArgs...)
	if err != nil {
		return nil, err
	}

	return apiutil.NewPaginationResp(req.Paginate.Page, req.Paginate.Limit, total, items), nil
}
