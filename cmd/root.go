/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/constant"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "hft-service",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	err := config.LoadConfig()
	if err != nil {
		logrus.Fatal(err)
	}

	logrus.SetReportCaller(config.Env.Log.ShowCaller)

	if config.Env.Env == constant.ProductionEnvironment {
		logrus.SetFormatter(&logrus.JSONFormatter{})
	}

	logLevel, err := logrus.ParseLevel(config.Env.Log.LogLevel)
	if err != nil {
		logrus.Fatal(err)
	}
	logrus.SetLevel(logLevel)
}
