package entity

type MarketKlineIndicatorEvent struct {
	Data       MarketKline        `json:"data"`
	Indicators map[string]float64 `json:"indicators"`
}
