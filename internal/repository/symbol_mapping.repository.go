package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
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

		indexes, ok := exchangeSymbolMapping[exchange]
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

		exchangeSymbolMapping[exchange] = indexes
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
