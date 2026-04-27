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
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

var binanceClientIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type BinanceExchange struct {
	apiKey            string
	apiSecret         string
	spotBaseURL       string
	futuresBaseURL    string
	defaultMarketType entity.MarketType
	recvWindow        int64
	httpClient        *http.Client

	symbolPrecisionMu sync.RWMutex
	symbolPrecision   map[string]binanceSymbolPrecision
	symbolMapping     atomic.Value
	js                nats.JetStreamContext
	marketKlineRepo   *repository.MarketKlineRepository
	symbolMappingRepo *repository.SymbolMappingRepository
	klineSubRepo      *repository.KlineSubscriptionRepository
}

type binanceSymbolPrecision struct {
	BasePrecision    int32
	QuotePrecision   int32
	PriceTickSize    decimal.Decimal
	QuantityStepSize decimal.Decimal
}

func InitBinanceExchange(ctx context.Context, exchangeConfig config.ExchangeConfig, symbolMappingRepo *repository.SymbolMappingRepository, klineSubRepo *repository.KlineSubscriptionRepository, js nats.JetStreamContext, marketKlineRepo *repository.MarketKlineRepository) *BinanceExchange {
	symbolMapping, err := symbolMappingRepo.GetByExchange(ctx, string(entity.ExchangeBinance))
	util.ContinueOrFatal(err)

	defaultMarketType := entity.NormalizeMarketType(os.Getenv("BINANCE_TRADE_MODE"))

	recvWindow := int64(5000)
	if raw := strings.TrimSpace(os.Getenv("BINANCE_RECV_WINDOW")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 && parsed <= 60000 {
			recvWindow = parsed
		}
	}

	spotBaseURL := strings.TrimSpace(os.Getenv("BINANCE_BASE_URL"))
	if spotBaseURL == "" {
		spotBaseURL = strings.TrimSpace(os.Getenv("BINANCE_SPOT_BASE_URL"))
	}
	if spotBaseURL == "" {
		spotBaseURL = "https://api.binance.com"
	}

	futuresBaseURL := strings.TrimSpace(os.Getenv("BINANCE_FUTURES_BASE_URL"))
	if futuresBaseURL == "" {
		futuresBaseURL = "https://fapi.binance.com"
	}

	newExchange := &BinanceExchange{
		apiKey:            strings.TrimSpace(exchangeConfig.APIKey),
		apiSecret:         strings.TrimSpace(exchangeConfig.APISecret),
		spotBaseURL:       strings.TrimRight(spotBaseURL, "/"),
		futuresBaseURL:    strings.TrimRight(futuresBaseURL, "/"),
		defaultMarketType: defaultMarketType,
		recvWindow:        recvWindow,
		httpClient:        &http.Client{Timeout: 15 * time.Second},
		js:                js,
		marketKlineRepo:   marketKlineRepo,
		symbolMappingRepo: symbolMappingRepo,
		klineSubRepo:      klineSubRepo,
	}
	persistSymbolMapping(&newExchange.symbolMapping, symbolMapping)

	logrus.WithFields(logrus.Fields{
		"default_market_type": newExchange.defaultMarketType,
		"spot_base_url":       newExchange.spotBaseURL,
		"futures_base_url":    newExchange.futuresBaseURL,
	}).Info("binance exchange initialized")

	RegisterExchange(entity.ExchangeBinance, newExchange)

	return newExchange
}

func (e *BinanceExchange) JetstreamEventInit(ctx context.Context) error {
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

func (e *BinanceExchange) JetstreamEventSubscribe(ctx context.Context) error {
	err := e.JetstreamEventInit(ctx)
	if err != nil {
		logrus.Error(err)
		return err
	}

	consumerName := constant.GetKlineInsertQueueGroup(string(entity.ExchangeBinance))
	err = util.EnsureConsumer(ctx, e.js, constant.KlineStreamName, &nats.ConsumerConfig{
		Durable:       consumerName,
		DeliverGroup:  consumerName,
		FilterSubject: constant.GetKlineExchangeStreamSubject(string(entity.ExchangeBinance)),
		AckPolicy:     nats.AckExplicitPolicy,
		DeliverPolicy: nats.DeliverNewPolicy,
		MaxDeliver:    int(config.Env.NatsJetstream.MaxRetries),
	})
	if err != nil {
		logrus.Error(err)
		return err
	}

	_, err = e.js.QueueSubscribe(
		constant.GetKlineExchangeStreamSubject(string(entity.ExchangeBinance)),
		consumerName,
		func(msg *nats.Msg) {
			err := util.ProcessWithTimeout(config.Env.NatsJetstream.TimeoutHandler["insert_kline"], config.Env.NatsJetstream.MaxRetries, msg, e.handleKlineDataEvent)
			if err != nil {
				logrus.Errorf("error processing message: %v", err)
				return
			}
		},
		nats.ManualAck(),
		nats.Durable(consumerName),
		nats.DeliverNew(), // only process new messages, ignore old messages when subscribe for the first time
	)
	util.ContinueOrFatal(err)

	return nil
}

func (e *BinanceExchange) handleKlineDataEvent(ctx context.Context, msg *nats.Msg) (err error) {
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

	err = e.marketKlineRepo.Create(ctx, &req.Data)
	if err != nil {
		logger.Error(err)
		return err
	}

	return nil
}

func (e *BinanceExchange) HandleKlineData(ctx context.Context, message []byte) error {
	return e.handleKlineDataByMarketType(ctx, message, entity.MarketTypeSpot)
}

func (e *BinanceExchange) handleKlineDataByMarketType(ctx context.Context, message []byte, marketType entity.MarketType) error {
	market := entity.NormalizeMarketType(string(marketType))

	var wsResponse struct {
		Result any    `json:"result"`
		ID     any    `json:"id"`
		Code   *int   `json:"code"`
		Msg    string `json:"msg"`
	}
	if err := json.Unmarshal(message, &wsResponse); err == nil {
		if wsResponse.Code != nil {
			logrus.WithFields(logrus.Fields{
				"exchange":    entity.ExchangeBinance,
				"market_type": market,
				"code":        *wsResponse.Code,
				"message":     wsResponse.Msg,
				"id":          wsResponse.ID,
			}).Warn("binance websocket returned error response")
		}
		if wsResponse.Result == nil && wsResponse.ID != nil {
			logrus.WithFields(logrus.Fields{
				"exchange":    entity.ExchangeBinance,
				"market_type": market,
				"id":          wsResponse.ID,
			}).Info("binance websocket subscription acknowledged")
		}
	}

	type klinePayload struct {
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
	}

	var wrapped struct {
		Stream string       `json:"stream"`
		Data   klinePayload `json:"data"`
	}
	if err := json.Unmarshal(message, &wrapped); err != nil {
		return err
	}

	payload := wrapped.Data
	// Binance supports both combined stream payloads (`{"stream":...,"data":...}`)
	// and raw stream payloads (`{"e":"kline",...}`), so fall back to raw parsing.
	if strings.TrimSpace(payload.Event) == "" {
		var raw klinePayload
		if err := json.Unmarshal(message, &raw); err != nil {
			return err
		}
		payload = raw
	}

	if payload.Event != "kline" || payload.Kline.Close == "" {
		return nil
	}

	openPrice, err := decimal.NewFromString(payload.Kline.Open)
	if err != nil {
		return fmt.Errorf("invalid open price: %w", err)
	}

	closePrice, err := decimal.NewFromString(payload.Kline.Close)
	if err != nil {
		return fmt.Errorf("invalid close price: %w", err)
	}

	highPrice, err := decimal.NewFromString(payload.Kline.High)
	if err != nil {
		return fmt.Errorf("invalid high price: %w", err)
	}

	lowPrice, err := decimal.NewFromString(payload.Kline.Low)
	if err != nil {
		return fmt.Errorf("invalid low price: %w", err)
	}

	baseVolume, err := decimal.NewFromString(payload.Kline.BaseVolume)
	if err != nil {
		return fmt.Errorf("invalid base volume: %w", err)
	}

	quoteVolume, err := decimal.NewFromString(payload.Kline.QuoteVolume)
	if err != nil {
		return fmt.Errorf("invalid quote volume: %w", err)
	}

	takerBaseVolume, err := decimal.NewFromString(payload.Kline.TakerBaseVolume)
	if err != nil {
		return fmt.Errorf("invalid taker base volume: %w", err)
	}

	takerQuoteVolume, err := decimal.NewFromString(payload.Kline.TakerQuoteVolume)
	if err != nil {
		return fmt.Errorf("invalid taker quote volume: %w", err)
	}

	eventAt := time.UnixMilli(payload.EventTime).UTC()
	openAt := time.UnixMilli(payload.Kline.OpenTime).UTC()
	closeAt := time.UnixMilli(payload.Kline.CloseTime).UTC()
	now := time.Now().UTC()

	symbol := strings.TrimSpace(payload.Kline.Symbol)
	if symbol == "" {
		symbol = strings.TrimSpace(payload.Symbol)
	}
	symbol = resolveInternalSymbolFromMapping(exchangeKlineResyncDeps{
		ExchangeName:      entity.ExchangeBinance,
		MarketType:        marketType,
		SymbolMapping:     &e.symbolMapping,
		SymbolMappingRepo: e.symbolMappingRepo,
		KlineSubRepo:      e.klineSubRepo,
	}, symbol)

	data := entity.MarketKline{
		Exchange:         string(entity.ExchangeBinance),
		MarketType:       string(market),
		EventType:        payload.Event,
		EventTime:        eventAt,
		Symbol:           symbol,
		Interval:         payload.Kline.Interval,
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
		TradeCount:       payload.Kline.TradeCount,
		IsClosed:         payload.Kline.IsClosed,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	err = util.PublishEvent(e.js, constant.GetKlineStreamSubject(string(entity.ExchangeBinance), data.Symbol, data.Interval), entity.MarketKlineEvent{
		Data: data,
	})
	if err != nil {
		return err
	}

	return nil
}

func (e *BinanceExchange) SubscribeKlineData(ctx context.Context, subscriptions []entity.KlineSubscription) error {
	typesToRun := []entity.MarketType{entity.MarketTypeSpot, entity.MarketTypeFutures}
	errCh := make(chan error, len(typesToRun))
	started := 0

	for _, marketType := range typesToRun {
		marketType := marketType
		deps := exchangeKlineResyncDeps{
			ExchangeName:      entity.ExchangeBinance,
			MarketType:        marketType,
			SymbolMapping:     &e.symbolMapping,
			SymbolMappingRepo: e.symbolMappingRepo,
			KlineSubRepo:      e.klineSubRepo,
		}

		normalized := normalizeExchangeSubscriptions(deps, subscriptions)
		if len(normalized) == 0 {
			continue
		}

		started++
		go func(subs []entity.KlineSubscription) {
			errCh <- subscribeKlineDataWithAutoResync(ctx, klineWSSubscriberConfig{
				ExchangeName: entity.ExchangeBinance,
				WSURLEnvKey:  e.wsURLEnvKeyByMarketType(marketType),
				DefaultWSURL: e.defaultWSURLByMarketType(marketType),
				ResolveWSURL: func(subscriptions []entity.KlineSubscription) string {
					return e.resolveBinanceKlineWSURL(marketType, deps, subscriptions)
				},
				NormalizeSubs: func(subscriptions []entity.KlineSubscription) []entity.KlineSubscription {
					normalized := normalizeExchangeSubscriptions(deps, subscriptions)
					for i := range normalized {
						normalized[i].Payload = ""
					}
					return normalized
				},
				Resync: func(ctx context.Context, conn *websocket.Conn, fallback []entity.KlineSubscription) ([]entity.KlineSubscription, klineResyncState, error) {
					return resyncSymbolMappingAndSubscriptions(
						ctx,
						conn,
						fallback,
						func(ctx context.Context) error {
							return refreshExchangeSymbolMapping(ctx, deps)
						},
						func(ctx context.Context, fallback []entity.KlineSubscription) ([]entity.KlineSubscription, error) {
							return loadExchangeLatestSubscriptions(ctx, deps, fallback)
						},
						func(ctx context.Context) (klineResyncState, error) {
							return loadExchangeResyncState(ctx, deps)
						},
					)
				},
				LoadResyncState: func(ctx context.Context) (klineResyncState, error) {
					return loadExchangeResyncState(ctx, deps)
				},
				HandleMessage: func(ctx context.Context, message []byte) error {
					return e.handleKlineDataByMarketType(ctx, message, marketType)
				},
			}, subs)
		}(normalized)
	}

	if started == 0 {
		return nil
	}

	for i := 0; i < started; i++ {
		err := <-errCh
		if err != nil {
			return err
		}
	}

	return nil
}

func (e *BinanceExchange) BackfillMarketKlines(ctx context.Context, req entity.MarketKlineBackfillRequest) (int, error) {
	marketType := e.resolveMarketType(string(req.MarketType))

	return backfillMarketKlines(ctx, marketKlineBackfillDeps{
		ExchangeName:  entity.ExchangeBinance,
		MarketType:    marketType,
		BaseURL:       e.baseURLByMarketType(marketType),
		KlinePath:     e.klinePathByMarketType(marketType),
		HTTPClient:    e.httpClient,
		SymbolMapping: &e.symbolMapping,
	}, e.marketKlineRepo, req)
}

func (e *BinanceExchange) PlaceOrder(ctx context.Context, order entity.OrderRequest) (*entity.OrderHistory, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if e.apiKey == "" || e.apiSecret == "" {
		return nil, fmt.Errorf("binance credentials are missing in config")
	}

	marketType := e.resolveMarketType(order.MarketType)
	order.MarketType = string(marketType)
	order.Side = entity.NormalizeOrderSideByMarket(string(order.Side), marketType)
	if order.Side == "" {
		return nil, fmt.Errorf("unsupported order side for market type %s", marketType)
	}

	deps := exchangeKlineResyncDeps{
		ExchangeName:      entity.ExchangeBinance,
		MarketType:        marketType,
		SymbolMapping:     &e.symbolMapping,
		SymbolMappingRepo: e.symbolMappingRepo,
		KlineSubRepo:      e.klineSubRepo,
	}

	orderSymbol := resolveExchangeOrderSymbolFromMapping(deps, order.Symbol)
	if strings.TrimSpace(orderSymbol) == "" {
		orderSymbol = order.Symbol
	}

	typeCode, err := binanceOrderTypeCode(order.Type)
	if err != nil {
		return nil, err
	}

	sideCode, err := binanceOrderSideCode(order.Side, marketType)
	if err != nil {
		return nil, err
	}

	normalizedQuantity := order.Quantity
	normalizedPrice := order.Price

	if precision, ok, err := e.getSymbolPrecision(ctx, marketType, orderSymbol); err != nil {
		logrus.WithError(err).WithField("symbol", orderSymbol).Warn("failed to fetch binance symbol precision")
	} else if ok {
		if precision.QuantityStepSize.GreaterThan(decimal.Zero) {
			normalizedQuantity = quantizeDownByStep(order.Quantity, precision.QuantityStepSize)
		} else {
			normalizedQuantity = order.Quantity.Truncate(precision.BasePrecision)
		}
		if !normalizedQuantity.GreaterThan(decimal.Zero) {
			return nil, fmt.Errorf("binance order quantity becomes zero after normalization: quantity=%s basePrecision=%d", order.Quantity.String(), precision.BasePrecision)
		}

		if order.Type == entity.OrderTypeLimit {
			if precision.PriceTickSize.GreaterThan(decimal.Zero) {
				normalizedPrice = quantizeDownByStep(order.Price, precision.PriceTickSize)
			} else {
				normalizedPrice = order.Price.Truncate(precision.QuotePrecision)
			}
			if !normalizedPrice.GreaterThan(decimal.Zero) {
				return nil, fmt.Errorf("binance order price becomes zero after normalization: price=%s quotePrecision=%d", order.Price.String(), precision.QuotePrecision)
			}
		}
	}

	basePairs := []string{
		"symbol=" + orderSymbol,
		"side=" + sideCode,
		"type=" + typeCode,
		"quantity=" + normalizedQuantity.String(),
	}

	if order.OrderID != nil && strings.TrimSpace(*order.OrderID) != "" {
		clientID, err := normalizeBinanceClientID(strings.TrimSpace(*order.OrderID))
		if err != nil {
			return nil, err
		}

		basePairs = append(basePairs, "newClientOrderId="+url.QueryEscape(clientID))
	}

	if order.Type == entity.OrderTypeLimit {
		basePairs = append(basePairs,
			"price="+normalizedPrice.String(),
			"timeInForce=GTC",
		)
	}

	futuresPositionSide := ""
	if marketType == entity.MarketTypeFutures {
		futuresPositionSide = e.futuresPositionSide(order)
		order.PositionSide = futuresPositionSide
	}

	submitOrder := func(positionSide string) (int, []byte, error) {
		pairs := make([]string, 0, len(basePairs)+3)
		pairs = append(pairs, basePairs...)
		if marketType == entity.MarketTypeFutures && strings.TrimSpace(positionSide) != "" {
			pairs = append(pairs, "positionSide="+positionSide)
		}

		timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
		pairs = append(pairs,
			"timestamp="+timestamp,
			"recvWindow="+strconv.FormatInt(e.recvWindow, 10),
		)

		payload := strings.Join(pairs, "&")
		signature := binanceHMACSHA256Hex(e.apiSecret, payload)
		bodyPayload := payload + "&signature=" + signature

		if strings.EqualFold(strings.TrimSpace(os.Getenv("BINANCE_DEBUG_SIGN")), "true") {
			logrus.WithFields(logrus.Fields{
				"payload":   payload,
				"signature": signature,
			}).Info("binance signed payload")
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURLByMarketType(marketType)+e.orderPathByMarketType(marketType), strings.NewReader(bodyPayload))
		if err != nil {
			return 0, nil, err
		}

		req.Header.Set("X-MBX-APIKEY", e.apiKey)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := e.httpClient.Do(req)
		if err != nil {
			return 0, nil, err
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return 0, nil, readErr
		}

		return resp.StatusCode, body, nil
	}

	statusCode, body, err := submitOrder(futuresPositionSide)
	if err != nil {
		return nil, err
	}

	if statusCode >= http.StatusBadRequest {
		var apiErr struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := json.Unmarshal(body, &apiErr); err != nil {
			return nil, fmt.Errorf("binance order rejected: status=%d body=%s", statusCode, string(body))
		}

		if marketType == entity.MarketTypeFutures && apiErr.Code == -4061 {
			if alternatePositionSide, ok := alternateFuturesPositionSide(futuresPositionSide, sideCode); ok && alternatePositionSide != futuresPositionSide {
				logrus.WithFields(logrus.Fields{
					"market_type":             marketType,
					"symbol":                  orderSymbol,
					"side":                    sideCode,
					"position_side":           futuresPositionSide,
					"alternate_position_side": alternatePositionSide,
					"code":                    apiErr.Code,
					"message":                 apiErr.Msg,
				}).Warn("binance order position mode mismatch, retrying with alternate position side")

				futuresPositionSide = alternatePositionSide
				order.PositionSide = futuresPositionSide
				statusCode, body, err = submitOrder(futuresPositionSide)
				if err != nil {
					return nil, err
				}

				if statusCode < http.StatusBadRequest {
					goto parseSuccess
				}

				if err := json.Unmarshal(body, &apiErr); err != nil {
					return nil, fmt.Errorf("binance order rejected: status=%d body=%s", statusCode, string(body))
				}
			}
		}

		logrus.WithFields(logrus.Fields{
			"status":        statusCode,
			"code":          apiErr.Code,
			"message":       apiErr.Msg,
			"market_type":   marketType,
			"symbol":        orderSymbol,
			"side":          sideCode,
			"type":          typeCode,
			"quantity":      normalizedQuantity.String(),
			"price":         normalizedPrice.String(),
			"position_side": futuresPositionSide,
		}).Info("binance order rejected")

		return nil, fmt.Errorf("binance order rejected: status=%d code=%d message=%s", statusCode, apiErr.Code, apiErr.Msg)
	}

parseSuccess:
	placeOrderResp, err := parseBinancePlaceOrderResponse(body, marketType == entity.MarketTypeFutures)
	if err != nil {
		return nil, fmt.Errorf("binance order parse failed: status=%d body=%s", statusCode, string(body))
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
		"position_side":    futuresPositionSide,
		"price":            normalizedPrice.String(),
		"quantity":         normalizedQuantity.String(),
		"source":           order.Source,
		"response":         string(body),
		"history_order_id": orderHistory.OrderID,
		"history_status":   orderHistory.Status,
	}).Info("order placed")

	return &orderHistory, nil
}

func (e *BinanceExchange) SyncOrderHistory(ctx context.Context, orderHistory entity.OrderHistory) (*entity.OrderHistory, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if e.apiKey == "" || e.apiSecret == "" {
		return nil, fmt.Errorf("binance credentials are missing in config")
	}

	marketType := e.resolveMarketType(orderHistory.MarketType)
	orderHistory.MarketType = string(marketType)

	deps := exchangeKlineResyncDeps{
		ExchangeName:      entity.ExchangeBinance,
		MarketType:        marketType,
		SymbolMapping:     &e.symbolMapping,
		SymbolMappingRepo: e.symbolMappingRepo,
		KlineSubRepo:      e.klineSubRepo,
	}

	orderSymbol := resolveExchangeOrderSymbolFromMapping(deps, orderHistory.Symbol)
	orderSymbol = strings.TrimSpace(orderSymbol)
	if orderSymbol == "" {
		return nil, fmt.Errorf("binance order symbol is empty")
	}

	if strings.TrimSpace(orderHistory.OrderID) == "" && !orderHistory.ClientOrderID.Valid {
		return nil, fmt.Errorf("binance order history missing order id")
	}

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	pairs := []string{
		"symbol=" + orderSymbol,
	}

	if strings.TrimSpace(orderHistory.OrderID) != "" {
		pairs = append(pairs, "orderId="+strings.TrimSpace(orderHistory.OrderID))
	} else {
		pairs = append(pairs, "origClientOrderId="+url.QueryEscape(strings.TrimSpace(orderHistory.ClientOrderID.String)))
	}

	pairs = append(pairs,
		"timestamp="+timestamp,
		"recvWindow="+strconv.FormatInt(e.recvWindow, 10),
	)

	payload := strings.Join(pairs, "&")
	signature := binanceHMACSHA256Hex(e.apiSecret, payload)
	endpoint := e.baseURLByMarketType(marketType) + e.orderPathByMarketType(marketType) + "?" + payload + "&signature=" + signature

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

	if resp.StatusCode >= http.StatusBadRequest {
		var apiErr struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := json.Unmarshal(body, &apiErr); err != nil {
			return nil, fmt.Errorf("binance order detail rejected: status=%d body=%s", resp.StatusCode, string(body))
		}

		return nil, fmt.Errorf("binance order detail rejected: status=%d code=%d message=%s", resp.StatusCode, apiErr.Code, apiErr.Msg)
	}

	detailResp, err := parseBinanceOrderDetailResponse(body, marketType == entity.MarketTypeFutures)
	if err != nil {
		return nil, fmt.Errorf("binance order detail parse failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	updatedHistory, err := e.mapOrderHistorySyncResponse(orderHistory, detailResp)
	if err != nil {
		return nil, err
	}

	return &updatedHistory, nil
}

func (e *BinanceExchange) mapPlaceOrderResponseToOrderHistory(order entity.OrderRequest, resp binanceOrderResponseSnapshot) (entity.OrderHistory, error) {
	marketType := e.resolveMarketType(order.MarketType)

	price, err := decimal.NewFromString(resp.Price)
	if err != nil {
		return entity.OrderHistory{}, fmt.Errorf("invalid binance order price: %w", err)
	}

	quantity, err := decimal.NewFromString(resp.OrigQty)
	if err != nil {
		return entity.OrderHistory{}, fmt.Errorf("invalid binance order quantity: %w", err)
	}

	filledQuantity, err := decimal.NewFromString(resp.ExecutedQty)
	if err != nil {
		return entity.OrderHistory{}, fmt.Errorf("invalid binance filled quantity: %w", err)
	}

	executedPrice := decimal.Zero
	if filledQuantity.GreaterThan(decimal.Zero) {
		quoteQty, err := binanceDecimalOrZero(resp.CumulativeQuoteQty)
		if err != nil {
			return entity.OrderHistory{}, fmt.Errorf("invalid binance cumulative quote quantity: %w", err)
		}

		executedPrice = quoteQty.Div(filledQuantity)
	}

	historySide, err := binanceOrderSideFromCode(resp.Side)
	if err != nil {
		return entity.OrderHistory{}, err
	}
	if marketType == entity.MarketTypeFutures {
		historySide = entity.NormalizeOrderSideByMarket(string(order.Side), marketType)
		if historySide == "" {
			switch strings.ToUpper(strings.TrimSpace(resp.Side)) {
			case "BUY":
				historySide = entity.OrderSideLong
			case "SELL":
				historySide = entity.OrderSideShort
			default:
				return entity.OrderHistory{}, fmt.Errorf("unsupported binance futures order side code: %s", resp.Side)
			}
		}
	}

	historyType, err := binanceOrderTypeFromCode(resp.Type)
	if err != nil {
		return entity.OrderHistory{}, err
	}

	historyStatus := binanceOrderStatusFromCode(resp.Status)
	now := time.Now().UTC()

	clientOrderID := sql.NullString{String: strings.TrimSpace(resp.ClientOrderID), Valid: strings.TrimSpace(resp.ClientOrderID) != ""}
	strategyID := sql.NullString{}
	if order.StrategyID != nil {
		trimmed := strings.TrimSpace(*order.StrategyID)
		if trimmed != "" {
			strategyID = sql.NullString{String: trimmed, Valid: true}
		}
	}

	createdAtExchange := sql.NullTime{}
	if resp.EventTime > 0 {
		createdAtExchange = sql.NullTime{Time: time.UnixMilli(resp.EventTime).UTC(), Valid: true}
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

	resolvedSymbol := resolveInternalSymbolFromOrderMapping(exchangeKlineResyncDeps{
		ExchangeName:      entity.ExchangeBinance,
		MarketType:        marketType,
		SymbolMapping:     &e.symbolMapping,
		SymbolMappingRepo: e.symbolMappingRepo,
		KlineSubRepo:      e.klineSubRepo,
	}, resp.Symbol)
	if resolvedSymbol == "" {
		resolvedSymbol = order.Symbol
	}

	return entity.OrderHistory{
		RequestID:         order.RequestID,
		UserID:            order.UserID,
		Exchange:          order.Exchange,
		MarketType:        string(marketType),
		PositionSide:      string(entity.NormalizePositionSide(order.PositionSide)),
		Symbol:            resolvedSymbol,
		OrderID:           strconv.FormatInt(resp.OrderID, 10),
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

func (e *BinanceExchange) mapOrderHistorySyncResponse(orderHistory entity.OrderHistory, resp binanceOrderResponseSnapshot) (entity.OrderHistory, error) {
	filledQuantity, err := binanceDecimalOrZero(resp.ExecutedQty)
	if err != nil {
		return entity.OrderHistory{}, fmt.Errorf("invalid binance filled quantity: %w", err)
	}

	executedPrice := decimal.Zero
	if filledQuantity.GreaterThan(decimal.Zero) {
		quoteQty, err := binanceDecimalOrZero(resp.CumulativeQuoteQty)
		if err != nil {
			return entity.OrderHistory{}, fmt.Errorf("invalid binance cumulative quote quantity: %w", err)
		}

		executedPrice = quoteQty.Div(filledQuantity)
	}

	status := binanceOrderStatusFromCode(resp.Status)
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

	if resp.OrderID > 0 && strings.TrimSpace(orderHistory.OrderID) == "" {
		orderHistory.OrderID = strconv.FormatInt(resp.OrderID, 10)
	}

	if clientID := strings.TrimSpace(resp.ClientOrderID); clientID != "" && !orderHistory.ClientOrderID.Valid {
		orderHistory.ClientOrderID = sql.NullString{String: clientID, Valid: true}
	}

	if resp.EventTime > 0 && !orderHistory.CreatedAtExchange.Valid {
		orderHistory.CreatedAtExchange = sql.NullTime{Time: time.UnixMilli(resp.EventTime).UTC(), Valid: true}
	}

	resolvedSymbol := resolveInternalSymbolFromOrderMapping(exchangeKlineResyncDeps{
		ExchangeName:      entity.ExchangeBinance,
		MarketType:        e.resolveMarketType(orderHistory.MarketType),
		SymbolMapping:     &e.symbolMapping,
		SymbolMappingRepo: e.symbolMappingRepo,
		KlineSubRepo:      e.klineSubRepo,
	}, resp.Symbol)
	if resolvedSymbol != "" {
		orderHistory.Symbol = resolvedSymbol
	}

	return orderHistory, nil
}

func binanceOrderSideFromCode(code string) (entity.OrderSide, error) {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "BUY":
		return entity.OrderSideBuy, nil
	case "SELL":
		return entity.OrderSideSell, nil
	default:
		return "", fmt.Errorf("unsupported binance order side code: %s", code)
	}
}

func binanceOrderTypeFromCode(code string) (entity.OrderType, error) {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "LIMIT":
		return entity.OrderTypeLimit, nil
	case "MARKET":
		return entity.OrderTypeMarket, nil
	default:
		return "", fmt.Errorf("unsupported binance order type code: %s", code)
	}
}

func binanceOrderStatusFromCode(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "NEW":
		return "NEW"
	case "PARTIALLY_FILLED":
		return "PARTIAL"
	case "FILLED":
		return "FILLED"
	case "CANCELED", "EXPIRED", "PENDING_CANCEL":
		return "CANCELED"
	case "REJECTED":
		return "REJECTED"
	default:
		if code == "" {
			return "UNKNOWN"
		}

		return code
	}
}

func binanceDecimalOrZero(raw string) (decimal.Decimal, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return decimal.Zero, nil
	}
	return decimal.NewFromString(trimmed)
}

func binanceOrderTypeCode(orderType entity.OrderType) (string, error) {
	switch orderType {
	case entity.OrderTypeLimit:
		return "LIMIT", nil
	case entity.OrderTypeMarket:
		return "MARKET", nil
	default:
		return "", fmt.Errorf("unsupported order type for binance: %s", orderType)
	}
}

func binanceOrderSideCode(orderSide entity.OrderSide, marketType entity.MarketType) (string, error) {
	normalized := entity.NormalizeOrderSideByMarket(string(orderSide), marketType)
	if normalized == "" {
		return "", fmt.Errorf("unsupported order side for binance: %s", orderSide)
	}

	if entity.NormalizeMarketType(string(marketType)) == entity.MarketTypeFutures {
		switch normalized {
		case entity.OrderSideLong:
			return "BUY", nil
		case entity.OrderSideShort:
			return "SELL", nil
		default:
			return "", fmt.Errorf("unsupported futures order side for binance: %s", orderSide)
		}
	}

	switch normalized {
	case entity.OrderSideBuy:
		return "BUY", nil
	case entity.OrderSideSell:
		return "SELL", nil
	default:
		return "", fmt.Errorf("unsupported spot order side for binance: %s", orderSide)
	}
}

func normalizeBinanceClientID(raw string) (string, error) {
	normalized := strings.TrimSpace(raw)
	normalized = strings.ReplaceAll(normalized, "-", "")

	if normalized == "" {
		return "", fmt.Errorf("binance clientId is empty")
	}

	if len(normalized) > 36 {
		normalized = normalized[:36]
	}

	if !binanceClientIDPattern.MatchString(normalized) {
		return "", fmt.Errorf("binance clientId contains unsupported characters")
	}

	return normalized, nil
}

func binanceHMACSHA256Hex(secret, payload string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func quantizeDownByStep(value decimal.Decimal, step decimal.Decimal) decimal.Decimal {
	if !step.GreaterThan(decimal.Zero) {
		return value
	}

	return value.Div(step).Floor().Mul(step)
}

func (e *BinanceExchange) getSymbolPrecision(ctx context.Context, marketType entity.MarketType, symbol string) (binanceSymbolPrecision, bool, error) {
	normalizedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	if normalizedSymbol == "" {
		return binanceSymbolPrecision{}, false, nil
	}
	cacheKey := e.symbolPrecisionCacheKey(marketType, normalizedSymbol)

	e.symbolPrecisionMu.RLock()
	if precision, exists := e.symbolPrecision[cacheKey]; exists {
		e.symbolPrecisionMu.RUnlock()
		return precision, true, nil
	}
	e.symbolPrecisionMu.RUnlock()

	if err := e.refreshSymbolPrecision(ctx, marketType, normalizedSymbol); err != nil {
		return binanceSymbolPrecision{}, false, err
	}

	e.symbolPrecisionMu.RLock()
	precision, exists := e.symbolPrecision[cacheKey]
	e.symbolPrecisionMu.RUnlock()

	return precision, exists, nil
}

func (e *BinanceExchange) refreshSymbolPrecision(ctx context.Context, marketType entity.MarketType, symbol string) error {
	normalizedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	if normalizedSymbol == "" {
		return nil
	}

	marketType = entity.NormalizeMarketType(string(marketType))
	endpoint := e.baseURLByMarketType(marketType) + e.exchangeInfoPathByMarketType(marketType) + "?symbol=" + url.QueryEscape(normalizedSymbol)
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
		Symbols []struct {
			Symbol            string `json:"symbol"`
			BasePrecision     int32  `json:"basePrecision"`
			QuotePrecision    int32  `json:"quotePrecision"`
			PricePrecision    int32  `json:"pricePrecision"`
			QuantityPrecision int32  `json:"quantityPrecision"`
			Filters           []struct {
				FilterType string `json:"filterType"`
				TickSize   string `json:"tickSize"`
				StepSize   string `json:"stepSize"`
			} `json:"filters"`
		} `json:"symbols"`
	}

	if err := json.Unmarshal(body, &symbolsResp); err != nil {
		return fmt.Errorf("binance symbols parse failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("binance symbols request failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	for _, item := range symbolsResp.Symbols {
		itemSymbol := strings.ToUpper(strings.TrimSpace(item.Symbol))
		if itemSymbol == "" {
			continue
		}

		basePrecision := item.BasePrecision
		quotePrecision := item.QuotePrecision
		if marketType == entity.MarketTypeFutures {
			basePrecision = item.QuantityPrecision
			quotePrecision = item.PricePrecision
		}

		priceTickSize := decimal.Zero
		quantityStepSize := decimal.Zero
		for _, filter := range item.Filters {
			switch strings.ToUpper(strings.TrimSpace(filter.FilterType)) {
			case "PRICE_FILTER":
				if tick, err := decimal.NewFromString(strings.TrimSpace(filter.TickSize)); err == nil && tick.GreaterThan(decimal.Zero) {
					priceTickSize = tick
				}
			case "LOT_SIZE", "MARKET_LOT_SIZE":
				if step, err := decimal.NewFromString(strings.TrimSpace(filter.StepSize)); err == nil && step.GreaterThan(decimal.Zero) {
					if quantityStepSize.Equal(decimal.Zero) || step.GreaterThan(quantityStepSize) {
						quantityStepSize = step
					}
				}
			}
		}

		cacheKey := e.symbolPrecisionCacheKey(marketType, itemSymbol)

		e.symbolPrecisionMu.Lock()
		if e.symbolPrecision == nil {
			e.symbolPrecision = make(map[string]binanceSymbolPrecision)
		}
		e.symbolPrecision[cacheKey] = binanceSymbolPrecision{
			BasePrecision:    basePrecision,
			QuotePrecision:   quotePrecision,
			PriceTickSize:    priceTickSize,
			QuantityStepSize: quantityStepSize,
		}
		e.symbolPrecisionMu.Unlock()
	}

	return nil
}

type binanceOrderResponseSnapshot struct {
	Symbol             string
	OrderID            int64
	ClientOrderID      string
	EventTime          int64
	Price              string
	OrigQty            string
	ExecutedQty        string
	CumulativeQuoteQty string
	Status             string
	Type               string
	Side               string
}

type binanceSpotPlaceOrderResponse struct {
	Symbol              string `json:"symbol"`
	OrderID             int64  `json:"orderId"`
	ClientOrderID       string `json:"clientOrderId"`
	TransactTime        int64  `json:"transactTime"`
	Price               string `json:"price"`
	OrigQty             string `json:"origQty"`
	ExecutedQty         string `json:"executedQty"`
	CummulativeQuoteQty string `json:"cummulativeQuoteQty"`
	Status              string `json:"status"`
	Type                string `json:"type"`
	Side                string `json:"side"`
}

type binanceSpotOrderDetailResponse struct {
	Symbol              string `json:"symbol"`
	OrderID             int64  `json:"orderId"`
	ClientOrderID       string `json:"clientOrderId"`
	Price               string `json:"price"`
	OrigQty             string `json:"origQty"`
	ExecutedQty         string `json:"executedQty"`
	CummulativeQuoteQty string `json:"cummulativeQuoteQty"`
	Status              string `json:"status"`
	Type                string `json:"type"`
	Side                string `json:"side"`
	Time                int64  `json:"time"`
}

type binanceFuturesOrderResponse struct {
	Symbol        string `json:"symbol"`
	OrderID       int64  `json:"orderId"`
	ClientOrderID string `json:"clientOrderId"`
	Price         string `json:"price"`
	OrigQty       string `json:"origQty"`
	ExecutedQty   string `json:"executedQty"`
	CumQuote      string `json:"cumQuote"`
	Status        string `json:"status"`
	Type          string `json:"type"`
	Side          string `json:"side"`
	UpdateTime    int64  `json:"updateTime"`
	Time          int64  `json:"time"`
}

func (e *BinanceExchange) resolveMarketType(raw string) entity.MarketType {
	if strings.TrimSpace(raw) == "" {
		return e.defaultMarketType
	}

	return entity.NormalizeMarketType(raw)
}

func (e *BinanceExchange) orderPathByMarketType(marketType entity.MarketType) string {
	if entity.NormalizeMarketType(string(marketType)) == entity.MarketTypeFutures {
		return "/fapi/v1/order"
	}

	return "/api/v3/order"
}

func (e *BinanceExchange) exchangeInfoPathByMarketType(marketType entity.MarketType) string {
	if entity.NormalizeMarketType(string(marketType)) == entity.MarketTypeFutures {
		return "/fapi/v1/exchangeInfo"
	}

	return "/api/v3/exchangeInfo"
}

func (e *BinanceExchange) klinePathByMarketType(marketType entity.MarketType) string {
	if entity.NormalizeMarketType(string(marketType)) == entity.MarketTypeFutures {
		return "/fapi/v1/klines"
	}

	return "/api/v3/klines"
}

func (e *BinanceExchange) baseURLByMarketType(marketType entity.MarketType) string {
	if entity.NormalizeMarketType(string(marketType)) == entity.MarketTypeFutures {
		return e.futuresBaseURL
	}

	return e.spotBaseURL
}

func (e *BinanceExchange) wsURLEnvKeyByMarketType(marketType entity.MarketType) string {
	if entity.NormalizeMarketType(string(marketType)) == entity.MarketTypeFutures {
		return "BINANCE_FUTURES_WS_URL"
	}

	return "BINANCE_WS_URL"
}

func (e *BinanceExchange) defaultWSURLByMarketType(marketType entity.MarketType) string {
	if entity.NormalizeMarketType(string(marketType)) == entity.MarketTypeFutures {
		return "wss://fstream.binance.com/market/stream"
	}

	return "wss://stream.binance.com:9443/stream"
}

func (e *BinanceExchange) resolveBinanceKlineWSURL(marketType entity.MarketType, deps exchangeKlineResyncDeps, subscriptions []entity.KlineSubscription) string {
	wsURL := strings.TrimSpace(os.Getenv(e.wsURLEnvKeyByMarketType(marketType)))
	if wsURL == "" {
		wsURL = e.defaultWSURLByMarketType(marketType)
	}

	parsed, err := url.Parse(strings.TrimSpace(wsURL))
	if err != nil {
		return wsURL
	}

	streams := make([]string, 0, len(subscriptions))
	for _, sub := range subscriptions {
		if sub.Exchange != string(entity.ExchangeBinance) {
			continue
		}
		if string(entity.NormalizeMarketType(sub.MarketType)) != string(entity.NormalizeMarketType(string(marketType))) {
			continue
		}

		exchangeSymbol := resolveExchangeKlineSymbolFromMapping(deps, sub.Symbol)
		exchangeSymbol = normalizeBinanceKlineStreamSymbol(exchangeSymbol, marketType)
		exchangeSymbol = strings.ToLower(strings.TrimSpace(exchangeSymbol))
		interval := strings.TrimSpace(sub.Interval)
		if exchangeSymbol == "" || interval == "" {
			continue
		}

		streams = append(streams, exchangeSymbol+"@kline_"+interval)
	}

	if len(streams) == 0 {
		return wsURL
	}

	if strings.HasSuffix(parsed.Path, "/ws") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/ws") + "/stream"
	}
	if !strings.HasSuffix(parsed.Path, "/stream") {
		parsed.Path = strings.TrimRight(parsed.Path, "/") + "/stream"
	}

	query := parsed.Query()
	query.Set("streams", strings.Join(streams, "/"))
	parsed.RawQuery = query.Encode()

	return parsed.String()
}

func normalizeBinanceKlineStreamSymbol(symbol string, marketType entity.MarketType) string {
	normalized := strings.ToUpper(strings.TrimSpace(symbol))
	if normalized == "" {
		return ""
	}

	if entity.NormalizeMarketType(string(marketType)) == entity.MarketTypeFutures && strings.HasSuffix(normalized, ".P") {
		normalized = strings.TrimSuffix(normalized, ".P")
	}

	normalized = strings.ReplaceAll(normalized, "_", "")
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, ".", "")

	return normalized
}

func (e *BinanceExchange) symbolPrecisionCacheKey(marketType entity.MarketType, symbol string) string {
	return string(entity.NormalizeMarketType(string(marketType))) + ":" + strings.ToUpper(strings.TrimSpace(symbol))
}

func (e *BinanceExchange) futuresPositionSide(order entity.OrderRequest) string {
	mode := normalizeBinanceFuturesPositionMode(os.Getenv("BINANCE_FUTURES_POSITION_MODE"))

	if mode == "ONE_WAY" {
		return "BOTH"
	}

	explicit := entity.NormalizePositionSide(order.PositionSide)
	if explicit == entity.PositionSideLong || explicit == entity.PositionSideShort {
		return string(explicit)
	}

	override := strings.ToUpper(strings.TrimSpace(os.Getenv("BINANCE_FUTURES_POSITION_SIDE")))
	switch override {
	case "BOTH", "LONG", "SHORT":
		if mode == "HEDGE" && override == "BOTH" {
			break
		}
		if mode == "ONE_WAY" && (override == "LONG" || override == "SHORT") {
			break
		}
		return override
	}

	if mode == "HEDGE" {
		if order.Side == entity.OrderSideShort {
			return "SHORT"
		}
		return "LONG"
	}

	return "BOTH"
}

func normalizeBinanceFuturesPositionMode(raw string) string {
	mode := strings.ToUpper(strings.TrimSpace(raw))
	switch mode {
	case "HEDGE", "HEDGE_MODE", "DUAL", "DUAL_SIDE", "DUALSIDE":
		return "HEDGE"
	case "ONE_WAY", "ONEWAY", "SINGLE":
		return "ONE_WAY"
	default:
		return "AUTO"
	}
}

func alternateFuturesPositionSide(current string, sideCode string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(current)) {
	case "BOTH":
		if strings.EqualFold(strings.TrimSpace(sideCode), "SELL") {
			return "SHORT", true
		}
		return "LONG", true
	case "LONG", "SHORT":
		return "BOTH", true
	default:
		if strings.EqualFold(strings.TrimSpace(sideCode), "SELL") {
			return "SHORT", true
		}
		return "LONG", true
	}
}

func parseBinancePlaceOrderResponse(body []byte, futures bool) (binanceOrderResponseSnapshot, error) {
	if futures {
		var resp binanceFuturesOrderResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return binanceOrderResponseSnapshot{}, err
		}

		eventTime := resp.UpdateTime
		if eventTime <= 0 {
			eventTime = resp.Time
		}

		return binanceOrderResponseSnapshot{
			Symbol:             resp.Symbol,
			OrderID:            resp.OrderID,
			ClientOrderID:      resp.ClientOrderID,
			EventTime:          eventTime,
			Price:              resp.Price,
			OrigQty:            resp.OrigQty,
			ExecutedQty:        resp.ExecutedQty,
			CumulativeQuoteQty: resp.CumQuote,
			Status:             resp.Status,
			Type:               resp.Type,
			Side:               resp.Side,
		}, nil
	}

	var resp binanceSpotPlaceOrderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return binanceOrderResponseSnapshot{}, err
	}

	return binanceOrderResponseSnapshot{
		Symbol:             resp.Symbol,
		OrderID:            resp.OrderID,
		ClientOrderID:      resp.ClientOrderID,
		EventTime:          resp.TransactTime,
		Price:              resp.Price,
		OrigQty:            resp.OrigQty,
		ExecutedQty:        resp.ExecutedQty,
		CumulativeQuoteQty: resp.CummulativeQuoteQty,
		Status:             resp.Status,
		Type:               resp.Type,
		Side:               resp.Side,
	}, nil
}

func parseBinanceOrderDetailResponse(body []byte, futures bool) (binanceOrderResponseSnapshot, error) {
	if futures {
		var resp binanceFuturesOrderResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return binanceOrderResponseSnapshot{}, err
		}

		eventTime := resp.UpdateTime
		if eventTime <= 0 {
			eventTime = resp.Time
		}

		return binanceOrderResponseSnapshot{
			Symbol:             resp.Symbol,
			OrderID:            resp.OrderID,
			ClientOrderID:      resp.ClientOrderID,
			EventTime:          eventTime,
			Price:              resp.Price,
			OrigQty:            resp.OrigQty,
			ExecutedQty:        resp.ExecutedQty,
			CumulativeQuoteQty: resp.CumQuote,
			Status:             resp.Status,
			Type:               resp.Type,
			Side:               resp.Side,
		}, nil
	}

	var resp binanceSpotOrderDetailResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return binanceOrderResponseSnapshot{}, err
	}

	return binanceOrderResponseSnapshot{
		Symbol:             resp.Symbol,
		OrderID:            resp.OrderID,
		ClientOrderID:      resp.ClientOrderID,
		EventTime:          resp.Time,
		Price:              resp.Price,
		OrigQty:            resp.OrigQty,
		ExecutedQty:        resp.ExecutedQty,
		CumulativeQuoteQty: resp.CummulativeQuoteQty,
		Status:             resp.Status,
		Type:               resp.Type,
		Side:               resp.Side,
	}, nil
}
