package entity

type TokocryptoPlaceOrderResponse struct {
	OrderID          int64               `json:"orderId"`
	ClientID         string              `json:"clientId"`
	Symbol           string              `json:"symbol"`
	SymbolType       int32               `json:"symbolType"`
	Side             int32               `json:"side"`
	Type             int32               `json:"type"`
	Price            string              `json:"price"`
	OrigQty          string              `json:"origQty"`
	OrigQuoteQty     string              `json:"origQuoteQty"`
	ExecutedQty      string              `json:"executedQty"`
	ExecutedPrice    string              `json:"executedPrice"`
	ExecutedQuoteQty string              `json:"executedQuoteQty"`
	TimeInForce      int32               `json:"timeInForce"`
	StopPrice        string              `json:"stopPrice"`
	IcebergQty       string              `json:"icebergQty"`
	Status           int32               `json:"status"`
	IsWorking        int32               `json:"isWorking"`
	CreateTime       int64               `json:"createTime"`
	EngineHeaders    map[string][]string `json:"engineHeaders"`
	BorderListID     int64               `json:"borderListId"`
	BorderID         string              `json:"borderId"`
}
