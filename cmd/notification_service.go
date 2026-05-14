/*
Copyright © 2026 Michael Putera Wardana <michaelputeraw@gmail.com>
*/
package cmd

import (
	"github.com/krobus00/hft-service/internal/bootstrap"
	"github.com/spf13/cobra"
)

var notificationServiceCmd = &cobra.Command{
	Use:   "notification-service",
	Short: "Consume order notification events and send Discord alerts",
	Long:  `Notification service consumes order notification JetStream events and forwards them to Discord.`,
	Run:   bootstrap.StartNotificationService,
}

func init() {
	rootCmd.AddCommand(notificationServiceCmd)
}
