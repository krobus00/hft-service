package grpc

import (
	"context"

	"github.com/guregu/null/v6"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/service/orderengine"
	pb "github.com/krobus00/hft-service/pb/order_engine"
	"github.com/shopspring/decimal"
)

type Server struct {
	orderEngineService *orderengine.OrderEngineService
	pb.UnimplementedOrderEngineServiceServer
}

func NewOrderEngineGRPCServer(orderEngineService *orderengine.OrderEngineService) *Server {
	return &Server{
		orderEngineService: orderEngineService,
	}
}

func (s *Server) PlaceOrder(ctx context.Context, req *pb.PlaceOrderRequest) (*pb.PlaceOrderResponse, error) {
	price, err := decimal.NewFromString(req.GetPrice())
	if err != nil {
		return nil, err
	}
	quantity, err := decimal.NewFromString(req.GetQuantity())
	if err != nil {
		return nil, err
	}
	orderHistory, err := s.orderEngineService.PlaceOrder(ctx, entity.OrderRequest{
		RequestID:      req.GetRequestId(),
		UserID:         req.GetUserId(),
		Exchange:       req.GetExchange(),
		OrderID:        null.NewString(req.GetOrderId(), req.GetOrderId() != "").Ptr(),
		Symbol:         req.GetSymbol(),
		Type:           entity.OrderType(req.GetType()),
		Side:           entity.OrderSide(req.GetSide()),
		Price:          price,
		Quantity:       quantity,
		RequestedAt:    req.GetExpiredAt(),
		ExpiredAt:      null.NewInt(req.GetExpiredAt(), req.GetExpiredAt() != 0).Ptr(),
		Source:         req.GetSource(),
		StrategyID:     null.NewString(req.GetStrategyId(), req.GetStrategyId() != "").Ptr(),
		IsPaperTrading: req.GetIsPaperTrading(),
	})
	if err != nil {
		return nil, err
	}

	return &pb.PlaceOrderResponse{
		Id:                orderHistory.ID,
		RequestId:         orderHistory.RequestID,
		UserId:            orderHistory.UserID,
		Exchange:          orderHistory.Exchange,
		Symbol:            orderHistory.Symbol,
		OrderId:           orderHistory.OrderID,
		ClientOrderId:     null.NewString(orderHistory.ClientOrderID.String, orderHistory.ClientOrderID.Valid).Ptr(),
		Side:              string(orderHistory.Side),
		Type:              string(orderHistory.Type),
		Price:             null.NewString(orderHistory.Price.String(), orderHistory.Price != nil).Ptr(),
		Quantity:          orderHistory.Quantity.String(),
		FilledQuantity:    orderHistory.FilledQuantity.String(),
		AvgFillPrice:      null.NewString(orderHistory.AvgFillPrice.String(), orderHistory.AvgFillPrice != nil).Ptr(),
		Status:            orderHistory.Status,
		Leverage:          null.NewString(orderHistory.Leverage.String(), orderHistory.Leverage != nil).Ptr(),
		Fee:               null.NewString(orderHistory.Fee.String(), orderHistory.Fee != nil).Ptr(),
		RealizedPnl:       null.NewString(orderHistory.RealizedPnl.String(), orderHistory.RealizedPnl != nil).Ptr(),
		CreatedAtExchange: null.NewInt(orderHistory.CreatedAtExchange.Time.UnixMilli(), orderHistory.CreatedAtExchange.Valid).Ptr(),
		SentAt:            null.NewInt(orderHistory.SentAt.Time.UnixMilli(), orderHistory.SentAt.Valid).Ptr(),
		AcknowledgedAt:    null.NewInt(orderHistory.AcknowledgedAt.Time.UnixMilli(), orderHistory.AcknowledgedAt.Valid).Ptr(),
		FilledAt:          null.NewInt(orderHistory.FilledAt.Time.UnixMilli(), orderHistory.FilledAt.Valid).Ptr(),
		StrategyId:        null.NewString(orderHistory.StrategyID.String, orderHistory.StrategyID.Valid).Ptr(),
		ErrorMessage:      null.NewString(orderHistory.ErrorMessage.String, orderHistory.ErrorMessage.Valid).Ptr(),
		CreatedAt:         orderHistory.CreatedAt.Unix(),
		UpdatedAt:         orderHistory.UpdatedAt.Unix(),
		IsPaperTrading:    orderHistory.IsPaperTrading,
	}, nil
}
