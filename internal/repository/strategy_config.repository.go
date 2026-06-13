package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/krobus00/hft-service/internal/entity"
)

type StrategyConfigRepository struct {
	db *sqlx.DB
}

func NewStrategyConfigRepository(db *sqlx.DB) *StrategyConfigRepository {
	return &StrategyConfigRepository{db: db}
}

func (r *StrategyConfigRepository) FindByID(ctx context.Context, id string) (*entity.StrategyConfig, error) {
	var item entity.StrategyConfig
	err := r.db.GetContext(ctx, &item, "SELECT * FROM strategy_configs WHERE id = $1 LIMIT 1", id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *StrategyConfigRepository) Create(ctx context.Context, item *entity.StrategyConfig) error {
	query := `INSERT INTO strategy_configs (
		strategy, exchange, market_type, symbol, interval, user_id, position_side, source,
		need_notification, is_paper_trading, order_type, order_qty, limit_slippage_pct,
		cooldown_bars, sl_cooldown_bars, max_consecutive_stop_losses, sl_pause_bars,
		take_profit_pct, stop_loss_pct, trailing_stop_pct, trailing_stop_trigger_pct,
		max_hold_bars, max_positions, enable_intrabar_risk_exit, created_at, updated_at
	) VALUES (
		:strategy, :exchange, :market_type, :symbol, :interval, :user_id, :position_side, :source,
		:need_notification, :is_paper_trading, :order_type, :order_qty, :limit_slippage_pct,
		:cooldown_bars, :sl_cooldown_bars, :max_consecutive_stop_losses, :sl_pause_bars,
		:take_profit_pct, :stop_loss_pct, :trailing_stop_pct, :trailing_stop_trigger_pct,
		:max_hold_bars, :max_positions, :enable_intrabar_risk_exit, :created_at, :updated_at
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

func (r *StrategyConfigRepository) Update(ctx context.Context, item *entity.StrategyConfig) error {
	item.UpdatedAt = time.Now().UTC()
	query := `UPDATE strategy_configs
		SET strategy = :strategy,
			exchange = :exchange,
			market_type = :market_type,
			symbol = :symbol,
			interval = :interval,
			user_id = :user_id,
			position_side = :position_side,
			source = :source,
			need_notification = :need_notification,
			is_paper_trading = :is_paper_trading,
			order_type = :order_type,
			order_qty = :order_qty,
			limit_slippage_pct = :limit_slippage_pct,
			cooldown_bars = :cooldown_bars,
			sl_cooldown_bars = :sl_cooldown_bars,
			max_consecutive_stop_losses = :max_consecutive_stop_losses,
			sl_pause_bars = :sl_pause_bars,
			take_profit_pct = :take_profit_pct,
			stop_loss_pct = :stop_loss_pct,
			trailing_stop_pct = :trailing_stop_pct,
			trailing_stop_trigger_pct = :trailing_stop_trigger_pct,
			max_hold_bars = :max_hold_bars,
			max_positions = :max_positions,
			enable_intrabar_risk_exit = :enable_intrabar_risk_exit,
			updated_at = :updated_at
		WHERE id = :id`
	_, err := r.db.NamedExecContext(ctx, query, item)
	return err
}

func (r *StrategyConfigRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM strategy_configs WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *StrategyConfigRepository) GetPagination(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	model := &entity.StrategyConfig{}
	baseSelect := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("*").
		From("strategy_configs")
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

	items := []entity.StrategyConfig{}
	if err := r.db.SelectContext(ctx, &items, selectQuery, selectArgs...); err != nil {
		return nil, err
	}

	return apiutil.NewPaginationResp(req.Paginate.Page, req.Paginate.Limit, total, items), nil
}
