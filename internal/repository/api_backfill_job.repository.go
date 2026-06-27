package repository

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
)

type APIBackfillJob struct {
	ID            string         `db:"id"`
	Status        string         `db:"status"`
	Exchange      string         `db:"exchange"`
	MarketType    string         `db:"market_type"`
	Symbol        string         `db:"symbol"`
	Interval      string         `db:"interval"`
	StartTime     sql.NullTime   `db:"start_time"`
	EndTime       sql.NullTime   `db:"end_time"`
	InsertedCount int64          `db:"inserted_count"`
	Error         sql.NullString `db:"error"`
	CreatedAt     sql.NullTime   `db:"created_at"`
	StartedAt     sql.NullTime   `db:"started_at"`
	CompletedAt   sql.NullTime   `db:"completed_at"`
	UpdatedAt     sql.NullTime   `db:"updated_at"`
}

type APIBackfillJobRepository struct {
	db *sqlx.DB
}

func NewAPIBackfillJobRepository(db *sqlx.DB) *APIBackfillJobRepository {
	return &APIBackfillJobRepository{db: db}
}

func (r *APIBackfillJobRepository) Create(ctx context.Context, job APIBackfillJob) error {
	_, err := r.db.NamedExecContext(ctx, `
INSERT INTO market_backfill_jobs (
	id, status, exchange, market_type, symbol, interval, start_time, end_time,
	inserted_count, error, created_at, updated_at
) VALUES (
	:id, :status, :exchange, :market_type, :symbol, :interval, :start_time, :end_time,
	:inserted_count, :error, :created_at, :updated_at
)`, job)
	return err
}

func (r *APIBackfillJobRepository) FindByID(ctx context.Context, id string) (*APIBackfillJob, error) {
	var job APIBackfillJob
	err := r.db.GetContext(ctx, &job, "SELECT * FROM market_backfill_jobs WHERE id = $1 LIMIT 1", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &job, nil
}

func (r *APIBackfillJobRepository) ListRecent(ctx context.Context, limit int) ([]APIBackfillJob, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	items := []APIBackfillJob{}
	err := r.db.SelectContext(ctx, &items, `
SELECT *
FROM market_backfill_jobs
ORDER BY updated_at DESC
LIMIT $1`, limit)
	return items, err
}

func (r *APIBackfillJobRepository) UpdateStatus(ctx context.Context, job APIBackfillJob) error {
	_, err := r.db.NamedExecContext(ctx, `
UPDATE market_backfill_jobs
SET status = :status,
    inserted_count = :inserted_count,
    error = :error,
    started_at = :started_at,
    completed_at = :completed_at,
    updated_at = :updated_at
WHERE id = :id`, job)
	return err
}
