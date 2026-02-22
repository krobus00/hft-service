/*
Copyright © 2026 Michael Putera Wardana <michaelputeraw@gmail.com>
*/
package cmd

import (
	"github.com/krobus00/hft-service/internal/bootstrap"
	"github.com/spf13/cobra"
)

// lazyGridStrategyCmd represents the lazyGridStrategy command
var lazyGridStrategyCmd = &cobra.Command{
	Use:   "lazy-grid-strategy",
	Short: "Run trading lazyGridStrategy",
	Long:  `Run lazyGridStrategy workers such as lazy-grid using market data stream.`,
	Run:   bootstrap.StartLazyGridStrategy,
}

func init() {
	rootCmd.AddCommand(lazyGridStrategyCmd)
}
