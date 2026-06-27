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

type StrategyRuleRepository struct{ db *sqlx.DB }

func NewStrategyRuleRepository(db *sqlx.DB) *StrategyRuleRepository {
	return &StrategyRuleRepository{db: db}
}

func (r *StrategyRuleRepository) ListEnabledByStrategyConfigIDs(ctx context.Context, ids []string) ([]entity.StrategyRule, error) {
	items := []entity.StrategyRule{}
	if len(ids) == 0 {
		return items, nil
	}
	query, args, err := sqlx.In("SELECT * FROM strategy_rules WHERE enabled AND strategy_config_id IN (?) ORDER BY strategy_config_id, created_at", ids)
	if err != nil {
		return nil, err
	}
	query = r.db.Rebind(query)
	err = r.db.SelectContext(ctx, &items, query, args...)
	return items, err
}

func (r *StrategyRuleRepository) FindByID(ctx context.Context, id string) (*entity.StrategyRule, error) {
	var item entity.StrategyRule
	err := r.db.GetContext(ctx, &item, "SELECT * FROM strategy_rules WHERE id = $1 LIMIT 1", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *StrategyRuleRepository) Create(ctx context.Context, item *entity.StrategyRule) error {
	rows, err := r.db.NamedQueryContext(ctx, `INSERT INTO strategy_rules (
		strategy_config_id, name, side, trade_condition, exit_type, order_reason, conditions, enabled, created_at, updated_at
	) VALUES (
		:strategy_config_id, :name, :side, :trade_condition, :exit_type, :order_reason, :conditions, :enabled, :created_at, :updated_at
	) RETURNING id`, item)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return rows.Scan(&item.ID)
	}
	return nil
}

func (r *StrategyRuleRepository) Update(ctx context.Context, item *entity.StrategyRule) error {
	item.UpdatedAt = time.Now().UTC()
	_, err := r.db.NamedExecContext(ctx, `UPDATE strategy_rules
SET strategy_config_id = :strategy_config_id,
	name = :name,
	side = :side,
	trade_condition = :trade_condition,
	exit_type = :exit_type,
	order_reason = :order_reason,
	conditions = :conditions,
	enabled = :enabled,
	updated_at = :updated_at
WHERE id = :id`, item)
	return err
}

func (r *StrategyRuleRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM strategy_rules WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *StrategyRuleRepository) GetPagination(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	model := &entity.StrategyRule{}
	baseSelect := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).Select("*").From("strategy_rules")
	baseSelect = req.ApplyFilter(baseSelect, model)

	countQuery, countArgs, err := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("COUNT(*)").FromSelect(baseSelect, "count_query").ToSql()
	if err != nil {
		return nil, err
	}
	var total int64
	if err := r.db.GetContext(ctx, &total, countQuery, countArgs...); err != nil {
		return nil, err
	}

	selectQuery, selectArgs, err := baseSelect.OrderBy(req.Sort.Field + " " + req.Sort.Direction).
		Limit(uint64(req.Paginate.Limit)).Offset(uint64(req.Paginate.Offset)).ToSql()
	if err != nil {
		return nil, err
	}
	items := []entity.StrategyRule{}
	if err := r.db.SelectContext(ctx, &items, selectQuery, selectArgs...); err != nil {
		return nil, err
	}
	return apiutil.NewPaginationResp(req.Paginate.Page, req.Paginate.Limit, total, items), nil
}
