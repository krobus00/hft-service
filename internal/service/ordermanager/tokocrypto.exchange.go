package ordermanager

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

type TokocryptoExchange struct {
	apiKey      string
	apiSecret   string
	baseURL     string
	recvWindow  int64
	httpClient  *http.Client
	pairMapping map[string]string

	symbolPrecisionMu sync.RWMutex
	symbolPrecision   map[string]tokocryptoSymbolPrecision
}

type tokocryptoSymbolPrecision struct {
	BasePrecision  int32
	QuotePrecision int32
}

func NewTokocryptoExchange(exchangeConfig config.ExchangeConfig, pairMapping map[string]string) *TokocryptoExchange {
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

	return &TokocryptoExchange{
		apiKey:      strings.TrimSpace(exchangeConfig.APIKey),
		apiSecret:   strings.TrimSpace(exchangeConfig.APISecret),
		baseURL:     strings.TrimRight(baseURL, "/"),
		recvWindow:  recvWindow,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
		pairMapping: pairMapping,
	}
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
	if e.apiKey == "" || e.apiSecret == "" {
		return fmt.Errorf("tokocrypto credentials are missing in config")
	}

	orderSymbol, ok := e.pairMapping[order.Symbol]
	if !ok {
		orderSymbol = order.Symbol
	}

	typeCode, err := tokocryptoOrderTypeCode(order.Type)
	if err != nil {
		return err
	}

	sideCode, err := tokocryptoOrderSideCode(order.Side)
	if err != nil {
		return err
	}

	normalizedQuantity := order.Quantity
	normalizedPrice := order.Price

	if precision, ok, err := e.getSymbolPrecision(ctx, orderSymbol); err != nil {
		logrus.WithError(err).WithField("symbol", orderSymbol).Warn("failed to fetch tokocrypto symbol precision")
	} else if ok {
		normalizedQuantity = order.Quantity.Truncate(precision.BasePrecision)
		if !normalizedQuantity.GreaterThan(decimal.Zero) {
			return fmt.Errorf("tokocrypto order quantity becomes zero after normalization: quantity=%s basePrecision=%d", order.Quantity.String(), precision.BasePrecision)
		}

		if order.Type == OrderTypeLimit {
			normalizedPrice = order.Price.Truncate(precision.QuotePrecision)
			if !normalizedPrice.GreaterThan(decimal.Zero) {
				return fmt.Errorf("tokocrypto order price becomes zero after normalization: price=%s quotePrecision=%d", order.Price.String(), precision.QuotePrecision)
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

	if order.Type == OrderTypeLimit {
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
		return err
	}

	req.Header.Set("X-MBX-APIKEY", e.apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
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
		return fmt.Errorf("tokocrypto order parse failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	if resp.StatusCode >= http.StatusBadRequest || apiResp.Code != 0 || (apiResp.Success != nil && !*apiResp.Success) {
		errMsg := apiResp.Message
		if errMsg == "" {
			errMsg = apiResp.Msg
		}
		if errMsg == "" {
			errMsg = "unknown error"
		}

		return fmt.Errorf("tokocrypto order rejected: status=%d code=%d message=%s", resp.StatusCode, apiResp.Code, errMsg)
	}

	logrus.WithFields(logrus.Fields{
		"exchange": order.Exchange,
		"symbol":   orderSymbol,
		"type":     order.Type,
		"side":     order.Side,
		"price":    normalizedPrice.String(),
		"quantity": normalizedQuantity.String(),
		"source":   order.Source,
		"response": string(apiResp.Data),
	}).Info("order placed")

	return nil
}

func tokocryptoOrderTypeCode(orderType OrderType) (int, error) {
	switch orderType {
	case OrderTypeLimit:
		return 1, nil
	case OrderTypeMarket:
		return 2, nil
	default:
		return 0, fmt.Errorf("unsupported order type for tokocrypto: %s", orderType)
	}
}

func tokocryptoOrderSideCode(orderSide OrderSide) (int, error) {
	switch orderSide {
	case OrderSideBuy:
		return 0, nil
	case OrderSideSell:
		return 1, nil
	default:
		return 0, fmt.Errorf("unsupported order side for tokocrypto: %s", orderSide)
	}
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
