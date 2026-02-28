package http

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/guregu/null/v6"
	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/service/orderengine"
	"github.com/shopspring/decimal"
)

var (
	errAPIKeyMissing  = errors.New("api key is required")
	errAPIKeyInvalid  = errors.New("invalid api key")
	errAPIKeyInactive = errors.New("api key is inactive")
	errAPIKeyExpired  = errors.New("api key is expired")
)

type PlaceOrderRequest struct {
	ApiKey         string `json:"api_key"`
	RequestID      string `json:"request_id"`
	UserID         string `json:"user_id"`
	OrderID        string `json:"order_id"`
	Exchange       string `json:"exchange"`
	Symbol         string `json:"symbol"`
	Type           string `json:"type"`
	Side           string `json:"side"`
	Price          string `json:"price"`
	Quantity       string `json:"quantity"`
	RequestedAt    int64  `json:"requested_at"`
	ExpiredAt      int64  `json:"expired_at"`
	Source         string `json:"source"`
	StrategyID     string `json:"strategy_id"`
	IsPaperTrading bool   `json:"is_paper_trading"`
}

type PlaceOrderResponse struct {
	ID                string  `json:"id"`
	RequestID         string  `json:"request_id"`
	UserID            string  `json:"user_id"`
	Exchange          string  `json:"exchange"`
	Symbol            string  `json:"symbol"`
	OrderID           string  `json:"order_id"`
	ClientOrderID     *string `json:"client_order_id,omitempty"`
	Side              string  `json:"side"`
	Type              string  `json:"type"`
	Price             *string `json:"price,omitempty"`
	Quantity          string  `json:"quantity"`
	FilledQuantity    string  `json:"filled_quantity"`
	AvgFillPrice      *string `json:"avg_fill_price,omitempty"`
	Status            string  `json:"status"`
	Leverage          *string `json:"leverage,omitempty"`
	Fee               *string `json:"fee,omitempty"`
	RealizedPnl       *string `json:"realized_pnl,omitempty"`
	CreatedAtExchange *int64  `json:"created_at_exchange,omitempty"`
	SentAt            *int64  `json:"sent_at,omitempty"`
	AcknowledgedAt    *int64  `json:"acknowledged_at,omitempty"`
	FilledAt          *int64  `json:"filled_at,omitempty"`
	StrategyID        *string `json:"strategy_id,omitempty"`
	ErrorMessage      *string `json:"error_message,omitempty"`
	CreatedAt         int64   `json:"created_at"`
	UpdatedAt         int64   `json:"updated_at"`
	IsPaperTrading    bool    `json:"is_paper_trading"`
}

type PlaceOrderAsyncResponse struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
}

type Handler struct {
	orderEngineService *orderengine.OrderEngineService
}

func NewOrderEngineHTTPHandler(orderEngineService *orderengine.OrderEngineService) *Handler {
	return &Handler{orderEngineService: orderEngineService}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/order-engine/v1/orders", h.PlaceOrder)
	mux.HandleFunc("/order-engine/v1/orders/async", h.PlaceOrderAsync)
}

func (h *Handler) PlaceOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	defer r.Body.Close()

	var req PlaceOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	if err := validateAPIKey(resolveAPIKey(r, &req)); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}

	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.UserID) == "" || strings.TrimSpace(req.Exchange) == "" || strings.TrimSpace(req.Symbol) == "" || strings.TrimSpace(req.Type) == "" || strings.TrimSpace(req.Side) == "" || strings.TrimSpace(req.Quantity) == "" || strings.TrimSpace(req.Source) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing required fields"})
		return
	}

	orderReq, err := mapHTTPRequestToOrderRequest(&req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	history, err := h.orderEngineService.PlaceOrder(r.Context(), orderReq)
	if err != nil {
		switch {
		case errors.Is(err, orderengine.ErrDuplicateOrder):
			writeJSON(w, http.StatusConflict, map[string]any{"error": "duplicate request"})
		case errors.Is(err, orderengine.ErrExchangeNotFound):
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "exchange not found"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		}
		return
	}

	resp := mapOrderHistoryToHTTPResponse(history)
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) PlaceOrderAsync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	defer r.Body.Close()

	var req PlaceOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	if err := validateAPIKey(resolveAPIKey(r, &req)); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}

	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.UserID) == "" || strings.TrimSpace(req.Exchange) == "" || strings.TrimSpace(req.Symbol) == "" || strings.TrimSpace(req.Type) == "" || strings.TrimSpace(req.Side) == "" || strings.TrimSpace(req.Quantity) == "" || strings.TrimSpace(req.Source) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing required fields"})
		return
	}

	orderReq, err := mapHTTPRequestToOrderRequest(&req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	err = h.orderEngineService.PlaceOrderAsync(r.Context(), orderReq)
	if err != nil {
		switch {
		case errors.Is(err, orderengine.ErrPublishOrderEventFailed):
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		}
		return
	}

	writeJSON(w, http.StatusAccepted, PlaceOrderAsyncResponse{
		RequestID: orderReq.RequestID,
		Status:    "queued",
	})
}

func mapHTTPRequestToOrderRequest(req *PlaceOrderRequest) (entity.OrderRequest, error) {
	price, err := decimal.NewFromString(req.Price)
	if err != nil {
		return entity.OrderRequest{}, errors.New("invalid price")
	}

	quantity, err := decimal.NewFromString(req.Quantity)
	if err != nil {
		return entity.OrderRequest{}, errors.New("invalid quantity")
	}

	return entity.OrderRequest{
		RequestID:      req.RequestID,
		UserID:         req.UserID,
		OrderID:        null.NewString(req.OrderID, req.OrderID != "").Ptr(),
		Exchange:       req.Exchange,
		Symbol:         req.Symbol,
		Type:           entity.OrderType(strings.ToUpper(req.Type)),
		Side:           entity.OrderSide(strings.ToUpper(req.Side)),
		Price:          price,
		Quantity:       quantity,
		RequestedAt:    req.RequestedAt,
		ExpiredAt:      null.NewInt(req.ExpiredAt, req.ExpiredAt != 0).Ptr(),
		Source:         req.Source,
		StrategyID:     null.NewString(req.StrategyID, req.StrategyID != "").Ptr(),
		IsPaperTrading: req.IsPaperTrading,
	}, nil
}

func mapOrderHistoryToHTTPResponse(orderHistory *entity.OrderHistory) *PlaceOrderResponse {
	var price *string
	if orderHistory.Price != nil {
		v := orderHistory.Price.String()
		price = &v
	}

	var avgFillPrice *string
	if orderHistory.AvgFillPrice != nil {
		v := orderHistory.AvgFillPrice.String()
		avgFillPrice = &v
	}

	var leverage *string
	if orderHistory.Leverage != nil {
		v := orderHistory.Leverage.String()
		leverage = &v
	}

	var fee *string
	if orderHistory.Fee != nil {
		v := orderHistory.Fee.String()
		fee = &v
	}

	var realizedPnl *string
	if orderHistory.RealizedPnl != nil {
		v := orderHistory.RealizedPnl.String()
		realizedPnl = &v
	}

	var clientOrderID *string
	if orderHistory.ClientOrderID.Valid {
		v := orderHistory.ClientOrderID.String
		clientOrderID = &v
	}

	var createdAtExchange *int64
	if orderHistory.CreatedAtExchange.Valid {
		v := orderHistory.CreatedAtExchange.Time.UnixMilli()
		createdAtExchange = &v
	}

	var sentAt *int64
	if orderHistory.SentAt.Valid {
		v := orderHistory.SentAt.Time.UnixMilli()
		sentAt = &v
	}

	var acknowledgedAt *int64
	if orderHistory.AcknowledgedAt.Valid {
		v := orderHistory.AcknowledgedAt.Time.UnixMilli()
		acknowledgedAt = &v
	}

	var filledAt *int64
	if orderHistory.FilledAt.Valid {
		v := orderHistory.FilledAt.Time.UnixMilli()
		filledAt = &v
	}

	var strategyID *string
	if orderHistory.StrategyID.Valid {
		v := orderHistory.StrategyID.String
		strategyID = &v
	}

	var errorMessage *string
	if orderHistory.ErrorMessage.Valid {
		v := orderHistory.ErrorMessage.String
		errorMessage = &v
	}

	return &PlaceOrderResponse{
		ID:                orderHistory.ID,
		RequestID:         orderHistory.RequestID,
		UserID:            orderHistory.UserID,
		Exchange:          orderHistory.Exchange,
		Symbol:            orderHistory.Symbol,
		OrderID:           orderHistory.OrderID,
		ClientOrderID:     clientOrderID,
		Side:              string(orderHistory.Side),
		Type:              string(orderHistory.Type),
		Price:             price,
		Quantity:          orderHistory.Quantity.String(),
		FilledQuantity:    orderHistory.FilledQuantity.String(),
		AvgFillPrice:      avgFillPrice,
		Status:            orderHistory.Status,
		Leverage:          leverage,
		Fee:               fee,
		RealizedPnl:       realizedPnl,
		CreatedAtExchange: createdAtExchange,
		SentAt:            sentAt,
		AcknowledgedAt:    acknowledgedAt,
		FilledAt:          filledAt,
		StrategyID:        strategyID,
		ErrorMessage:      errorMessage,
		CreatedAt:         orderHistory.CreatedAt.UnixMilli(),
		UpdatedAt:         orderHistory.UpdatedAt.UnixMilli(),
		IsPaperTrading:    orderHistory.IsPaperTrading,
	}
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func resolveAPIKey(r *http.Request, req *PlaceOrderRequest) string {
	if headerKey := strings.TrimSpace(r.Header.Get("X-API-Key")); headerKey != "" {
		return headerKey
	}

	return strings.TrimSpace(req.ApiKey)
}

func validateAPIKey(rawAPIKey string) error {
	apiKey := strings.TrimSpace(rawAPIKey)
	if apiKey == "" {
		return errAPIKeyMissing
	}

	if config.Env == nil || len(config.Env.APIKeys) == 0 {
		return errAPIKeyInvalid
	}

	now := time.Now().UTC()
	for _, candidate := range config.Env.APIKeys {
		storedKey := strings.TrimSpace(candidate.Key)
		if storedKey == "" {
			continue
		}

		if subtle.ConstantTimeCompare([]byte(apiKey), []byte(storedKey)) != 1 {
			continue
		}

		if !candidate.Active {
			return errAPIKeyInactive
		}

		expiredAt, hasExpiry, err := parseExpiry(candidate.ExpiredAt)
		if err != nil {
			return errAPIKeyInvalid
		}
		if !hasExpiry {
			return nil
		}

		if !now.Before(expiredAt) {
			return errAPIKeyExpired
		}

		return nil
	}

	return errAPIKeyInvalid
}

func parseExpiry(value any) (time.Time, bool, error) {
	if value == nil {
		return time.Time{}, false, nil
	}

	switch v := value.(type) {
	case time.Time:
		if v.IsZero() {
			return time.Time{}, false, nil
		}
		return v.UTC(), true, nil
	case string:
		raw := strings.TrimSpace(v)
		if raw == "" {
			return time.Time{}, false, nil
		}

		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			return parsed.UTC(), true, nil
		}

		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return time.Time{}, false, err
		}

		return parsed.UTC().Add(24 * time.Hour), true, nil
	default:
		return time.Time{}, false, errors.New("unsupported expiry type")
	}
}
