/*
Copyright © 2026 Michael Putera Wardana <michaelputeraw@gmail.com>
*/
package cmd

import (
	"github.com/krobus00/hft-service/internal/bootstrap"
	"github.com/spf13/cobra"
)

// marketDataGatewayCmd represents the marketDataGateway command
var marketDataGatewayCmd = &cobra.Command{
	Use:   "market-data-gateway",
	Short: "Market data gateway service",
	Long: `Market Data Gateway subscribes to market data feeds, receives real-time market information,
stores it persistently, and distributes it to connected strategy services.

This service acts as a central hub that:
- Subscribes to external market data providers
- Receives and processes market updates
- Persists data for historical analysis
- Distributes market data to strategy services in real-time`,
	Run: bootstrap.StartMarketDataGateway,
}

func init() {
	rootCmd.AddCommand(marketDataGatewayCmd)
}
