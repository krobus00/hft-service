package repository

import (
	"context"

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

	exchangeSymbolMapping := make(entity.ExchangeSymbolMapping)
	for _, mapping := range mappings {
		if _, ok := exchangeSymbolMapping[mapping.Exchange]; !ok {
			exchangeSymbolMapping[mapping.Exchange] = make(map[string]string)
		}
		exchangeSymbolMapping[mapping.Exchange][mapping.Symbol] = mapping.KlineSymbol
	}

	return exchangeSymbolMapping, nil
}

func (r *SymbolMappingRepository) GetByExchange(ctx context.Context, exchange string) (entity.ExchangeSymbolMapping, error) {
	var mappings []entity.SymbolMapping
	err := r.db.SelectContext(ctx, &mappings, "SELECT * FROM symbol_mappings WHERE exchange = $1 order by created_at desc", exchange)
	if err != nil {
		return nil, err
	}

	exchangeSymbolMapping := make(entity.ExchangeSymbolMapping)
	for _, mapping := range mappings {
		if _, ok := exchangeSymbolMapping[mapping.Exchange]; !ok {
			exchangeSymbolMapping[mapping.Exchange] = make(map[string]string)
		}
		exchangeSymbolMapping[mapping.Exchange][mapping.Symbol] = mapping.KlineSymbol
	}

	return exchangeSymbolMapping, nil
}
