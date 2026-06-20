package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/krobus00/hft-service/internal/entity"
)

type OrderHistoryRepository struct {
	db *sqlx.DB
}

func NewOrderHistoryRepository(db *sqlx.DB) *OrderHistoryRepository {
	return &OrderHistoryRepository{db: db}
}

func (r *OrderHistoryRepository) Create(ctx context.Context, orderHistory *entity.OrderHistory) error {
	queryBuilder := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Insert(orderHistory.TableName()).
		Columns(
			"request_id",
			"user_id",
			"exchange",
			"market_type",
			"position_side",
			"symbol",
			"order_id",
			"entry_order_id",
			"client_order_id",
			"side",
			"type",
			"price",
			"quantity",
			"filled_quantity",
			"avg_fill_price",
			"status",
			"leverage",
			"fee",
			"realized_pnl",
			"created_at_exchange",
			"sent_at",
			"acknowledged_at",
			"filled_at",
			"strategy_id",
			"trade_condition",
			"order_reason",
			"exit_type",
			"error_message",
			"created_at",
			"updated_at",
			"is_paper_trading",
		).
		Values(
			orderHistory.RequestID,
			orderHistory.UserID,
			orderHistory.Exchange,
			orderHistory.MarketType,
			orderHistory.PositionSide,
			orderHistory.Symbol,
			orderHistory.OrderID,
			orderHistory.EntryOrderID,
			orderHistory.ClientOrderID,
			orderHistory.Side,
			orderHistory.Type,
			orderHistory.Price,
			orderHistory.Quantity,
			orderHistory.FilledQuantity,
			orderHistory.AvgFillPrice,
			orderHistory.Status,
			orderHistory.Leverage,
			orderHistory.Fee,
			orderHistory.RealizedPnl,
			orderHistory.CreatedAtExchange,
			orderHistory.SentAt,
			orderHistory.AcknowledgedAt,
			orderHistory.FilledAt,
			orderHistory.StrategyID,
			orderHistory.TradeCondition,
			orderHistory.OrderReason,
			orderHistory.ExitType,
			orderHistory.ErrorMessage,
			orderHistory.CreatedAt,
			orderHistory.UpdatedAt,
			orderHistory.IsPaperTrading,
		).
		Suffix("RETURNING id")

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return err
	}

	var id string
	err = r.db.QueryRowContext(ctx, query, args...).Scan(&id)
	if err != nil {
		return err
	}

	orderHistory.ID = id

	return err
}

func (r *OrderHistoryRepository) GetByRequestID(ctx context.Context, requestID string) (*entity.OrderHistory, error) {
	var orderHistory entity.OrderHistory
	err := r.db.GetContext(ctx, &orderHistory, "SELECT * FROM order_histories WHERE request_id = $1", requestID)
	if err != nil {
		return nil, err
	}
	return &orderHistory, nil
}

func (r *OrderHistoryRepository) FindByID(ctx context.Context, id string) (*entity.OrderHistory, error) {
	var orderHistory entity.OrderHistory
	err := r.db.GetContext(ctx, &orderHistory, "SELECT * FROM order_histories WHERE id = $1 LIMIT 1", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &orderHistory, nil
}

func (r *OrderHistoryRepository) FindByIDWithMetrics(ctx context.Context, id string) (*entity.OrderHistoryWithMetrics, error) {
	query, args, err := orderHistoryMetricsSelect().
		Where(sq.Eq{"oh.id": id}).
		Limit(1).
		ToSql()
	if err != nil {
		return nil, err
	}

	var orderHistory entity.OrderHistoryWithMetrics
	err = r.db.GetContext(ctx, &orderHistory, query, args...)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &orderHistory, nil
}

func (r *OrderHistoryRepository) GetPagination(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	model := &entity.OrderHistory{}
	baseSelect := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("*").
		From("order_histories")
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

	items := []entity.OrderHistory{}
	if err := r.db.SelectContext(ctx, &items, selectQuery, selectArgs...); err != nil {
		return nil, err
	}

	return apiutil.NewPaginationResp(req.Paginate.Page, req.Paginate.Limit, total, items), nil
}

func (r *OrderHistoryRepository) GetPaginationWithMetrics(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	model := &entity.OrderHistory{}
	baseSelect := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("oh.*").
		From("order_histories oh").
		Where(sq.Eq{"oh.trade_condition": string(entity.TradeConditionEntry)})
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

	baseSortField, baseSortDirection := orderHistoryBaseSort(req)
	metricSortField, metricSortDirection := orderHistoryMetricSort(req)

	pagedIDs := baseSelect.
		OrderBy(baseSortField + " " + baseSortDirection).
		Limit(uint64(req.Paginate.Limit)).
		Offset(uint64(req.Paginate.Offset))

	selectBuilder := orderHistoryMetricsSelect().
		Where(sq.Expr("oh.id IN (SELECT id FROM (?) paged_ids)", pagedIDs)).
		OrderBy(metricSortField + " " + metricSortDirection)
	selectQuery, selectArgs, err := selectBuilder.ToSql()
	if err != nil {
		return nil, err
	}

	items := []entity.OrderHistoryWithMetrics{}
	if err := r.db.SelectContext(ctx, &items, selectQuery, selectArgs...); err != nil {
		return nil, err
	}

	return apiutil.NewPaginationResp(req.Paginate.Page, req.Paginate.Limit, total, items), nil
}

func (r *OrderHistoryRepository) ListOpenEntriesByStrategyConfig(ctx context.Context, config entity.StrategyConfig) ([]entity.OrderHistoryWithMetrics, error) {
	query := orderHistoryMetricsSelect().
		Where(sq.Eq{
			"oh.strategy_id":     config.Strategy,
			"oh.exchange":        config.Exchange,
			"oh.market_type":     config.MarketType,
			"oh.symbol":          config.Symbol,
			"oh.trade_condition": string(entity.TradeConditionEntry),
		}).
		Where("trade.exit_price IS NULL").
		OrderBy("oh.created_at DESC")

	sqlQuery, args, err := query.ToSql()
	if err != nil {
		return nil, err
	}

	items := []entity.OrderHistoryWithMetrics{}
	err = r.db.SelectContext(ctx, &items, sqlQuery, args...)
	return items, err
}

func (r *OrderHistoryRepository) GetDashboardOverview(ctx context.Context, since time.Time, recentLimit uint64) (entity.DashboardOrderSummary, []entity.OrderHistoryWithMetrics, error) {
	var summary entity.DashboardOrderSummary
	if err := r.db.GetContext(ctx, &summary, `SELECT
	(SELECT COUNT(*) FROM order_histories WHERE created_at >= $1) AS orders_24h,
	(SELECT COUNT(*) FROM order_histories WHERE created_at >= $1 AND (status IN ('REJECTED', 'EXPIRED') OR error_message <> '')) AS problem_orders_24h,
	(SELECT COUNT(*) FROM order_trades WHERE exit_time >= $1) AS closed_trades,
	(SELECT COUNT(*) FROM order_trades WHERE exit_time >= $1 AND profit > 0) AS winning_trades,
	(SELECT COUNT(*) FROM order_trades WHERE exit_time IS NULL) AS running_trades,
	(SELECT COALESCE(SUM(profit), 0) FROM order_trades WHERE exit_time >= $1) AS realized_pnl,
	(SELECT COALESCE(SUM(CASE
		WHEN trade.side IN ('LONG','BUY') THEN (price.price - trade.entry_price) * trade.quantity
		WHEN trade.side IN ('SHORT','SELL') THEN (trade.entry_price - price.price) * trade.quantity
	END), 0)
	FROM order_trades trade
	JOIN price_references price ON price.exchange = trade.exchange
		AND price.market_type = trade.market_type
		AND price.symbol = trade.symbol
	WHERE trade.exit_time IS NULL AND trade.entry_price IS NOT NULL) AS running_pnl,
	(SELECT MAX(updated_at) FROM price_references) AS last_price_at
	`, since); err != nil {
		return entity.DashboardOrderSummary{}, nil, err
	}

	recentQuery, recentArgs, err := orderHistoryMetricsSelect().
		Where(sq.Eq{"oh.trade_condition": string(entity.TradeConditionEntry)}).
		OrderBy("oh.created_at DESC").
		Limit(recentLimit).
		ToSql()
	if err != nil {
		return entity.DashboardOrderSummary{}, nil, err
	}
	recent := []entity.OrderHistoryWithMetrics{}
	if err := r.db.SelectContext(ctx, &recent, recentQuery, recentArgs...); err != nil {
		return entity.DashboardOrderSummary{}, nil, err
	}
	return summary, recent, nil
}

func orderHistoryBaseSort(req *apiutil.PaginationReq) (string, string) {
	if isOrderHistoryMetricSort(req.Sort.Field) {
		return "created_at", "DESC"
	}
	return req.Sort.Field, req.Sort.Direction
}

func orderHistoryMetricSort(req *apiutil.PaginationReq) (string, string) {
	if isOrderHistoryMetricSort(req.Sort.Field) {
		return req.Sort.Field, req.Sort.Direction + " NULLS LAST"
	}
	return req.Sort.Field, req.Sort.Direction
}

func isOrderHistoryMetricSort(field string) bool {
	switch field {
	case "state", "entry_price", "exit_price", "pnl":
		return true
	default:
		return false
	}
}

func orderHistoryMetricsSelect() sq.SelectBuilder {
	return sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select(
			"oh.*",
			`CASE
				WHEN trade.exit_price IS NOT NULL THEN 'closed'
				ELSE 'running'
			END AS state`,
			`COALESCE(trade.entry_price, oh.avg_fill_price, oh.price) AS entry_price`,
			`trade.exit_price AS exit_price`,
			`CASE
				WHEN COALESCE(trade.entry_price, oh.avg_fill_price, oh.price) IS NULL THEN NULL
				WHEN trade.exit_price IS NOT NULL THEN trade.profit
				WHEN pr.price IS NOT NULL THEN CASE
					WHEN oh.side IN ('LONG','BUY') THEN (pr.price - COALESCE(trade.entry_price, oh.avg_fill_price, oh.price)) * COALESCE(trade.quantity, NULLIF(oh.filled_quantity, 0), oh.quantity)
					WHEN oh.side IN ('SHORT','SELL') THEN (COALESCE(trade.entry_price, oh.avg_fill_price, oh.price) - pr.price) * COALESCE(trade.quantity, NULLIF(oh.filled_quantity, 0), oh.quantity)
				END
			END AS pnl`,
		).
		From("order_histories oh").
		LeftJoin("order_trades trade ON trade.entry_history_id = oh.id").
		LeftJoin(`price_references pr ON pr.exchange = oh.exchange
			AND pr.market_type = oh.market_type
			AND pr.symbol = oh.symbol`)
}

func (r *OrderHistoryRepository) ListExchanges(ctx context.Context) ([]string, error) {
	items := []string{}
	err := r.db.SelectContext(ctx, &items, "SELECT DISTINCT exchange FROM order_histories WHERE exchange <> '' ORDER BY exchange")
	return items, err
}

func (r *OrderHistoryRepository) GetByStatus(ctx context.Context, statuses []string) ([]entity.OrderHistory, error) {
	if len(statuses) == 0 {
		return []entity.OrderHistory{}, nil
	}

	queryBuilder := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Select("*").
		From("order_histories").
		Where(sq.Eq{"status": statuses}).
		OrderBy("created_at desc")

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, err
	}

	var orderHistories []entity.OrderHistory
	err = r.db.SelectContext(ctx, &orderHistories, query, args...)
	if err != nil {
		return nil, err
	}

	return orderHistories, nil
}

func (r *OrderHistoryRepository) Update(ctx context.Context, orderHistory *entity.OrderHistory) error {
	queryBuilder := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Update(orderHistory.TableName()).
		Set("request_id", orderHistory.RequestID).
		Set("user_id", orderHistory.UserID).
		Set("exchange", orderHistory.Exchange).
		Set("market_type", orderHistory.MarketType).
		Set("position_side", orderHistory.PositionSide).
		Set("symbol", orderHistory.Symbol).
		Set("order_id", orderHistory.OrderID).
		Set("entry_order_id", orderHistory.EntryOrderID).
		Set("client_order_id", orderHistory.ClientOrderID).
		Set("side", orderHistory.Side).
		Set("type", orderHistory.Type).
		Set("price", orderHistory.Price).
		Set("quantity", orderHistory.Quantity).
		Set("filled_quantity", orderHistory.FilledQuantity).
		Set("avg_fill_price", orderHistory.AvgFillPrice).
		Set("status", orderHistory.Status).
		Set("leverage", orderHistory.Leverage).
		Set("fee", orderHistory.Fee).
		Set("realized_pnl", orderHistory.RealizedPnl).
		Set("created_at_exchange", orderHistory.CreatedAtExchange).
		Set("sent_at", orderHistory.SentAt).
		Set("acknowledged_at", orderHistory.AcknowledgedAt).
		Set("filled_at", orderHistory.FilledAt).
		Set("strategy_id", orderHistory.StrategyID).
		Set("trade_condition", orderHistory.TradeCondition).
		Set("order_reason", orderHistory.OrderReason).
		Set("exit_type", orderHistory.ExitType).
		Set("error_message", orderHistory.ErrorMessage).
		Set("is_paper_trading", orderHistory.IsPaperTrading).
		Set("updated_at", orderHistory.UpdatedAt).
		Where(sq.Eq{"id": orderHistory.ID})

	query, args, err := queryBuilder.ToSql()
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

func (r *OrderHistoryRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM order_histories WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *OrderHistoryRepository) ListTradePnL(ctx context.Context, filter entity.OrderReportFilter) ([]entity.OrderTradePnL, int64, error) {
	whereSQL, args := tradeReportWhere(filter)
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	args = append(args, limit, (page-1)*limit)
	limitArg := len(args) - 1
	offsetArg := len(args)

	query := fmt.Sprintf(`SELECT
	entry_order_id, COALESCE(strategy_id, '') AS strategy_id, symbol, side,
	entry_price, exit_price, quantity AS qty, profit,
	SUM(profit) OVER (ORDER BY exit_time ASC, entry_order_id ASC) AS running_profit,
	entry_time, exit_time, COUNT(*) OVER() AS total_count
FROM order_trades
WHERE %s
ORDER BY exit_time DESC, entry_order_id DESC
LIMIT $%d OFFSET $%d`, whereSQL, limitArg, offsetArg)

	items := []entity.OrderTradePnL{}
	if err := r.db.SelectContext(ctx, &items, query, args...); err != nil {
		return nil, 0, err
	}
	total := int64(0)
	if len(items) > 0 {
		total = items[0].TotalCount
	}
	return items, total, nil
}

func (r *OrderHistoryRepository) ListDailyReport(ctx context.Context, filter entity.OrderReportFilter) ([]entity.DailyOrderReport, error) {
	whereSQL, args := tradeReportWhere(filter)
	query := fmt.Sprintf(`SELECT
	date_trunc('day', exit_time)::date AS trade_date,
	COALESCE(strategy_id, '') AS strategy_id,
	symbol,
	MIN(entry_time) AS start_trade_at,
	MAX(exit_time) AS end_trade_at,
	COALESCE(SUM(profit), 0) AS total_profit,
	COALESCE(AVG(CASE WHEN profit > 0 THEN 1 ELSE 0 END), 0) AS win_rate,
	COALESCE(AVG(quantity), 0) AS avg_size,
	COUNT(*) AS total_trades,
	COALESCE(SUM(CASE WHEN profit >= 0 THEN 1 ELSE 0 END), 0) AS winning_trades,
	COALESCE(SUM(CASE WHEN profit < 0 THEN 1 ELSE 0 END), 0) AS losing_trades
FROM order_trades
WHERE %s
GROUP BY date_trunc('day', exit_time)::date, strategy_id, symbol
ORDER BY trade_date DESC, strategy_id ASC, symbol ASC
LIMIT 1000`, whereSQL)

	items := []entity.DailyOrderReport{}
	err := r.db.SelectContext(ctx, &items, query, args...)
	return items, err
}

func (r *OrderHistoryRepository) ListStrategyPerformance(ctx context.Context, filter entity.OrderReportFilter) ([]entity.StrategyPerformanceReport, error) {
	whereSQL, args := tradeReportWhere(filter)
	query := fmt.Sprintf(`SELECT
	COALESCE(NULLIF(strategy_id, ''), 'unknown') AS strategy_id,
	COALESCE(SUM(profit), 0) AS total_profit,
	COALESCE(AVG(CASE WHEN profit > 0 THEN 1 ELSE 0 END), 0) AS win_rate,
	COALESCE(AVG(profit), 0) AS avg_profit,
	COALESCE(AVG(quantity), 0) AS avg_size,
	COALESCE(MAX(profit), 0) AS best_trade,
	COALESCE(MIN(profit), 0) AS worst_trade,
	COALESCE(
		SUM(CASE WHEN profit > 0 THEN profit ELSE 0 END) /
		NULLIF(ABS(SUM(CASE WHEN profit < 0 THEN profit ELSE 0 END)), 0),
		0
	) AS profit_factor,
	COUNT(*) AS total_trades,
	COALESCE(SUM(CASE WHEN profit > 0 THEN 1 ELSE 0 END), 0) AS winning_trades,
	COALESCE(SUM(CASE WHEN profit < 0 THEN 1 ELSE 0 END), 0) AS losing_trades,
	MIN(entry_time) AS first_trade_at,
	MAX(exit_time) AS last_trade_at
FROM order_trades
WHERE %s
GROUP BY COALESCE(NULLIF(strategy_id, ''), 'unknown')
ORDER BY total_profit DESC, win_rate DESC, total_trades DESC, strategy_id ASC
LIMIT 1000`, whereSQL)

	items := []entity.StrategyPerformanceReport{}
	err := r.db.SelectContext(ctx, &items, query, args...)
	return items, err
}

func tradeReportWhere(filter entity.OrderReportFilter) (string, []any) {
	conditions := []string{"exit_price IS NOT NULL", "profit IS NOT NULL"}
	args := []any{}
	if filter.StartTime != nil {
		args = append(args, *filter.StartTime)
		conditions = append(conditions, fmt.Sprintf("exit_time >= $%d", len(args)))
	}
	if filter.EndTime != nil {
		args = append(args, *filter.EndTime)
		conditions = append(conditions, fmt.Sprintf("exit_time <= $%d", len(args)))
	}
	if strings.TrimSpace(filter.StrategyID) != "" {
		args = append(args, strings.TrimSpace(filter.StrategyID))
		conditions = append(conditions, fmt.Sprintf("strategy_id = $%d", len(args)))
	}
	if strings.TrimSpace(filter.Symbol) != "" {
		args = append(args, strings.TrimSpace(filter.Symbol))
		conditions = append(conditions, fmt.Sprintf("symbol = $%d", len(args)))
	}
	return strings.Join(conditions, " AND "), args
}
