package cmd

import (
	"github.com/krobus00/hft-service/internal/bootstrap"
	"github.com/spf13/cobra"
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "start centralized dashboard API service",
	Long:  "start centralized dashboard API service",
	Run:   bootstrap.StartAPI,
}

func init() {
	rootCmd.AddCommand(apiCmd)
}
