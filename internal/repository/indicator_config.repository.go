package repository

import (
	"context"
	"database/sql"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/krobus00/hft-service/internal/entity"
)

type IndicatorConfigRepository struct {
	db *sqlx.DB
}

func NewIndicatorConfigRepository(db *sqlx.DB) *IndicatorConfigRepository {
	return &IndicatorConfigRepository{db: db}
}

func (r *IndicatorConfigRepository) ListForPair(ctx context.Context, exchange, marketType, symbol, interval string) ([]entity.IndicatorConfig, error) {
	items := []entity.IndicatorConfig{}
	err := r.db.SelectContext(ctx, &items, `
SELECT *
FROM indicator_configs
WHERE lower(exchange) = lower($1)
  AND lower(market_type) = lower($2)
  AND symbol = $3
  AND interval = $4
  AND enabled IS TRUE
ORDER BY output_name`, exchange, marketType, symbol, interval)
	return items, err
}

func (r *IndicatorConfigRepository) FindByID(ctx context.Context, id string) (*entity.IndicatorConfig, error) {
	var item entity.IndicatorConfig
	err := r.db.GetContext(ctx, &item, "SELECT * FROM indicator_configs WHERE id = $1 LIMIT 1", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *IndicatorConfigRepository) Create(ctx context.Context, item *entity.IndicatorConfig) error {
	query := `INSERT INTO indicator_configs (
		exchange, market_type, symbol, interval, indicator, output_name, params, enabled, created_at, updated_at
	) VALUES (
		:exchange, :market_type, :symbol, :interval, :indicator, :output_name, :params, :enabled, :created_at, :updated_at
	) RETURNING id`
	rows, err := r.db.NamedQueryContext(ctx, query, item)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return rows.Scan(&item.ID)
	}
	return nil
}

func (r *IndicatorConfigRepository) Update(ctx context.Context, item *entity.IndicatorConfig) error {
	item.UpdatedAt = time.Now().UTC()
	_, err := r.db.NamedExecContext(ctx, `UPDATE indicator_configs
SET exchange = :exchange,
	market_type = :market_type,
	symbol = :symbol,
	interval = :interval,
	indicator = :indicator,
	output_name = :output_name,
	params = :params,
	enabled = :enabled,
	updated_at = :updated_at
WHERE id = :id`, item)
	return err
}

func (r *IndicatorConfigRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM indicator_configs WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *IndicatorConfigRepository) GetPagination(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	model := &entity.IndicatorConfig{}
	baseSelect := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("*").
		From("indicator_configs")
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

	items := []entity.IndicatorConfig{}
	if err := r.db.SelectContext(ctx, &items, selectQuery, selectArgs...); err != nil {
		return nil, err
	}

	return apiutil.NewPaginationResp(req.Paginate.Page, req.Paginate.Limit, total, items), nil
}
