package repository

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
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
			"exchange",
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
			data.Exchange,
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
		Suffix(`ON CONFLICT (exchange, symbol, interval, open_time)
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
	updated_at = EXCLUDED.updated_at`)

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	return err
}
