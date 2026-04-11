/*
Copyright © 2026 Michael Putera Wardana <michaelputeraw@gmail.com>
*/
package cmd

import (
	"github.com/krobus00/hft-service/internal/bootstrap"
	"github.com/spf13/cobra"
)

// dashboardGatewayCmd represents the dashboardGateway command
var dashboardGatewayCmd = &cobra.Command{
	Use:   "dashboard-gateway",
	Short: "Start the dashboard gateway service",
	Long: `The Dashboard Gateway provides authentication APIs for the dashboard,
including access token, refresh token, and role-based authorization checks.`,
	Run: bootstrap.StartDashboardGateway,
}

func init() {
	rootCmd.AddCommand(dashboardGatewayCmd)
}
