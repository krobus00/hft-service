package repository

import (
	"context"
	"database/sql"

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
