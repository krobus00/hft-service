/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/krobus00/hft-service/internal/bootstrap"
	"github.com/spf13/cobra"
)

// marketDataGatewayCmd represents the marketDataGateway command
var marketDataGatewayCmd = &cobra.Command{
	Use:   "market-data-gateway",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		bootstrap.StartMarketDataGateway()
	},
}

func init() {
	rootCmd.AddCommand(marketDataGatewayCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// marketDataGatewayCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// marketDataGatewayCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
