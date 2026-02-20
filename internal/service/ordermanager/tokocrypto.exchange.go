package ordermanager

import (
	"context"
	"encoding/json"

	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

type TokocryptoExchange struct{}

func NewTokocryptoExchange() *TokocryptoExchange {
	return &TokocryptoExchange{}
}

func (e *TokocryptoExchange) HandleKlineData(ctx context.Context, message []byte) (KlineData, error) {
	var payload struct {
		Stream string `json:"stream"`
		Data   struct {
			Event     string `json:"e"`
			EventTime int64  `json:"E"`
			Kline     struct {
				Close    string `json:"c"`
				IsClosed bool   `json:"x"`
			} `json:"k"`
		} `json:"data"`
	}

	if err := json.Unmarshal(message, &payload); err != nil {
		return KlineData{}, err
	}

	if payload.Data.Event != "kline" || payload.Data.Kline.Close == "" {
		return KlineData{}, nil
	}

	price, err := decimal.NewFromString(payload.Data.Kline.Close)
	if err != nil {
		return KlineData{}, err
	}

	return KlineData{
		Close:    price,
		IsClosed: payload.Data.Kline.IsClosed,
	}, nil
}

func (e *TokocryptoExchange) PlaceOrder(ctx context.Context, order OrderRequest) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	logrus.WithFields(logrus.Fields{
		"exchange": order.Exchange,
		"symbol":   order.Symbol,
		"type":     order.Type,
		"side":     order.Side,
		"price":    order.Price.String(),
		"quantity": order.Quantity.String(),
		"source":   order.Source,
	}).Info("order placed")

	return nil
}
