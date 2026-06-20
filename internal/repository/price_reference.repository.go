package repository

import (
	"context"
	"database/sql"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/krobus00/hft-service/internal/entity"
)

type PriceReferenceRepository struct {
	db *sqlx.DB
}

func (r *PriceReferenceRepository) FindByID(ctx context.Context, id string) (*entity.PriceReference, error) {
	var item entity.PriceReference
	if err := r.db.GetContext(ctx, &item, "SELECT * FROM price_references WHERE id = $1", id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *PriceReferenceRepository) FindByMarket(ctx context.Context, exchange, marketType, symbol string) (*entity.PriceReference, error) {
	var item entity.PriceReference
	if err := r.db.GetContext(ctx, &item, "SELECT * FROM price_references WHERE exchange = $1 AND market_type = $2 AND symbol = $3", exchange, marketType, symbol); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *PriceReferenceRepository) GetPagination(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	model := &entity.PriceReference{}
	base := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).Select("*").From("price_references")
	base = req.ApplyFilter(base, model)

	countQuery, countArgs, err := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("COUNT(*)").FromSelect(base, "count_query").ToSql()
	if err != nil {
		return nil, err
	}
	var total int64
	if err := r.db.GetContext(ctx, &total, countQuery, countArgs...); err != nil {
		return nil, err
	}

	query, args, err := base.OrderBy(req.Sort.Field + " " + req.Sort.Direction).
		Limit(uint64(req.Paginate.Limit)).Offset(uint64(req.Paginate.Offset)).ToSql()
	if err != nil {
		return nil, err
	}
	items := []entity.PriceReference{}
	if err := r.db.SelectContext(ctx, &items, query, args...); err != nil {
		return nil, err
	}
	return apiutil.NewPaginationResp(req.Paginate.Page, req.Paginate.Limit, total, items), nil
}

func NewPriceReferenceRepository(db *sqlx.DB) *PriceReferenceRepository {
	return &PriceReferenceRepository{db: db}
}

func (r *PriceReferenceRepository) UpsertKline(ctx context.Context, kline entity.MarketKline) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO price_references (exchange, market_type, symbol, price, event_time)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (exchange, market_type, symbol) DO UPDATE SET
    price = EXCLUDED.price,
    event_time = EXCLUDED.event_time,
    updated_at = now()
WHERE price_references.event_time <= EXCLUDED.event_time`,
		kline.Exchange, kline.MarketType, kline.Symbol, kline.ClosePrice, kline.EventTime)
	return err
}
