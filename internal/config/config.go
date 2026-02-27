package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/shopspring/decimal"
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
	APIKeys                 []APIKeyConfig            `mapstructure:"api_keys"`
	Port                    map[string]string         `mapstructure:"port"`
	Exchanges               map[string]ExchangeConfig `mapstructure:"exchanges"`
	Database                map[string]DatabaseConfig `mapstructure:"database"`
	Redis                   map[string]RedisConfig    `mapstructure:"redis"`
	NatsJetstream           NatsJetstreamConfig       `mapstructure:"nats_jetstream"`
	Strategy                StrategyConfig            `mapstructure:"strategy"`
}

type StrategyConfig struct {
	LazyGrid LazyGridStrategyConfig `mapstructure:"lazy_grid"`
}

type LazyGridStrategyConfig struct {
	ResetStateOnStart bool `mapstructure:"reset_state_on_start"`
}

type APIKeyConfig struct {
	Name      string `mapstructure:"name"`
	Key       string `mapstructure:"key"`
	Active    bool   `mapstructure:"active"`
	ExpiredAt any    `mapstructure:"expired_at"`
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
	CacheDSN string `mapstructure:"cache_dsn"`
}

func LoadConfig(configPath string) error {
	viper.Reset()

	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		viper.SetConfigName("config")
		viper.SetConfigType("yml")
		viper.AddConfigPath(".")
	} else {
		ext := strings.ToLower(filepath.Ext(configPath))
		if ext == ".yml" || ext == ".yaml" {
			viper.SetConfigFile(configPath)
		} else {
			viper.SetConfigName(filepath.Base(configPath))
			viper.SetConfigType("yml")
			configDir := filepath.Dir(configPath)
			if configDir == "." || configDir == "" {
				viper.AddConfigPath(".")
			} else {
				viper.AddConfigPath(configDir)
			}
		}
	}

	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)

	err := viper.ReadInConfig()
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	err = viper.Unmarshal(&Env)
	if err != nil {
		return fmt.Errorf("failed to unmarshal config file: %w", err)
	}

	return nil
}
