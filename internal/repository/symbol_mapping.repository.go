package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/krobus00/hft-service/internal/entity"
)

type SymbolMappingRepository struct {
	db *sqlx.DB
}

func NewSymbolMappingRepository(db *sqlx.DB) *SymbolMappingRepository {
	return &SymbolMappingRepository{db: db}
}

func (r *SymbolMappingRepository) GetAll(ctx context.Context) (entity.ExchangeSymbolMapping, error) {
	var mappings []entity.SymbolMapping
	err := r.db.SelectContext(ctx, &mappings, "SELECT * FROM symbol_mappings order by created_at desc")
	if err != nil {
		return nil, err
	}

	return buildExchangeSymbolMapping(mappings), nil
}

func (r *SymbolMappingRepository) GetByExchange(ctx context.Context, exchange string) (entity.ExchangeSymbolMapping, error) {
	var mappings []entity.SymbolMapping
	err := r.db.SelectContext(ctx, &mappings, "SELECT * FROM symbol_mappings WHERE exchange = $1 order by created_at desc", exchange)
	if err != nil {
		return nil, err
	}

	return buildExchangeSymbolMapping(mappings), nil
}

func buildExchangeSymbolMapping(mappings []entity.SymbolMapping) entity.ExchangeSymbolMapping {
	exchangeSymbolMapping := make(entity.ExchangeSymbolMapping)

	for _, mapping := range mappings {
		exchange := strings.TrimSpace(mapping.Exchange)
		if exchange == "" {
			continue
		}
		marketType := string(entity.NormalizeMarketType(mapping.MarketType))

		marketTypeMapping, ok := exchangeSymbolMapping[exchange]
		if !ok {
			marketTypeMapping = make(map[string]entity.ExchangeSymbols)
		}

		indexes, ok := marketTypeMapping[marketType]
		if !ok {
			indexes = entity.ExchangeSymbols{
				InternalToKline: make(map[string]string),
				InternalToOrder: make(map[string]string),
				KlineToInternal: make(map[string]string),
				OrderToInternal: make(map[string]string),
			}
		}

		internalSymbol := strings.ToUpper(strings.TrimSpace(mapping.Symbol))
		klineSymbol := strings.TrimSpace(mapping.KlineSymbol)
		orderSymbol := strings.TrimSpace(mapping.OrderSymbol)

		if internalSymbol != "" {
			if klineSymbol != "" {
				indexes.InternalToKline[internalSymbol] = klineSymbol
				indexes.KlineToInternal[strings.ToUpper(klineSymbol)] = internalSymbol
			}

			if orderSymbol != "" {
				indexes.InternalToOrder[internalSymbol] = orderSymbol
				indexes.OrderToInternal[strings.ToUpper(orderSymbol)] = internalSymbol
			}
		}

		marketTypeMapping[marketType] = indexes
		exchangeSymbolMapping[exchange] = marketTypeMapping
	}

	return exchangeSymbolMapping
}

func (r *SymbolMappingRepository) GetLatestUpdatedAtByExchange(ctx context.Context, exchange string) (time.Time, error) {
	var updatedAt sql.NullTime
	err := r.db.GetContext(ctx, &updatedAt, "SELECT MAX(updated_at) FROM symbol_mappings WHERE exchange = $1", exchange)
	if err != nil {
		return time.Time{}, err
	}

	if !updatedAt.Valid {
		return time.Time{}, nil
	}

	return updatedAt.Time, nil
}

func (r *SymbolMappingRepository) GetLatestUpdatedAtByExchangeAndMarketType(ctx context.Context, exchange, marketType string) (time.Time, error) {
	var updatedAt sql.NullTime
	err := r.db.GetContext(ctx, &updatedAt, "SELECT MAX(updated_at) FROM symbol_mappings WHERE exchange = $1 AND market_type = $2", exchange, marketType)
	if err != nil {
		return time.Time{}, err
	}

	if !updatedAt.Valid {
		return time.Time{}, nil
	}

	return updatedAt.Time, nil
}

func (r *SymbolMappingRepository) FindByID(ctx context.Context, id string) (*entity.SymbolMapping, error) {
	var item entity.SymbolMapping
	err := r.db.GetContext(ctx, &item, "SELECT * FROM symbol_mappings WHERE id = $1 LIMIT 1", id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *SymbolMappingRepository) Create(ctx context.Context, item *entity.SymbolMapping) error {
	query := `INSERT INTO symbol_mappings (exchange, market_type, symbol, kline_symbol, order_symbol, created_at, updated_at)
			  VALUES (:exchange, :market_type, :symbol, :kline_symbol, :order_symbol, :created_at, :updated_at)
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

func (r *SymbolMappingRepository) Update(ctx context.Context, item *entity.SymbolMapping) error {
	query := `UPDATE symbol_mappings
			  SET exchange = :exchange,
				  market_type = :market_type,
				  symbol = :symbol,
				  kline_symbol = :kline_symbol,
				  order_symbol = :order_symbol,
				  updated_at = :updated_at
			  WHERE id = :id`
	_, err := r.db.NamedExecContext(ctx, query, item)
	return err
}

func (r *SymbolMappingRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM symbol_mappings WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *SymbolMappingRepository) GetPagination(ctx context.Context, req *apiutil.PaginationReq) (*apiutil.PaginationResp, error) {
	model := &entity.SymbolMapping{}
	baseSelect := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select("*").
		From("symbol_mappings")
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

	items := []entity.SymbolMapping{}
	if err := r.db.SelectContext(ctx, &items, selectQuery, selectArgs...); err != nil {
		return nil, err
	}

	return apiutil.NewPaginationResp(req.Paginate.Page, req.Paginate.Limit, total, items), nil
}
