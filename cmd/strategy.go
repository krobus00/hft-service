/*
Copyright © 2026 Michael Putera Wardana <michaelputeraw@gmail.com>
*/
package cmd

import (
	"github.com/krobus00/hft-service/internal/bootstrap"
	"github.com/spf13/cobra"
)

// strategyCmd represents the strategy command
var strategyCmd = &cobra.Command{
	Use:   "strategy",
	Short: "Run trading strategy",
	Long:  `Run strategy workers such as lazy-grid using market data stream.`,
	Run: func(cmd *cobra.Command, args []string) {
		bootstrap.StartLazyGridStrategy()
	},
}

func init() {
	rootCmd.AddCommand(strategyCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// strategyCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// strategyCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
