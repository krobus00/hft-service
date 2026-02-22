package config

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	ServiceName    = ""
	ServiceVersion = ""
)

var (
	Env *EnvConfig
)

type EnvConfig struct {
	Env                     string                    `mapstructure:"env"`
	Log                     LogConfig                 `mapstructure:"log"`
	GracefulShutdownTimeout time.Duration             `mapstructure:"graceful_shutdown_timeout"`
	Exchanges               map[string]ExchangeConfig `mapstructure:"exchanges"`
	Database                map[string]DatabaseConfig `mapstructure:"database"`
	Redis                   RedisConfig               `mapstructure:"redis"`
	NatsJetstream           NatsJetstreamConfig       `mapstructure:"nats_jetstream"`
}

type NatsJetstreamConfig struct {
	URL             string                   `mapstructure:"url"`
	MaxRetries      int                      `mapstructure:"max_retries"`
	ReconnectFactor float64                  `mapstructure:"reconnect_factor"`
	MinJitter       time.Duration            `mapstructure:"min_jitter"`
	MaxJitter       time.Duration            `mapstructure:"max_jitter"`
	TimeoutHandler  map[string]time.Duration `mapstructure:"timeout_handler"`
}

type DatabaseConfig struct {
	DSN             string        `mapstructure:"dsn"`
	PingInterval    time.Duration `mapstructure:"ping_interval"`
	ReconnectFactor float64       `mapstructure:"reconnect_factor"`
	MinJitter       time.Duration `mapstructure:"min_jitter"`
	MaxJitter       time.Duration `mapstructure:"max_jitter"`
	MaxRetry        int           `mapstructure:"max_retry"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	MaxActiveConns  int           `mapstructure:"max_active_conns"`
	MaxConnLifetime time.Duration `mapstructure:"max_conn_lifetime"`
}

type LogConfig struct {
	ShowCaller bool   `mapstructure:"show_caller"`
	LogLevel   string `mapstructure:"log_level"`
}

type ExchangeConfig struct {
	Name          string          `mapstructure:"name"`
	APIKey        string          `mapstructure:"api_key"`
	APISecret     string          `mapstructure:"api_secret"`
	LimitBuyFee   decimal.Decimal `mapstructure:"limit_buy_fee"`   // in percentage, e.g. 0.1 for 0.1%
	LimitSellFee  decimal.Decimal `mapstructure:"limit_sell_fee"`  // in percentage, e.g. 0.1 for 0.1%
	MarketBuyFee  decimal.Decimal `mapstructure:"market_buy_fee"`  // in percentage, e.g. 0.1 for 0.1%
	MarketSellFee decimal.Decimal `mapstructure:"market_sell_fee"` // in percentage, e.g. 0.1 for 0.1%
}

type RedisConfig struct {
	MarketData RedisMarketDataConfig `mapstructure:"market_data"`
}

type RedisMarketDataConfig struct {
	CacheDSN string `mapstructure:"cache_dsn"`
}

func LoadConfig() error {
	viper.SetConfigName("config")
	viper.SetConfigType("yml")
	viper.AddConfigPath(".")
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)

	err := viper.ReadInConfig()
	if err != nil {
		logrus.Fatal("failed to read config file: ", err)
	}

	err = viper.Unmarshal(&Env)
	if err != nil {
		logrus.Fatal("failed to unmarshal config file: ", err)
		return err
	}

	return nil
}
