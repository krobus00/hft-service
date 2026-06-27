package cmd

import (
	"github.com/krobus00/hft-service/internal/bootstrap"
	"github.com/spf13/cobra"
)

var strategyServiceCmd = &cobra.Command{
	Use:   "strategy-service",
	Short: "Run database-defined Go strategy rules",
	Long:  `Strategy service consumes enriched kline indicator events and publishes order requests from enabled strategy rules.`,
	Run:   bootstrap.StartStrategyService,
}

func init() {
	rootCmd.AddCommand(strategyServiceCmd)
}
