package repository

import (
	"context"
	"database/sql"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/krobus00/hft-service/internal/entity"
)

type MarketKlineRepository struct {
	db *sqlx.DB
}

func NewMarketKlineRepository(db *sqlx.DB) *MarketKlineRepository {
	return &MarketKlineRepository{db: db}
}

func (r *MarketKlineRepository) Create(ctx context.Context, data *entity.MarketKline) error {
	queryBuilder := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Insert(data.TableName()).
		Columns(
			"id",
			"exchange",
			"market_type",
			"event_type",
			"event_time",
			"symbol",
			"interval",
			"open_time",
			"close_time",
			"open_price",
			"high_price",
			"low_price",
			"close_price",
			"base_volume",
			"quote_volume",
			"taker_base_volume",
			"taker_quote_volume",
			"trade_count",
			"is_closed",
			"created_at",
			"updated_at",
		).
		Values(
			nullDefaultString(data.ID),
			data.Exchange,
			data.MarketType,
			data.EventType,
			data.EventTime,
			data.Symbol,
			data.Interval,
			data.OpenTime,
			data.CloseTime,
			data.OpenPrice,
			data.HighPrice,
			data.LowPrice,
			data.ClosePrice,
			data.BaseVolume,
			data.QuoteVolume,
			data.TakerBaseVolume,
			data.TakerQuoteVolume,
			data.TradeCount,
			data.IsClosed,
			data.CreatedAt,
			data.UpdatedAt,
		).
		Suffix(`ON CONFLICT (exchange, market_type, symbol, interval, open_time)
DO UPDATE SET
	event_type = EXCLUDED.event_type,
	event_time = EXCLUDED.event_time,
	close_time = EXCLUDED.close_time,
	open_price = EXCLUDED.open_price,
	high_price = EXCLUDED.high_price,
	low_price = EXCLUDED.low_price,
	close_price = EXCLUDED.close_price,
	base_volume = EXCLUDED.base_volume,
	quote_volume = EXCLUDED.quote_volume,
	taker_base_volume = EXCLUDED.taker_base_volume,
	taker_quote_volume = EXCLUDED.taker_quote_volume,
	trade_count = EXCLUDED.trade_count,
	is_closed = EXCLUDED.is_closed,
	updated_at = EXCLUDED.updated_at
RETURNING id`)

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return err
	}

	err = r.db.QueryRowContext(ctx, query, args...).Scan(&data.ID)
	return err
}

// DeleteOldKlines removes the oldest kline rows for a given subscription key
// when the total count exceeds maxCount, keeping the most recent maxCount rows by open_time.
//
// Strategy: find the boundary open_time at position maxCount (0-indexed OFFSET) using the
// idx_kline_lookup index, then delete every row older than that boundary in a single range
// sweep on the same index. This avoids materializing a keep-list and performs two narrow
// index seeks instead of an anti-join. If total rows <= maxCount the subquery returns NULL
// and nothing is deleted.
func (r *MarketKlineRepository) DeleteOldKlines(ctx context.Context, exchange, marketType, symbol, interval string, maxCount int) error {
	if maxCount <= 0 {
		return nil
	}
	const query = `
DELETE FROM market_klines
WHERE exchange = $1
  AND market_type = $2
  AND symbol = $3
  AND interval = $4
  AND open_time < (
      SELECT open_time
      FROM market_klines
      WHERE exchange = $1
        AND market_type = $2
        AND symbol = $3
        AND interval = $4
      ORDER BY open_time DESC
      OFFSET $5
      LIMIT 1
  )`
	_, err := r.db.ExecContext(ctx, query, exchange, marketType, symbol, interval, maxCount)
	return err
}

func (r *MarketKlineRepository) FindByOpenTime(ctx context.Context, openTime string) (*entity.MarketKline, error) {
	var item entity.MarketKline
	err := r.db.GetContext(ctx, &item, "SELECT * FROM market_klines WHERE open_time::text = $1 LIMIT 1", openTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *MarketKlineRepository) ListExchanges(ctx context.Context) ([]string, error) {
	items := []string{}
	err := r.db.SelectContext(ctx, &items, "SELECT DISTINCT exchange FROM market_klines WHERE exchange <> '' ORDER BY exchange")
	return items, err
}

func (r *MarketKlineRepository) FindByID(ctx context.Context, id string) (*entity.MarketKline, error) {
	var item entity.MarketKline
	err := r.db.GetContext(ctx, &item, "SELECT * FROM market_klines WHERE id = $1 LIMIT 1", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *MarketKlineRepository) Update(ctx context.Context, data *entity.MarketKline) error {
	query := `UPDATE market_klines
SET exchange = :exchange,
    market_type = :market_type,
    event_type = :event_type,
    event_time = :event_time,
    symbol = :symbol,
    interval = :interval,
    open_time = :open_time,
    close_time = :close_time,
    open_price = :open_price,
    high_price = :high_price,
    low_price = :low_price,
    close_price = :close_price,
    base_volume = :base_volume,
    quote_volume = :quote_volume,
    taker_base_volume = :taker_base_volume,
    taker_quote_volume = :taker_quote_volume,
    trade_count = :trade_count,
    is_closed = :is_closed,
    updated_at = :updated_at
WHERE id = :id`
	result, err := r.db.NamedExecContext(ctx, query, data)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *MarketKlineRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM market_klines WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *MarketKlineRepository) GetPagination(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	model := &entity.MarketKline{}
	baseSelect := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("*").
		From("market_klines")
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

	items := []entity.MarketKline{}
	if err := r.db.SelectContext(ctx, &items, selectQuery, selectArgs...); err != nil {
		return nil, err
	}

	return apiutil.NewPaginationResp(req.Paginate.Page, req.Paginate.Limit, total, items), nil
}

func nullDefaultString(value string) any {
	if value == "" {
		return sq.Expr("DEFAULT")
	}
	return value
}
