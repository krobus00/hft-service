package constant

import (
	"fmt"
	"strings"
)

const (
	KlineQueueNameInsert   = "KLINE_INSERT"
	KlineQueueNameStrategy = "KLINE_STRATEGY"

	KlineStreamName       = "KLINE"
	KlineStreamSubjectAll = "KLINE.>"

	OrderEngineQueueName  = "order_engine_queue"
	OrderEngineQueueGroup = "order_engine_group"

	OrderEngineStreamName              = "order_engine"
	OrderEngineStreamSubjectAll        = "order_engine.*"
	OrderEngineStreamSubjectPlaceOrder = "order_engine.place_order"
)

func GetKlineStreamSubject(exchange, symbol, interval string) string {
	subject := fmt.Sprintf("KLINE.%s.%s.%s", exchange, symbol, interval)
	return strings.ToUpper(subject)
}

func GetKlineExchangeStreamSubject(exchange string) string {
	subject := fmt.Sprintf("KLINE.%s.>", exchange)
	return strings.ToUpper(subject)
}

func GetKlineInsertQueueGroup(exchange string) string {
	queueGroup := fmt.Sprintf("%s_%s", KlineQueueNameInsert, exchange)
	return strings.ToUpper(queueGroup)
}

func GetKlineStrategyQueueGroup(exchange string, strategy string) string {
	queueGroup := fmt.Sprintf("%s_%s_%s", KlineQueueNameStrategy, exchange, strategy)
	return strings.ToUpper(queueGroup)
}
