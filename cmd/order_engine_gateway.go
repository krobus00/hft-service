/*
Copyright © 2026 Michael Putera Wardana <michaelputeraw@gmail.com>
*/
package cmd

import (
	"github.com/krobus00/hft-service/internal/bootstrap"
	"github.com/spf13/cobra"
)

// orderEngineCmd represents the orderEngine command
var orderEngineCmd = &cobra.Command{
	Use:   "order-engine-gateway",
	Short: "Start the Order Engine Gateway service",
	Long: `The Order Engine Gateway serves as a central gateway for routing and managing 
all order flows to various exchanges. It handles order submission, modification, 
and cancellation requests, providing a unified interface for order management 
across multiple exchange integrations.`,
	Run: bootstrap.StartOrderEngineGateway,
}

func init() {
	rootCmd.AddCommand(orderEngineCmd)
}
