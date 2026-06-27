package indicator

import (
	"context"
	"testing"
	"time"

	"github.com/krobus00/hft-service/internal/entity"
	"github.com/shopspring/decimal"
)

func TestCalculateLatestUsesConfiguredIndicators(t *testing.T) {
	rows := make([]entity.MarketKline, 0, 20)
	for i := 1; i <= 20; i++ {
		rows = append(rows, entity.MarketKline{
			Exchange:    "BINANCE",
			MarketType:  "spot",
			Symbol:      "BTC_USDT",
			Interval:    "1m",
			CloseTime:   time.Unix(int64(i), 0),
			HighPrice:   decimal.NewFromInt(int64(i + 1)),
			LowPrice:    decimal.NewFromInt(int64(i - 1)),
			ClosePrice:  decimal.NewFromInt(int64(i)),
			QuoteVolume: decimal.NewFromInt(10),
			IsClosed:    true,
		})
	}

	cfgs := []entity.IndicatorConfig{
		{Indicator: "ema", OutputName: "ema_20", Params: `{"period":20}`, Enabled: true},
		{Indicator: "vwap", OutputName: "vwap_20", Params: `{"period":20}`, Enabled: true},
		{Indicator: "bollinger_bands", OutputName: "bb_20_2", Params: `{"period":20}`, Enabled: true},
	}
	out := calculateLatest(context.Background(), rows, prepareSpecs(cfgs))

	assertClose(t, out["ema_20"], 10.5, 0.000000001)
	assertClose(t, out["vwap_20"], 10.5, 0.000000001)
	if _, ok := out["bb_20_2_upper"]; !ok {
		t.Fatal("expected bb_20_2_upper")
	}
	if _, ok := out["bb_20_2_mid"]; !ok {
		t.Fatal("expected bb_20_2_mid")
	}
	if _, ok := out["bb_20_2_lower"]; !ok {
		t.Fatal("expected bb_20_2_lower")
	}
}

func TestMaxLookbackUsesConfiguredNeed(t *testing.T) {
	specs := prepareSpecs([]entity.IndicatorConfig{
		{Indicator: "ema", OutputName: "ema_21", Params: `{"period":21}`, Enabled: true},
		{Indicator: "macd", OutputName: "macd", Params: `{"fast":12,"slow":26,"signal":9}`, Enabled: true},
	})

	if got := maxLookback(specs); got != 35 {
		t.Fatalf("got lookback %d want 35", got)
	}
}

func assertClose(t *testing.T, got, want, epsilon float64) {
	t.Helper()
	if got < want-epsilon || got > want+epsilon {
		t.Fatalf("got %.12f want %.12f", got, want)
	}
}
