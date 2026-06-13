package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

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

func NewBackfillService(client pb.MarketDataServiceClient) *BackfillService {
	return &BackfillService{
		client: client,
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

	go s.run(job.ID)

	return cloneBackfillJob(job), nil
}

func (s *BackfillService) Get(id string) (*BackfillJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	return cloneBackfillJob(job), true
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

func newJobID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
