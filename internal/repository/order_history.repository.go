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
		Set("price", orderHistory.Price).
		Set("filled_quantity", orderHistory.FilledQuantity).
		Set("avg_fill_price", orderHistory.AvgFillPrice).
		Set("status", orderHistory.Status).
		Set("fee", orderHistory.Fee).
		Set("realized_pnl", orderHistory.RealizedPnl).
		Set("acknowledged_at", orderHistory.AcknowledgedAt).
		Set("filled_at", orderHistory.FilledAt).
		Set("updated_at", orderHistory.UpdatedAt).
		Where(sq.Eq{"id": orderHistory.ID})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	return err
}
