package grpc

import (
	"context"
	"strings"
	"time"

	"github.com/krobus00/hft-service/internal/entity"
	pb "github.com/krobus00/hft-service/pb/market_data"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	exchanges map[entity.ExchangeName]entity.Exchange
	pb.UnimplementedMarketDataServiceServer
}

func NewMarketDataGRPCServer(exchanges map[entity.ExchangeName]entity.Exchange) *Server {
	return &Server{exchanges: exchanges}
}

func (s *Server) BackfillMarketKlines(ctx context.Context, req *pb.BackfillMarketKlinesRequest) (*pb.BackfillMarketKlinesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	exchange := entity.ExchangeName(strings.ToLower(strings.TrimSpace(req.GetExchange())))
	if exchange == "" {
		return nil, status.Error(codes.InvalidArgument, "exchange is required")
	}

	symbol := strings.ToUpper(strings.TrimSpace(req.GetSymbol()))
	if symbol == "" {
		return nil, status.Error(codes.InvalidArgument, "symbol is required")
	}

	interval := strings.TrimSpace(req.GetInterval())
	if interval == "" {
		return nil, status.Error(codes.InvalidArgument, "interval is required")
	}

	if req.GetStartTime() <= 0 || req.GetEndTime() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "start_time and end_time must be unix milliseconds")
	}
	if req.GetEndTime() <= req.GetStartTime() {
		return nil, status.Error(codes.InvalidArgument, "end_time must be greater than start_time")
	}

	exchangeClient, ok := s.exchanges[exchange]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "exchange %s is not configured", exchange)
	}

	insertedCount, err := exchangeClient.BackfillMarketKlines(ctx, entity.MarketKlineBackfillRequest{
		MarketType: entity.NormalizeMarketType(req.GetMarketType()),
		Symbol:    symbol,
		Interval:  interval,
		StartTime: time.UnixMilli(req.GetStartTime()).UTC(),
		EndTime:   time.UnixMilli(req.GetEndTime()).UTC(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "backfill market klines failed: %v", err)
	}

	return &pb.BackfillMarketKlinesResponse{InsertedCount: int64(insertedCount)}, nil
}
