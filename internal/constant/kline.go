package constant

const (
	KlineQueueName  = "kline_queue"
	KlineQueueGroup = "kline_group"

	KlineStreamName       = "kline"
	KlineStreamSubjectAll = "kline.*"

	OrderEngineQueueName  = "order_engine_queue"
	OrderEngineQueueGroup = "order_engine_group"

	OrderEngineStreamName              = "order_engine"
	OrderEngineStreamSubjectAll        = "order_engine.*"
	OrderEngineStreamSubjectPlaceOrder = "order_engine.place_order"
)
