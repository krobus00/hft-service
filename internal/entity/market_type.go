package entity

import "strings"

type MarketType string

const (
	MarketTypeSpot    MarketType = "spot"
	MarketTypeFutures MarketType = "futures"
)

func NormalizeMarketType(raw string) MarketType {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == string(MarketTypeFutures) {
		return MarketTypeFutures
	}

	return MarketTypeSpot
}
