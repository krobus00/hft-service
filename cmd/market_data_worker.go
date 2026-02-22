/*
Copyright © 2026 Michael Putera Wardana <michaelputeraw@gmail.com>
*/
package cmd

import (
	"github.com/krobus00/hft-service/internal/bootstrap"
	"github.com/spf13/cobra"
)

// marketDataWorkerCmd represents the marketDataWorker command
var marketDataWorkerCmd = &cobra.Command{
	Use:   "market-data-worker",
	Short: "Consume market data from market-data-gateway",
	Long: `Consumes published market data from market-data-gateway and processes it.
This worker subscribes to market data feeds and handles real-time data updates.`,
	Run: bootstrap.StartMarketDataWorker,
}

func init() {
	rootCmd.AddCommand(marketDataWorkerCmd)
}
