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

type KlineSubscriptionRepository struct {
	db *sqlx.DB
}

func NewKlineSubscriptionRepository(db *sqlx.DB) *KlineSubscriptionRepository {
	return &KlineSubscriptionRepository{db: db}
}

func (r *KlineSubscriptionRepository) GetAll(ctx context.Context) ([]entity.KlineSubscription, error) {
	var subscriptions []entity.KlineSubscription
	err := r.db.SelectContext(ctx, &subscriptions, "SELECT * FROM kline_subscriptions order by created_at desc")
	return subscriptions, err
}

func (r *KlineSubscriptionRepository) GetByExchange(ctx context.Context, exchange string) ([]entity.KlineSubscription, error) {
	var subscriptions []entity.KlineSubscription
	err := r.db.SelectContext(ctx, &subscriptions, "SELECT * FROM kline_subscriptions WHERE exchange = $1 order by created_at desc", exchange)
	return subscriptions, err
}

func (r *KlineSubscriptionRepository) GetByExchangeAndMarketType(ctx context.Context, exchange, marketType string) ([]entity.KlineSubscription, error) {
	var subscriptions []entity.KlineSubscription
	err := r.db.SelectContext(ctx, &subscriptions, "SELECT * FROM kline_subscriptions WHERE exchange = $1 AND market_type = $2 order by created_at desc", exchange, marketType)
	return subscriptions, err
}

func (r *KlineSubscriptionRepository) GetByExchangeMarketTypeSymbolInterval(ctx context.Context, exchange, marketType, symbol, interval string) (*entity.KlineSubscription, error) {
	var sub entity.KlineSubscription
	err := r.db.GetContext(ctx, &sub,
		"SELECT * FROM kline_subscriptions WHERE exchange = $1 AND market_type = $2 AND symbol = $3 AND interval = $4 LIMIT 1",
		exchange, marketType, symbol, interval,
	)
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (r *KlineSubscriptionRepository) GetLatestUpdatedAtByExchange(ctx context.Context, exchange string) (time.Time, error) {
	var updatedAt sql.NullTime
	err := r.db.GetContext(ctx, &updatedAt, "SELECT MAX(updated_at) FROM kline_subscriptions WHERE exchange = $1", exchange)
	if err != nil {
		return time.Time{}, err
	}

	if !updatedAt.Valid {
		return time.Time{}, nil
	}

	return updatedAt.Time, nil
}

func (r *KlineSubscriptionRepository) GetLatestUpdatedAtByExchangeAndMarketType(ctx context.Context, exchange, marketType string) (time.Time, error) {
	var updatedAt sql.NullTime
	err := r.db.GetContext(ctx, &updatedAt, "SELECT MAX(updated_at) FROM kline_subscriptions WHERE exchange = $1 AND market_type = $2", exchange, marketType)
	if err != nil {
		return time.Time{}, err
	}

	if !updatedAt.Valid {
		return time.Time{}, nil
	}

	return updatedAt.Time, nil
}

func (r *KlineSubscriptionRepository) FindByID(ctx context.Context, id string) (*entity.KlineSubscription, error) {
	var item entity.KlineSubscription
	err := r.db.GetContext(ctx, &item, "SELECT * FROM kline_subscriptions WHERE id = $1 LIMIT 1", id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *KlineSubscriptionRepository) Create(ctx context.Context, item *entity.KlineSubscription) error {
	query := `INSERT INTO kline_subscriptions (exchange, market_type, symbol, interval, payload, max_kline_data, created_at, updated_at)
			  VALUES (:exchange, :market_type, :symbol, :interval, :payload, :max_kline_data, :created_at, :updated_at)
			  RETURNING id`
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

func (r *KlineSubscriptionRepository) Update(ctx context.Context, item *entity.KlineSubscription) error {
	query := `UPDATE kline_subscriptions
			  SET exchange = :exchange,
				  market_type = :market_type,
				  symbol = :symbol,
				  interval = :interval,
				  payload = :payload,
				  max_kline_data = :max_kline_data,
				  updated_at = :updated_at
			  WHERE id = :id`
	_, err := r.db.NamedExecContext(ctx, query, item)
	return err
}

func (r *KlineSubscriptionRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM kline_subscriptions WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *KlineSubscriptionRepository) GetPagination(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	model := &entity.KlineSubscription{}
	baseSelect := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("*").
		From("kline_subscriptions")
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

	items := []entity.KlineSubscription{}
	if err := r.db.SelectContext(ctx, &items, selectQuery, selectArgs...); err != nil {
		return nil, err
	}

	return apiutil.NewPaginationResp(req.Paginate.Page, req.Paginate.Limit, total, items), nil
}
