package exchange

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/constant"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/nats-io/nats.go"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

var tokocryptoClientIDPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type TokocryptoExchange struct {
	apiKey          string
	apiSecret       string
	userCredentials map[string]exchangeCredentials
	baseURL         string
	recvWindow      int64
	httpClient      *http.Client

	symbolPrecisionMu sync.RWMutex
	symbolPrecision   map[string]tokocryptoSymbolPrecision
	symbolMapping     atomic.Value
	js                nats.JetStreamContext
	marketKlineRepo   *repository.MarketKlineRepository
	symbolMappingRepo *repository.SymbolMappingRepository
	klineSubRepo      *repository.KlineSubscriptionRepository
}

type tokocryptoSymbolPrecision struct {
	BasePrecision  int32
	QuotePrecision int32
}

func InitTokocryptoExchange(ctx context.Context, exchangeConfig config.ExchangeConfig, symbolMappingRepo *repository.SymbolMappingRepository, klineSubRepo *repository.KlineSubscriptionRepository, js nats.JetStreamContext, marketKlineRepo *repository.MarketKlineRepository) *TokocryptoExchange {
	symbolMapping, err := symbolMappingRepo.GetByExchange(ctx, string(entity.ExchangeTokoCrypto))
	util.ContinueOrFatal(err)

	baseURL := strings.TrimSpace(os.Getenv("TOKOCRYPTO_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://www.tokocrypto.com"
	}

	newExchange := &TokocryptoExchange{
		apiKey:            strings.TrimSpace(exchangeConfig.APIKey),
		apiSecret:         strings.TrimSpace(exchangeConfig.APISecret),
		userCredentials:   loadExchangeAccounts(exchangeConfig.Accounts),
		baseURL:           strings.TrimRight(baseURL, "/"),
		recvWindow:        recvWindowFromEnv("TOKOCRYPTO_RECV_WINDOW"),
		httpClient:        &http.Client{Timeout: 15 * time.Second},
		js:                js,
		marketKlineRepo:   marketKlineRepo,
		symbolMappingRepo: symbolMappingRepo,
		klineSubRepo:      klineSubRepo,
	}
	persistSymbolMapping(&newExchange.symbolMapping, symbolMapping)

	logrus.WithFields(logrus.Fields{
		"account_count": len(newExchange.userCredentials),
		"base_url":      newExchange.baseURL,
	}).Info("tokocrypto exchange initialized")

	RegisterExchange(entity.ExchangeTokoCrypto, newExchange)

	return newExchange
}

func (e *TokocryptoExchange) credentialsForUser(userID string) (string, string, error) {
	return credentialsForUser(entity.ExchangeTokoCrypto, exchangeCredentials{
		APIKey:    e.apiKey,
		APISecret: e.apiSecret,
	}, e.userCredentials, userID)
}

func (e *TokocryptoExchange) JetstreamEventInit(ctx context.Context) error {
	return ensureKlineStream(ctx, e.js)
}

func (e *TokocryptoExchange) JetstreamEventSubscribe(ctx context.Context) error {
	return subscribeKlineInsertEvents(ctx, e.js, entity.ExchangeTokoCrypto, e.handleKlineDataEvent)
}

func (e *TokocryptoExchange) handleKlineDataEvent(ctx context.Context, msg *nats.Msg) (err error) {
	return insertKlineEvent(ctx, e.marketKlineRepo, msg)
}

func (e *TokocryptoExchange) HandleKlineData(ctx context.Context, message []byte) error {
	var payload struct {
		Stream string `json:"stream"`
		Data   struct {
			Event     string `json:"e"`
			EventTime int64  `json:"E"`
			Symbol    string `json:"s"`
			Kline     struct {
				OpenTime         int64  `json:"t"`
				CloseTime        int64  `json:"T"`
				Symbol           string `json:"s"`
				Interval         string `json:"i"`
				FirstTradeID     int64  `json:"f"`
				LastTradeID      int64  `json:"L"`
				Open             string `json:"o"`
				Close            string `json:"c"`
				High             string `json:"h"`
				Low              string `json:"l"`
				BaseVolume       string `json:"v"`
				TradeCount       int32  `json:"n"`
				IsClosed         bool   `json:"x"`
				QuoteVolume      string `json:"q"`
				TakerBaseVolume  string `json:"V"`
				TakerQuoteVolume string `json:"Q"`
			} `json:"k"`
		} `json:"data"`
	}

	if err := json.Unmarshal(message, &payload); err != nil {
		return err
	}

	if payload.Data.Event != "kline" || payload.Data.Kline.Close == "" {
		return nil
	}

	openPrice, err := decimal.NewFromString(payload.Data.Kline.Open)
	if err != nil {
		return fmt.Errorf("invalid open price: %w", err)
	}

	closePrice, err := decimal.NewFromString(payload.Data.Kline.Close)
	if err != nil {
		return fmt.Errorf("invalid close price: %w", err)
	}

	highPrice, err := decimal.NewFromString(payload.Data.Kline.High)
	if err != nil {
		return fmt.Errorf("invalid high price: %w", err)
	}

	lowPrice, err := decimal.NewFromString(payload.Data.Kline.Low)
	if err != nil {
		return fmt.Errorf("invalid low price: %w", err)
	}

	baseVolume, err := decimal.NewFromString(payload.Data.Kline.BaseVolume)
	if err != nil {
		return fmt.Errorf("invalid base volume: %w", err)
	}

	quoteVolume, err := decimal.NewFromString(payload.Data.Kline.QuoteVolume)
	if err != nil {
		return fmt.Errorf("invalid quote volume: %w", err)
	}

	takerBaseVolume, err := decimal.NewFromString(payload.Data.Kline.TakerBaseVolume)
	if err != nil {
		return fmt.Errorf("invalid taker base volume: %w", err)
	}

	takerQuoteVolume, err := decimal.NewFromString(payload.Data.Kline.TakerQuoteVolume)
	if err != nil {
		return fmt.Errorf("invalid taker quote volume: %w", err)
	}

	eventAt := time.UnixMilli(payload.Data.EventTime).UTC()
	openAt := time.UnixMilli(payload.Data.Kline.OpenTime).UTC()
	closeAt := time.UnixMilli(payload.Data.Kline.CloseTime).UTC()
	now := time.Now().UTC()

	symbol := strings.TrimSpace(payload.Data.Kline.Symbol)
	if symbol == "" {
		symbol = strings.TrimSpace(payload.Data.Symbol)
	}
	symbol = resolveInternalSymbolFromMapping(e.klineResyncDeps(), symbol)

	data := entity.MarketKline{
		Exchange:         string(entity.ExchangeTokoCrypto),
		MarketType:       string(entity.MarketTypeSpot),
		EventType:        payload.Data.Event,
		EventTime:        eventAt,
		Symbol:           symbol,
		Interval:         payload.Data.Kline.Interval,
		OpenTime:         openAt,
		CloseTime:        closeAt,
		OpenPrice:        openPrice,
		HighPrice:        highPrice,
		LowPrice:         lowPrice,
		ClosePrice:       closePrice,
		BaseVolume:       baseVolume,
		QuoteVolume:      quoteVolume,
		TakerBaseVolume:  takerBaseVolume,
		TakerQuoteVolume: takerQuoteVolume,
		TradeCount:       payload.Data.Kline.TradeCount,
		IsClosed:         payload.Data.Kline.IsClosed,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	err = util.PublishEvent(e.js, constant.GetKlineStreamSubject(string(entity.ExchangeTokoCrypto), data.Symbol, data.Interval), entity.MarketKlineEvent{
		Data: data,
	})
	if err != nil {
		return err
	}

	return nil
}

func (e *TokocryptoExchange) SubscribeKlineData(ctx context.Context, subscriptions []entity.KlineSubscription) error {
	deps := e.klineResyncDeps()

	logrus.WithFields(logrus.Fields{
		"exchange":           entity.ExchangeTokoCrypto,
		"subscription_count": len(subscriptions),
	}).Info("starting exchange kline subscriptions")

	cfg := newExchangeKlineWSConfig(deps, exchangeKlineWSOptions{
		WSURLEnvKey:        "TOKOCRYPTO_WS_URL",
		DefaultWSURL:       "wss://stream-cloud.tokocrypto.site/stream",
		HandleKlineMessage: e.HandleKlineData,
	})

	err := subscribeKlineDataWithAutoResync(ctx, cfg, subscriptions)
	if err != nil {
		logrus.WithError(err).WithField("exchange", entity.ExchangeTokoCrypto).Warn("exchange kline subscription stopped with error")
		return err
	}

	logrus.WithField("exchange", entity.ExchangeTokoCrypto).Info("exchange kline subscriptions stopped")
	return nil
}

func (e *TokocryptoExchange) klineResyncDeps() exchangeKlineResyncDeps {
	return exchangeKlineResyncDeps{
		ExchangeName:      entity.ExchangeTokoCrypto,
		MarketType:        entity.MarketTypeSpot,
		SymbolMapping:     &e.symbolMapping,
		SymbolMappingRepo: e.symbolMappingRepo,
		KlineSubRepo:      e.klineSubRepo,
	}
}

func (e *TokocryptoExchange) BackfillMarketKlines(ctx context.Context, req entity.MarketKlineBackfillRequest) (int, error) {
	return backfillMarketKlines(ctx, marketKlineBackfillDeps{
		ExchangeName:  entity.ExchangeTokoCrypto,
		MarketType:    entity.MarketTypeSpot,
		BaseURL:       "https://www.tokocrypto.site",
		KlinePath:     "/api/v3/klines",
		HTTPClient:    e.httpClient,
		SymbolMapping: &e.symbolMapping,
	}, e.marketKlineRepo, req)
}

func (e *TokocryptoExchange) PlaceOrder(ctx context.Context, order entity.OrderRequest) (*entity.OrderHistory, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	apiKey, apiSecret, err := e.credentialsForUser(order.UserID)
	if err != nil {
		return nil, err
	}

	deps := e.klineResyncDeps()

	orderSymbol := resolveExchangeOrderSymbolFromMapping(deps, order.Symbol)
	if strings.TrimSpace(orderSymbol) == "" {
		orderSymbol = order.Symbol
	}

	typeCode, err := tokocryptoOrderTypeCode(order.Type)
	if err != nil {
		return nil, err
	}

	sideCode, err := tokocryptoOrderSideCode(order.Side)
	if err != nil {
		return nil, err
	}

	startedAt := time.Now()
	orderLogger := logrus.WithFields(logrus.Fields{
		"exchange":     entity.ExchangeTokoCrypto,
		"market_type":  entity.MarketTypeSpot,
		"request_id":   order.RequestID,
		"user_id":      order.UserID,
		"symbol":       order.Symbol,
		"order_symbol": orderSymbol,
		"type":         order.Type,
		"type_code":    typeCode,
		"side":         order.Side,
		"side_code":    sideCode,
		"source":       order.Source,
	})
	orderLogger.Info("placing exchange order")

	normalizedQuantity := order.Quantity
	normalizedPrice := order.Price

	if precision, ok, err := e.getSymbolPrecision(ctx, orderSymbol); err != nil {
		logrus.WithError(err).WithField("symbol", orderSymbol).Warn("failed to fetch tokocrypto symbol precision")
	} else if ok {
		normalizedQuantity = order.Quantity.Truncate(precision.BasePrecision)
		if !normalizedQuantity.GreaterThan(decimal.Zero) {
			return nil, fmt.Errorf("tokocrypto order quantity becomes zero after normalization: quantity=%s basePrecision=%d", order.Quantity.String(), precision.BasePrecision)
		}

		if order.Type == entity.OrderTypeLimit {
			normalizedPrice = order.Price.Truncate(precision.QuotePrecision)
			if !normalizedPrice.GreaterThan(decimal.Zero) {
				return nil, fmt.Errorf("tokocrypto order price becomes zero after normalization: price=%s quotePrecision=%d", order.Price.String(), precision.QuotePrecision)
			}
		}
	}

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	pairs := []string{
		"symbol=" + orderSymbol,
		"side=" + strconv.Itoa(sideCode),
		"type=" + strconv.Itoa(typeCode),
		"quantity=" + normalizedQuantity.String(),
	}

	if order.OrderID != nil && strings.TrimSpace(*order.OrderID) != "" {
		clientID, err := normalizeTokocryptoClientID(strings.TrimSpace(*order.OrderID))
		if err != nil {
			return nil, err
		}

		pairs = append(pairs, "clientId="+url.QueryEscape(clientID))
	}

	if order.Type == entity.OrderTypeLimit {
		pairs = append(pairs,
			"price="+normalizedPrice.String(),
			"timeInForce=1",
		)
	}

	orderLogger = orderLogger.WithFields(logrus.Fields{
		"quantity":              order.Quantity.String(),
		"normalized_quantity":   normalizedQuantity.String(),
		"price":                 order.Price.String(),
		"normalized_price":      normalizedPrice.String(),
		"requested_at_exchange": order.RequestedAt,
	})
	orderLogger.Info("submitting exchange order")

	pairs = append(pairs,
		"timestamp="+timestamp,
		"recvWindow="+strconv.FormatInt(e.recvWindow, 10),
	)

	payload := strings.Join(pairs, "&")
	signature := signPayload(apiSecret, payload)
	bodyPayload := payload + "&signature=" + signature

	if strings.EqualFold(strings.TrimSpace(os.Getenv("TOKOCRYPTO_DEBUG_SIGN")), "true") {
		logrus.WithFields(logrus.Fields{
			"payload":   payload,
			"signature": signature,
		}).Info("tokocrypto signed payload")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/open/v1/orders", strings.NewReader(bodyPayload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-MBX-APIKEY", apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		orderLogger.WithFields(logrus.Fields{
			"duration_ms": time.Since(startedAt).Milliseconds(),
		}).WithError(err).Warn("exchange order submit failed")
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp struct {
		Code      int             `json:"code"`
		Msg       string          `json:"msg"`
		Message   string          `json:"message"`
		Success   *bool           `json:"success"`
		Timestamp int64           `json:"timestamp"`
		Data      json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("tokocrypto order parse failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	if resp.StatusCode >= http.StatusBadRequest || apiResp.Code != 0 || (apiResp.Success != nil && !*apiResp.Success) {
		errMsg := apiResp.Message
		if errMsg == "" {
			errMsg = apiResp.Msg
		}
		if errMsg == "" {
			errMsg = "unknown error"
		}

		orderLogger.WithFields(logrus.Fields{
			"http_status": resp.StatusCode,
			"code":        apiResp.Code,
			"message":     errMsg,
			"duration_ms": time.Since(startedAt).Milliseconds(),
		}).Info("tokocrypto order rejected")

		return nil, fmt.Errorf("tokocrypto order rejected: status=%d code=%d message=%s", resp.StatusCode, apiResp.Code, errMsg)
	}

	var placeOrderResp entity.TokocryptoPlaceOrderResponse
	if err := json.Unmarshal(apiResp.Data, &placeOrderResp); err != nil {
		return nil, fmt.Errorf("tokocrypto order data parse failed: %w", err)
	}

	orderHistory, err := e.mapPlaceOrderResponseToOrderHistory(order, placeOrderResp)
	if err != nil {
		return nil, err
	}

	orderLogger.WithFields(logrus.Fields{
		"http_status":      resp.StatusCode,
		"response":         string(apiResp.Data),
		"history_order_id": orderHistory.OrderID,
		"history_status":   orderHistory.Status,
		"duration_ms":      time.Since(startedAt).Milliseconds(),
	}).Info("order placed")

	return &orderHistory, nil
}

func (e *TokocryptoExchange) SyncOrderHistory(ctx context.Context, orderHistory entity.OrderHistory) (*entity.OrderHistory, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	apiKey, apiSecret, err := e.credentialsForUser(orderHistory.UserID)
	if err != nil {
		return nil, err
	}

	deps := e.klineResyncDeps()

	orderSymbol := resolveExchangeOrderSymbolFromMapping(deps, orderHistory.Symbol)
	orderSymbol = strings.TrimSpace(orderSymbol)
	if orderSymbol == "" {
		return nil, fmt.Errorf("tokocrypto order symbol is empty")
	}

	if strings.TrimSpace(orderHistory.OrderID) == "" && !orderHistory.ClientOrderID.Valid {
		return nil, fmt.Errorf("tokocrypto order history missing order id")
	}

	startedAt := time.Now()
	logger := logrus.WithFields(logrus.Fields{
		"exchange":        entity.ExchangeTokoCrypto,
		"market_type":     entity.MarketTypeSpot,
		"request_id":      orderHistory.RequestID,
		"user_id":         orderHistory.UserID,
		"symbol":          orderHistory.Symbol,
		"order_symbol":    orderSymbol,
		"order_id":        orderHistory.OrderID,
		"client_order_id": orderHistory.ClientOrderID.String,
		"status":          orderHistory.Status,
	})
	logger.Info("syncing exchange order history")

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	pairs := []string{
		"symbol=" + orderSymbol,
	}

	if strings.TrimSpace(orderHistory.OrderID) != "" {
		pairs = append(pairs, "orderId="+strings.TrimSpace(orderHistory.OrderID))
	} else {
		pairs = append(pairs, "clientId="+url.QueryEscape(strings.TrimSpace(orderHistory.ClientOrderID.String)))
	}

	pairs = append(pairs,
		"timestamp="+timestamp,
		"recvWindow="+strconv.FormatInt(e.recvWindow, 10),
	)

	payload := strings.Join(pairs, "&")
	signature := signPayload(apiSecret, payload)
	endpoint := e.baseURL + "/open/v1/orders/detail?" + payload + "&signature=" + signature

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-MBX-APIKEY", apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp struct {
		Code      int             `json:"code"`
		Msg       string          `json:"msg"`
		Message   string          `json:"message"`
		Success   *bool           `json:"success"`
		Timestamp int64           `json:"timestamp"`
		Data      json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("tokocrypto order detail parse failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	if resp.StatusCode >= http.StatusBadRequest || apiResp.Code != 0 || (apiResp.Success != nil && !*apiResp.Success) {
		errMsg := apiResp.Message
		if errMsg == "" {
			errMsg = apiResp.Msg
		}
		if errMsg == "" {
			errMsg = "unknown error"
		}

		logger.WithFields(logrus.Fields{
			"http_status": resp.StatusCode,
			"code":        apiResp.Code,
			"message":     errMsg,
			"duration_ms": time.Since(startedAt).Milliseconds(),
		}).Info("tokocrypto order detail rejected")

		return nil, fmt.Errorf("tokocrypto order detail rejected: status=%d code=%d message=%s", resp.StatusCode, apiResp.Code, errMsg)
	}

	var detailResp entity.TokocryptoOrderDetailResponse
	if err := json.Unmarshal(apiResp.Data, &detailResp); err != nil {
		return nil, fmt.Errorf("tokocrypto order detail data parse failed: %w", err)
	}

	updatedHistory, err := e.mapOrderHistorySyncResponse(orderHistory, detailResp)
	if err != nil {
		return nil, err
	}

	logger.WithFields(logrus.Fields{
		"http_status":        resp.StatusCode,
		"updated_status":     updatedHistory.Status,
		"filled_quantity":    updatedHistory.FilledQuantity.String(),
		"has_avg_fill_price": updatedHistory.AvgFillPrice != nil,
		"duration_ms":        time.Since(startedAt).Milliseconds(),
	}).Info("exchange order history synced")

	return &updatedHistory, nil
}

func (e *TokocryptoExchange) mapPlaceOrderResponseToOrderHistory(order entity.OrderRequest, resp entity.TokocryptoPlaceOrderResponse) (entity.OrderHistory, error) {
	price, err := decimal.NewFromString(resp.Price)
	if err != nil {
		return entity.OrderHistory{}, fmt.Errorf("invalid tokocrypto order price: %w", err)
	}

	quantity, err := decimal.NewFromString(resp.OrigQty)
	if err != nil {
		return entity.OrderHistory{}, fmt.Errorf("invalid tokocrypto order quantity: %w", err)
	}

	filledQuantity, err := decimal.NewFromString(resp.ExecutedQty)
	if err != nil {
		return entity.OrderHistory{}, fmt.Errorf("invalid tokocrypto filled quantity: %w", err)
	}

	executedPrice, err := decimal.NewFromString(resp.ExecutedPrice)
	if err != nil {
		return entity.OrderHistory{}, fmt.Errorf("invalid tokocrypto executed price: %w", err)
	}

	historySide, err := tokocryptoOrderSideFromCode(resp.Side)
	if err != nil {
		return entity.OrderHistory{}, err
	}

	historyType, err := tokocryptoOrderTypeFromCode(resp.Type)
	if err != nil {
		return entity.OrderHistory{}, err
	}

	if executedPrice.GreaterThan(decimal.Zero) && price.LessThanOrEqual(decimal.Zero) {
		price = executedPrice
	}

	historyStatus := tokocryptoOrderStatusFromCode(resp.Status)
	now := time.Now().UTC()

	clientOrderID := sql.NullString{String: strings.TrimSpace(resp.ClientID), Valid: strings.TrimSpace(resp.ClientID) != ""}
	strategyID := orderStrategyID(order.StrategyID)

	createdAtExchange := sql.NullTime{}
	if resp.CreateTime > 0 {
		createdAtExchange = sql.NullTime{Time: time.UnixMilli(resp.CreateTime).UTC(), Valid: true}
	}

	sentAt := orderSentAt(order.RequestedAt)

	acknowledgedAt := sql.NullTime{Time: now, Valid: true}

	var avgFillPrice *decimal.Decimal
	if executedPrice.GreaterThan(decimal.Zero) {
		avgFillPrice = &executedPrice
	}

	resolvedSymbol := resolveInternalSymbolFromOrderMapping(e.klineResyncDeps(), resp.Symbol)
	if resolvedSymbol == "" {
		resolvedSymbol = order.Symbol
	}

	return entity.OrderHistory{
		RequestID:         order.RequestID,
		UserID:            order.UserID,
		Exchange:          order.Exchange,
		MarketType:        string(entity.MarketTypeSpot),
		PositionSide:      string(entity.PositionSideBoth),
		Symbol:            resolvedSymbol,
		OrderID:           fmt.Sprintf("%d", resp.OrderID),
		ClientOrderID:     clientOrderID,
		Side:              historySide,
		Type:              historyType,
		Price:             &price,
		Quantity:          quantity,
		FilledQuantity:    filledQuantity,
		AvgFillPrice:      avgFillPrice,
		Status:            historyStatus,
		Leverage:          nil,
		Fee:               nil,
		RealizedPnl:       nil,
		CreatedAtExchange: createdAtExchange,
		SentAt:            sentAt,
		AcknowledgedAt:    acknowledgedAt,
		FilledAt:          sql.NullTime{},
		StrategyID:        strategyID,
		TradeCondition:    order.TradeCondition,
		ErrorMessage:      sql.NullString{},
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

func (e *TokocryptoExchange) mapOrderHistorySyncResponse(orderHistory entity.OrderHistory, resp entity.TokocryptoOrderDetailResponse) (entity.OrderHistory, error) {
	filledQuantity, err := decimalOrZero(resp.ExecutedQty)
	if err != nil {
		return entity.OrderHistory{}, fmt.Errorf("invalid tokocrypto filled quantity: %w", err)
	}

	executedPrice, err := decimalOrZero(resp.ExecutedPrice)
	if err != nil {
		return entity.OrderHistory{}, fmt.Errorf("invalid tokocrypto executed price: %w", err)
	}

	status := tokocryptoOrderStatusFromCode(resp.Status)
	now := time.Now().UTC()

	orderHistory.Status = status
	orderHistory.FilledQuantity = filledQuantity
	if executedPrice.GreaterThan(decimal.Zero) {
		orderHistory.AvgFillPrice = &executedPrice
		if orderHistory.Price == nil || orderHistory.Price.LessThanOrEqual(decimal.Zero) {
			price := executedPrice
			orderHistory.Price = &price
		}
	}
	orderHistory.UpdatedAt = now

	if status == "FILLED" && !orderHistory.FilledAt.Valid {
		orderHistory.FilledAt = sql.NullTime{Time: now, Valid: true}
	}

	if strings.TrimSpace(resp.OrderID) != "" && strings.TrimSpace(orderHistory.OrderID) == "" {
		orderHistory.OrderID = resp.OrderID
	}

	if clientID := strings.TrimSpace(resp.ClientID); clientID != "" && !orderHistory.ClientOrderID.Valid {
		orderHistory.ClientOrderID = sql.NullString{String: clientID, Valid: true}
	}

	if resp.CreateTime > 0 && !orderHistory.CreatedAtExchange.Valid {
		orderHistory.CreatedAtExchange = sql.NullTime{Time: time.UnixMilli(resp.CreateTime).UTC(), Valid: true}
	}

	resolvedSymbol := resolveInternalSymbolFromOrderMapping(e.klineResyncDeps(), resp.Symbol)
	if resolvedSymbol != "" {
		orderHistory.Symbol = resolvedSymbol
	}

	return orderHistory, nil
}

func tokocryptoOrderSideFromCode(code int32) (entity.OrderSide, error) {
	switch code {
	case 0:
		return entity.OrderSideBuy, nil
	case 1:
		return entity.OrderSideSell, nil
	default:
		return "", fmt.Errorf("unsupported tokocrypto order side code: %d", code)
	}
}

func tokocryptoOrderTypeFromCode(code int32) (entity.OrderType, error) {
	switch code {
	case 1:
		return entity.OrderTypeLimit, nil
	case 2:
		return entity.OrderTypeMarket, nil
	default:
		return "", fmt.Errorf("unsupported tokocrypto order type code: %d", code)
	}
}

func tokocryptoOrderStatusFromCode(code int32) string {
	switch code {
	case 0:
		return "NEW"
	case 1:
		return "PARTIAL"
	case 2:
		return "FILLED"
	case 3:
		return "CANCELED"
	case 4:
		return "REJECTED"
	default:
		return fmt.Sprintf("UNKNOWN_%d", code)
	}
}

func tokocryptoOrderTypeCode(orderType entity.OrderType) (int, error) {
	switch orderType {
	case entity.OrderTypeLimit:
		return 1, nil
	case entity.OrderTypeMarket:
		return 2, nil
	default:
		return 0, fmt.Errorf("unsupported order type for tokocrypto: %s", orderType)
	}
}

func tokocryptoOrderSideCode(orderSide entity.OrderSide) (int, error) {
	switch orderSide {
	case entity.OrderSideBuy:
		return 0, nil
	case entity.OrderSideSell:
		return 1, nil
	default:
		return 0, fmt.Errorf("unsupported order side for tokocrypto: %s", orderSide)
	}
}

func normalizeTokocryptoClientID(raw string) (string, error) {
	normalized := strings.TrimSpace(raw)
	normalized = strings.ReplaceAll(normalized, "-", "")

	if normalized == "" {
		return "", fmt.Errorf("tokocrypto clientId is empty")
	}

	if len(normalized) > 32 {
		normalized = normalized[:32]
	}

	if !tokocryptoClientIDPattern.MatchString(normalized) {
		return "", fmt.Errorf("tokocrypto clientId contains unsupported characters")
	}

	return normalized, nil
}

func (e *TokocryptoExchange) getSymbolPrecision(ctx context.Context, symbol string) (tokocryptoSymbolPrecision, bool, error) {
	normalizedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	if normalizedSymbol == "" {
		return tokocryptoSymbolPrecision{}, false, nil
	}

	e.symbolPrecisionMu.RLock()
	if precision, exists := e.symbolPrecision[normalizedSymbol]; exists {
		e.symbolPrecisionMu.RUnlock()
		return precision, true, nil
	}
	e.symbolPrecisionMu.RUnlock()

	if err := e.refreshSymbolPrecision(ctx, normalizedSymbol); err != nil {
		return tokocryptoSymbolPrecision{}, false, err
	}

	e.symbolPrecisionMu.RLock()
	precision, exists := e.symbolPrecision[normalizedSymbol]
	e.symbolPrecisionMu.RUnlock()

	return precision, exists, nil
}

func (e *TokocryptoExchange) refreshSymbolPrecision(ctx context.Context, symbol string) error {
	normalizedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	if normalizedSymbol == "" {
		return nil
	}

	endpoint := e.baseURL + "/bapi/asset/v1/public/asset-service/product/get-exchange-info?symbol=" + url.QueryEscape(normalizedSymbol)
	logger := logrus.WithFields(logrus.Fields{
		"exchange":    entity.ExchangeTokoCrypto,
		"market_type": entity.MarketTypeSpot,
		"symbol":      normalizedSymbol,
	})
	logger.Debug("refreshing symbol precision")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var symbolsResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Success *bool  `json:"success"`
		Data    []struct {
			Symbol         string `json:"symbol"`
			BasePrecision  int32  `json:"basePrecision"`
			QuotePrecision int32  `json:"quotePrecision"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &symbolsResp); err != nil {
		return fmt.Errorf("tokocrypto symbols parse failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	if resp.StatusCode >= http.StatusBadRequest || symbolsResp.Code != 0 || (symbolsResp.Success != nil && !*symbolsResp.Success) {
		return fmt.Errorf("tokocrypto symbols request failed: status=%d code=%d message=%s", resp.StatusCode, symbolsResp.Code, symbolsResp.Message)
	}

	for _, item := range symbolsResp.Data {
		itemSymbol := strings.ToUpper(strings.TrimSpace(item.Symbol))
		if itemSymbol == "" {
			continue
		}

		e.symbolPrecisionMu.Lock()
		if e.symbolPrecision == nil {
			e.symbolPrecision = make(map[string]tokocryptoSymbolPrecision)
		}
		e.symbolPrecision[itemSymbol] = tokocryptoSymbolPrecision{
			BasePrecision:  item.BasePrecision,
			QuotePrecision: item.QuotePrecision,
		}
		e.symbolPrecisionMu.Unlock()
	}

	logger.WithFields(logrus.Fields{
		"http_status":  resp.StatusCode,
		"symbol_count": len(symbolsResp.Data),
	}).Debug("symbol precision refreshed")

	return nil
}
