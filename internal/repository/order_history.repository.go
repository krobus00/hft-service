package repository

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
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
			"symbol",
			"order_id",
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
			"error_message",
			"created_at",
			"updated_at",
		).
		Values(
			orderHistory.RequestID,
			orderHistory.UserID,
			orderHistory.Exchange,
			orderHistory.Symbol,
			orderHistory.OrderID,
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
			orderHistory.ErrorMessage,
			orderHistory.CreatedAt,
			orderHistory.UpdatedAt,
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
