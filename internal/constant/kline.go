package constant

const (
	KlineQueueNameInsert   = "kline_queue_insert"
	KlineQueueNameStrategy = "kline_queue_strategy"
	KlineQueueGroup        = "kline_group"

	KlineStreamName        = "kline"
	KlineStreamSubjectAll  = "kline.*"
	KlineStreamSubjectData = "kline.data"

	OrderEngineQueueName  = "order_engine_queue"
	OrderEngineQueueGroup = "order_engine_group"

	OrderEngineStreamName              = "order_engine"
	OrderEngineStreamSubjectAll        = "order_engine.*"
	OrderEngineStreamSubjectPlaceOrder = "order_engine.place_order"
)
