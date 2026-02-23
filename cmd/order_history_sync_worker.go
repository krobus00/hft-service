/*
Copyright © 2026 Michael Putera Wardana <michaelputeraw@gmail.com>
*/
package cmd

import (
	"github.com/krobus00/hft-service/internal/bootstrap"
	"github.com/spf13/cobra"
)

// orderHistorySyncWorkerCmd represents the order history sync worker command
var orderHistorySyncWorkerCmd = &cobra.Command{
	Use:   "order-history-sync-worker",
	Short: "Sync order history status with exchanges",
	Long: `Periodically fetches order updates for orders in NEW or PARTIAL state
and updates the order engine history records with the latest exchange status.`,
	Run: bootstrap.StartOrderHistorySyncWorker,
}

func init() {
	rootCmd.AddCommand(orderHistorySyncWorkerCmd)
}
