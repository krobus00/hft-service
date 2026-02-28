package exchange

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/constant"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/nats-io/nats.go"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

const (
	tokocryptoWSReconnectMinDelay = 1 * time.Second
	tokocryptoWSReconnectMaxDelay = 15 * time.Second
	tokocryptoWSReconnectFactor   = 2.0
)

var tokocryptoClientIDPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type TokocryptoExchange struct {
	apiKey     string
	apiSecret  string
	baseURL    string
	recvWindow int64
	httpClient *http.Client

	symbolPrecisionMu sync.RWMutex
	symbolPrecision   map[string]tokocryptoSymbolPrecision
	symbolMapping     entity.ExchangeSymbolMapping
	js                nats.JetStreamContext
	marketKlineRepo   *repository.MarketKlineRepository
}

type tokocryptoSymbolPrecision struct {
	BasePrecision  int32
	QuotePrecision int32
}

func InitTokocryptoExchange(ctx context.Context, exchangeConfig config.ExchangeConfig, symbolMappingRepo *repository.SymbolMappingRepository, js nats.JetStreamContext, marketKlineRepo *repository.MarketKlineRepository) *TokocryptoExchange {
	symbolMapping, err := symbolMappingRepo.GetByExchange(ctx, string(entity.ExchangeTokoCrypto))
	util.ContinueOrFatal(err)

	recvWindow := int64(5000)
	if raw := strings.TrimSpace(os.Getenv("TOKOCRYPTO_RECV_WINDOW")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 && parsed <= 60000 {
			recvWindow = parsed
		}
	}

	baseURL := strings.TrimSpace(os.Getenv("TOKOCRYPTO_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://www.tokocrypto.com"
	}

	newExchange := &TokocryptoExchange{
		apiKey:          strings.TrimSpace(exchangeConfig.APIKey),
		apiSecret:       strings.TrimSpace(exchangeConfig.APISecret),
		baseURL:         strings.TrimRight(baseURL, "/"),
		recvWindow:      recvWindow,
		httpClient:      &http.Client{Timeout: 15 * time.Second},
		symbolMapping:   symbolMapping,
		js:              js,
		marketKlineRepo: marketKlineRepo,
	}

	RegisterExchange(entity.ExchangeTokoCrypto, newExchange)

	return newExchange
}

func (e *TokocryptoExchange) JetstreamEventInit(ctx context.Context) error {
	streamConfig := &nats.StreamConfig{
		Name:      constant.KlineStreamName,
		Subjects:  []string{constant.KlineStreamSubjectAll},
		Storage:   nats.FileStorage, // use MemoryStorage for ultra-low latency
		Retention: nats.LimitsPolicy,
		MaxAge:    5 * time.Minute,
		Replicas:  1,
	}

	stream, err := e.js.StreamInfo(constant.KlineStreamName, nats.Context(ctx))
	if err != nil && !errors.Is(err, nats.ErrStreamNotFound) {
		logrus.Error(err)
		return err
	}

	if stream == nil {
		logrus.Infof("creating stream: %s", constant.KlineStreamName)
		_, err = e.js.AddStream(streamConfig, nats.Context(ctx))
		return err
	}

	logrus.Infof("updating stream: %s", constant.KlineStreamName)
	_, err = e.js.UpdateStream(streamConfig, nats.Context(ctx))
	if err != nil {
		logrus.Error(err)
		return err
	}

	logrus.Infof("stream %s is ready", constant.KlineStreamName)

	return nil
}

func (e *TokocryptoExchange) JetstreamEventSubscribe(ctx context.Context) error {
	err := e.JetstreamEventInit(ctx)
	if err != nil {
		logrus.Error(err)
		return err
	}

	_, err = e.js.QueueSubscribe(
		constant.GetKlineExchangeStreamSubject(string(entity.ExchangeTokoCrypto)),
		constant.GetKlineInsertQueueGroup(string(entity.ExchangeTokoCrypto)),
		func(msg *nats.Msg) {
			err := util.ProcessWithTimeout(config.Env.NatsJetstream.TimeoutHandler["insert_kline"], msg, e.handleKlineDataEvent)
			if err != nil {
				logrus.Errorf("error processing message: %v", err)
				return
			}

			err = msg.Ack()
			if err != nil {
				logrus.Errorf("failed to acknowledge message: %v", err)
				return
			}
		},
		nats.ManualAck(),
		nats.DeliverNew(), // only process new messages, ignore old messages when subscribe for the first time
	)
	util.ContinueOrFatal(err)

	return nil
}

func (e *TokocryptoExchange) handleKlineDataEvent(ctx context.Context, msg *nats.Msg) (err error) {
	logger := logrus.WithFields(logrus.Fields{
		"req": string(msg.Data),
	})

	var req *entity.MarketKlineEvent
	err = json.Unmarshal(msg.Data, &req)
	if err != nil {
		logger.Error(err)
		return err
	}

	if req.Data.EventTime.UTC().Add(1 * time.Minute).Before(time.Now().UTC()) {
		logger.Info("skipping kline data event that is too old")
		return nil
	}

	defer func() {
		if err != nil {
			req.RetryCount++
			if req.RetryCount >= config.Env.NatsJetstream.MaxRetries {
				return
			}

			err := util.PublishEvent(e.js, constant.GetKlineStreamSubject(string(entity.ExchangeTokoCrypto), req.Data.Symbol, req.Data.Interval), req)
			if err != nil {
				logger.Error(err)
				return
			}
		}
	}()

	err = e.marketKlineRepo.Create(ctx, &req.Data)
	if err != nil {
		logger.Error(err)
		return err
	}

	return nil
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
	symbol = e.resolveInternalSymbol(symbol)

	data := entity.MarketKline{
		Exchange:         string(entity.ExchangeTokoCrypto),
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
		RetryCount: 0,
		Data:       data,
	})
	if err != nil {
		return err
	}

	return nil
}

func (e *TokocryptoExchange) resolveInternalSymbol(exchangeSymbol string) string {
	normalized := strings.ToUpper(strings.TrimSpace(exchangeSymbol))
	if normalized == "" {
		return ""
	}

	for internalSymbol, klineSymbol := range e.symbolMapping[string(entity.ExchangeTokoCrypto)] {
		if strings.EqualFold(strings.TrimSpace(klineSymbol), normalized) {
			return internalSymbol
		}
	}

	return normalized
}

func (e *TokocryptoExchange) SubscribeKlineData(ctx context.Context, subscriptions []entity.KlineSubscription) error {
	wsURL := strings.TrimSpace(os.Getenv("TOKOCRYPTO_WS_URL"))
	if wsURL == "" {
		wsURL = "wss://stream-cloud.tokocrypto.site/stream"
	}

	wsHost, err := url.Parse(wsURL)
	if err != nil {
		return fmt.Errorf("invalid tokocrypto ws url: %w", err)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	attempt := 0

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		logrus.Infof("connecting to %s", wsHost.String())
		conn, _, err := websocket.DefaultDialer.Dial(wsHost.String(), nil)
		if err != nil {
			wait := tokocryptoReconnectDelay(attempt, rng)
			attempt++
			logrus.WithFields(logrus.Fields{"retry_in": wait.String(), "attempt": attempt}).Warnf("tokocrypto ws dial failed: %v", err)
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return nil
			}
		}

		attempt = 0
		conn.SetPongHandler(func(string) error {
			return nil
		})

		for _, v := range subscriptions {
			// only subs tokocrypto kline data, ignore other subs if any
			if v.Exchange != string(entity.ExchangeTokoCrypto) {
				continue
			}

			logrus.Infof("start subscription for symbol: %s, interval: %s", v.Symbol, v.Interval)

			if err := conn.WriteMessage(websocket.TextMessage, []byte(v.Payload)); err != nil {
				conn.Close()
				wait := tokocryptoReconnectDelay(attempt, rng)
				attempt++
				logrus.WithFields(logrus.Fields{"retry_in": wait.String(), "attempt": attempt}).Warnf("tokocrypto ws subscribe failed: %v", err)
				select {
				case <-time.After(wait):
					continue
				case <-ctx.Done():
					return nil
				}
			}
		}

		stopPing := make(chan struct{})
		go func(c *websocket.Conn) {
			ticker := time.NewTicker(2 * time.Minute)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
						logrus.Error(err)
						return
					}
				case <-ctx.Done():
					return
				case <-stopPing:
					return
				}
			}
		}(conn)

		ctxDone := make(chan struct{})
		go func(c *websocket.Conn) {
			select {
			case <-ctx.Done():
				_ = c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				_ = c.Close()
			case <-ctxDone:
			}
		}(conn)

		readErr := false
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if ctx.Err() != nil {
					close(stopPing)
					close(ctxDone)
					return nil
				}

				readErr = true
				logrus.Errorf("tokocrypto ws read failed: %v", err)
				break
			}

			err = e.HandleKlineData(ctx, message)
			if err != nil {
				logrus.Errorf("tokocrypto ws handle kline data failed: %v", err)
				continue
			}
		}

		close(stopPing)
		close(ctxDone)
		_ = conn.Close()

		if !readErr {
			continue
		}

		wait := tokocryptoReconnectDelay(attempt, rng)
		attempt++
		logrus.WithFields(logrus.Fields{"retry_in": wait.String(), "attempt": attempt}).Warn("reconnecting tokocrypto ws")
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil
		}
	}
}

func (e *TokocryptoExchange) PlaceOrder(ctx context.Context, order entity.OrderRequest) (*entity.OrderHistory, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if e.apiKey == "" || e.apiSecret == "" {
		return nil, fmt.Errorf("tokocrypto credentials are missing in config")
	}

	orderSymbol, ok := e.symbolMapping[string(entity.ExchangeTokoCrypto)][order.Symbol]
	if !ok {
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

	pairs = append(pairs,
		"timestamp="+timestamp,
		"recvWindow="+strconv.FormatInt(e.recvWindow, 10),
	)

	payload := strings.Join(pairs, "&")
	signature := hmacSHA256Hex(e.apiSecret, payload)
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

	req.Header.Set("X-MBX-APIKEY", e.apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

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

		logrus.Infof("tokocrypto order rejected: status=%d code=%d message=%s", resp.StatusCode, apiResp.Code, errMsg)

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

	logrus.WithFields(logrus.Fields{
		"exchange":         order.Exchange,
		"symbol":           orderSymbol,
		"type":             order.Type,
		"side":             order.Side,
		"price":            normalizedPrice.String(),
		"quantity":         normalizedQuantity.String(),
		"source":           order.Source,
		"response":         string(apiResp.Data),
		"history_order_id": orderHistory.OrderID,
		"history_status":   orderHistory.Status,
	}).Info("order placed")

	return &orderHistory, nil
}

func (e *TokocryptoExchange) SyncOrderHistory(ctx context.Context, orderHistory entity.OrderHistory) (*entity.OrderHistory, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if e.apiKey == "" || e.apiSecret == "" {
		return nil, fmt.Errorf("tokocrypto credentials are missing in config")
	}

	orderSymbol := orderHistory.Symbol
	if mapped, ok := e.symbolMapping[string(entity.ExchangeTokoCrypto)][orderSymbol]; ok {
		orderSymbol = mapped
	}
	orderSymbol = strings.TrimSpace(orderSymbol)
	if orderSymbol == "" {
		return nil, fmt.Errorf("tokocrypto order symbol is empty")
	}

	if strings.TrimSpace(orderHistory.OrderID) == "" && !orderHistory.ClientOrderID.Valid {
		return nil, fmt.Errorf("tokocrypto order history missing order id")
	}

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
	signature := hmacSHA256Hex(e.apiSecret, payload)
	endpoint := e.baseURL + "/open/v1/orders/detail?" + payload + "&signature=" + signature

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-MBX-APIKEY", e.apiKey)

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

	historyStatus := tokocryptoOrderStatusFromCode(resp.Status)
	now := time.Now().UTC()

	clientOrderID := sql.NullString{String: strings.TrimSpace(resp.ClientID), Valid: strings.TrimSpace(resp.ClientID) != ""}
	strategyID := sql.NullString{}
	if order.StrategyID != nil {
		trimmed := strings.TrimSpace(*order.StrategyID)
		if trimmed != "" {
			strategyID = sql.NullString{String: trimmed, Valid: true}
		}
	}

	createdAtExchange := sql.NullTime{}
	if resp.CreateTime > 0 {
		createdAtExchange = sql.NullTime{Time: time.UnixMilli(resp.CreateTime).UTC(), Valid: true}
	}

	sentAt := sql.NullTime{
		Time:  time.Now(),
		Valid: true,
	}
	if order.RequestedAt > 0 {
		sentAt = sql.NullTime{Time: time.UnixMilli(order.RequestedAt).UTC(), Valid: true}
	}

	acknowledgedAt := sql.NullTime{Time: now, Valid: true}

	var avgFillPrice *decimal.Decimal
	if executedPrice.GreaterThan(decimal.Zero) {
		avgFillPrice = &executedPrice
	}

	resolvedSymbol := e.resolveInternalSymbol(resp.Symbol)
	if resolvedSymbol == "" {
		resolvedSymbol = order.Symbol
	}

	return entity.OrderHistory{
		RequestID:         order.RequestID,
		UserID:            order.UserID,
		Exchange:          order.Exchange,
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
		ErrorMessage:      sql.NullString{},
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

func (e *TokocryptoExchange) mapOrderHistorySyncResponse(orderHistory entity.OrderHistory, resp entity.TokocryptoOrderDetailResponse) (entity.OrderHistory, error) {
	filledQuantity, err := tokocryptoDecimalOrZero(resp.ExecutedQty)
	if err != nil {
		return entity.OrderHistory{}, fmt.Errorf("invalid tokocrypto filled quantity: %w", err)
	}

	executedPrice, err := tokocryptoDecimalOrZero(resp.ExecutedPrice)
	if err != nil {
		return entity.OrderHistory{}, fmt.Errorf("invalid tokocrypto executed price: %w", err)
	}

	status := tokocryptoOrderStatusFromCode(resp.Status)
	now := time.Now().UTC()

	orderHistory.Status = status
	orderHistory.FilledQuantity = filledQuantity
	if executedPrice.GreaterThan(decimal.Zero) {
		orderHistory.AvgFillPrice = &executedPrice
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

	resolvedSymbol := e.resolveInternalSymbol(resp.Symbol)
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

func tokocryptoDecimalOrZero(raw string) (decimal.Decimal, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return decimal.Zero, nil
	}
	return decimal.NewFromString(trimmed)
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

func hmacSHA256Hex(secret, payload string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload))
	return fmt.Sprintf("%x", h.Sum(nil))
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

	return nil
}

func tokocryptoReconnectDelay(attempt int, rng *rand.Rand) time.Duration {
	backoff := float64(tokocryptoWSReconnectMinDelay) * math.Pow(tokocryptoWSReconnectFactor, float64(attempt))
	if backoff > float64(tokocryptoWSReconnectMaxDelay) {
		backoff = float64(tokocryptoWSReconnectMaxDelay)
	}

	base := time.Duration(backoff)
	if tokocryptoWSReconnectMaxDelay <= tokocryptoWSReconnectMinDelay {
		return base
	}

	jitterWindow := tokocryptoWSReconnectMaxDelay - tokocryptoWSReconnectMinDelay
	jitter := time.Duration(rng.Int63n(int64(jitterWindow) + 1))
	result := base + jitter
	if result > tokocryptoWSReconnectMaxDelay {
		return tokocryptoWSReconnectMaxDelay
	}

	return result
}
