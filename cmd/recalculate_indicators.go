package cmd

import (
	"github.com/krobus00/hft-service/internal/bootstrap"
	"github.com/spf13/cobra"
)

var recalculateIndicatorBatchSize int

var recalculateIndicatorsCmd = &cobra.Command{
	Use:   "recalculate-indicators",
	Short: "recalculate missing kline indicator rows",
	Run:   bootstrap.StartRecalculateIndicators,
}

func init() {
	rootCmd.AddCommand(recalculateIndicatorsCmd)
	recalculateIndicatorsCmd.Flags().IntVar(&recalculateIndicatorBatchSize, "batch-size", 1000, "missing indicator rows to process per batch")
}
