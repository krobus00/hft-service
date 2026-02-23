/*
Copyright © 2026 Michael Putera Wardana <michaelputeraw@gmail.com>
*/
package cmd

import (
	"github.com/krobus00/hft-service/internal/bootstrap"
	"github.com/spf13/cobra"
)

// orderEngineWorkerCmd represents the order engine worker command
var orderEngineWorkerCmd = &cobra.Command{
	Use:   "order-engine-worker",
	Short: "Sync order engine with exchanges",
	Long:  `The order engine worker is responsible for synchronizing order history data between the service and the connected exchanges.`,
	Run:   bootstrap.StartOrderEngineWorker,
}

func init() {
	rootCmd.AddCommand(orderEngineWorkerCmd)
}
