package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/krobus00/hft-service/internal/repository"
	pb "github.com/krobus00/hft-service/pb/market_data"
)

const (
	BackfillJobPending   = "pending"
	BackfillJobRunning   = "running"
	BackfillJobSucceeded = "succeeded"
	BackfillJobFailed    = "failed"
)

type BackfillService struct {
	client pb.MarketDataServiceClient
	repo   *repository.APIBackfillJobRepository
	mu     sync.RWMutex
	jobs   map[string]*BackfillJob
}

type BackfillRequest struct {
	Exchange   string    `json:"exchange"`
	MarketType string    `json:"market_type"`
	Symbol     string    `json:"symbol"`
	Interval   string    `json:"interval"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
}

type BackfillJob struct {
	ID            string          `json:"id"`
	Status        string          `json:"status"`
	Request       BackfillRequest `json:"request"`
	InsertedCount int64           `json:"inserted_count"`
	Error         string          `json:"error,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	StartedAt     *time.Time      `json:"started_at,omitempty"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

func NewBackfillService(client pb.MarketDataServiceClient, repo *repository.APIBackfillJobRepository) *BackfillService {
	return &BackfillService{
		client: client,
		repo:   repo,
		jobs:   map[string]*BackfillJob{},
	}
}

func (s *BackfillService) Start(req BackfillRequest) (*BackfillJob, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("market data grpc client is not configured")
	}
	req = normalizeBackfillRequest(req)
	if err := req.Validate(); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	job := &BackfillJob{
		ID:        newJobID(),
		Status:    BackfillJobPending,
		Request:   req,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()
	if s.repo != nil {
		if err := s.repo.Create(context.Background(), backfillJobRecord(job)); err != nil {
			return nil, err
		}
	}

	go s.run(job.ID)

	return cloneBackfillJob(job), nil
}

func (s *BackfillService) Get(id string) (*BackfillJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		if s.repo == nil {
			return nil, false
		}
		record, err := s.repo.FindByID(context.Background(), id)
		if err != nil || record == nil {
			return nil, false
		}
		return backfillJobFromRecord(*record), true
	}
	return cloneBackfillJob(job), true
}

func (s *BackfillService) List(limit int) ([]BackfillJob, error) {
	if s.repo == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		items := make([]BackfillJob, 0, len(s.jobs))
		for _, job := range s.jobs {
			items = append(items, *cloneBackfillJob(job))
		}
		return items, nil
	}
	records, err := s.repo.ListRecent(context.Background(), limit)
	if err != nil {
		return nil, err
	}
	items := make([]BackfillJob, 0, len(records))
	for _, record := range records {
		items = append(items, *backfillJobFromRecord(record))
	}
	return items, nil
}

func (s *BackfillService) Wait(ctx context.Context, id string, timeout time.Duration) (*BackfillJob, bool) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		job, ok := s.Get(id)
		if !ok || isBackfillTerminal(job.Status) {
			return job, ok
		}
		select {
		case <-ctx.Done():
			return job, ok
		case <-deadline.C:
			return job, ok
		case <-ticker.C:
		}
	}
}

func (s *BackfillService) run(id string) {
	job, ok := s.markRunning(id)
	if !ok {
		return
	}

	resp, err := s.client.BackfillMarketKlines(context.Background(), &pb.BackfillMarketKlinesRequest{
		Exchange:   job.Request.Exchange,
		MarketType: job.Request.MarketType,
		Symbol:     job.Request.Symbol,
		Interval:   job.Request.Interval,
		StartTime:  job.Request.StartTime.UTC().UnixMilli(),
		EndTime:    job.Request.EndTime.UTC().UnixMilli(),
	})
	if err != nil {
		s.markFailed(id, err)
		return
	}
	s.markSucceeded(id, resp.GetInsertedCount())
}

func (s *BackfillService) markRunning(id string) (*BackfillJob, bool) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	job.Status = BackfillJobRunning
	job.StartedAt = &now
	job.UpdatedAt = now
	s.persistLocked(job)
	return cloneBackfillJob(job), true
}

func (s *BackfillService) markSucceeded(id string, insertedCount int64) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Status = BackfillJobSucceeded
		job.InsertedCount = insertedCount
		job.CompletedAt = &now
		job.UpdatedAt = now
		s.persistLocked(job)
	}
}

func (s *BackfillService) markFailed(id string, err error) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Status = BackfillJobFailed
		job.Error = err.Error()
		job.CompletedAt = &now
		job.UpdatedAt = now
		s.persistLocked(job)
	}
}

func (s *BackfillService) persistLocked(job *BackfillJob) {
	if s.repo != nil {
		_ = s.repo.UpdateStatus(context.Background(), backfillJobRecord(job))
	}
}

func (r BackfillRequest) Validate() error {
	if strings.TrimSpace(r.Exchange) == "" {
		return errors.New("exchange is required")
	}
	if strings.TrimSpace(r.MarketType) == "" {
		return errors.New("market_type is required")
	}
	if strings.TrimSpace(r.Symbol) == "" {
		return errors.New("symbol is required")
	}
	if strings.TrimSpace(r.Interval) == "" {
		return errors.New("interval is required")
	}
	if r.StartTime.IsZero() || r.EndTime.IsZero() {
		return errors.New("start_time and end_time are required")
	}
	if !r.EndTime.After(r.StartTime) {
		return errors.New("end_time must be greater than start_time")
	}
	return nil
}

func normalizeBackfillRequest(req BackfillRequest) BackfillRequest {
	req.Exchange = strings.ToLower(strings.TrimSpace(req.Exchange))
	req.MarketType = strings.ToLower(strings.TrimSpace(req.MarketType))
	req.Symbol = strings.ToUpper(strings.TrimSpace(req.Symbol))
	req.Interval = strings.TrimSpace(req.Interval)
	req.StartTime = req.StartTime.UTC()
	req.EndTime = req.EndTime.UTC()
	return req
}

func isBackfillTerminal(status string) bool {
	return status == BackfillJobSucceeded || status == BackfillJobFailed
}

func cloneBackfillJob(job *BackfillJob) *BackfillJob {
	if job == nil {
		return nil
	}
	copy := *job
	return &copy
}

func backfillJobRecord(job *BackfillJob) repository.APIBackfillJob {
	record := repository.APIBackfillJob{
		ID: job.ID, Status: job.Status, Exchange: job.Request.Exchange, MarketType: job.Request.MarketType,
		Symbol: job.Request.Symbol, Interval: job.Request.Interval, InsertedCount: job.InsertedCount,
		Error:     sql.NullString{String: job.Error, Valid: true},
		StartTime: sql.NullTime{Time: job.Request.StartTime, Valid: true},
		EndTime:   sql.NullTime{Time: job.Request.EndTime, Valid: true},
		CreatedAt: sql.NullTime{Time: job.CreatedAt, Valid: true},
		UpdatedAt: sql.NullTime{Time: job.UpdatedAt, Valid: true},
	}
	if job.StartedAt != nil {
		record.StartedAt = sql.NullTime{Time: *job.StartedAt, Valid: true}
	}
	if job.CompletedAt != nil {
		record.CompletedAt = sql.NullTime{Time: *job.CompletedAt, Valid: true}
	}
	return record
}

func backfillJobFromRecord(record repository.APIBackfillJob) *BackfillJob {
	job := &BackfillJob{
		ID: record.ID, Status: record.Status, InsertedCount: record.InsertedCount,
		Error:     record.Error.String,
		Request:   BackfillRequest{Exchange: record.Exchange, MarketType: record.MarketType, Symbol: record.Symbol, Interval: record.Interval},
		CreatedAt: record.CreatedAt.Time, UpdatedAt: record.UpdatedAt.Time,
	}
	if record.StartTime.Valid {
		job.Request.StartTime = record.StartTime.Time
	}
	if record.EndTime.Valid {
		job.Request.EndTime = record.EndTime.Time
	}
	if record.StartedAt.Valid {
		job.StartedAt = &record.StartedAt.Time
	}
	if record.CompletedAt.Valid {
		job.CompletedAt = &record.CompletedAt.Time
	}
	return job
}

func newJobID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
