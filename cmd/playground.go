/*
Copyright © 2026 Michael Putera Wardana <michaelputeraw@gmail.com>
*/
package cmd

import (
	"context"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"
)

// playgroundCmd represents the playground command
var playgroundCmd = &cobra.Command{
	Use:   "playground",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		nc, err := nats.Connect("nats://localhost:4222")
		if err != nil {
			log.Fatal(err)
		}
		defer nc.Drain()

		js, err := nc.JetStream()
		if err != nil {
			log.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Create or update stream
		_, err = js.AddStream(&nats.StreamConfig{
			Name:      "KLINE",
			Subjects:  []string{"KLINE.>"},
			Storage:   nats.FileStorage, // use MemoryStorage for ultra-low latency
			Retention: nats.LimitsPolicy,
			MaxAge:    1 * time.Hour,
			Replicas:  1,
		}, nats.Context(ctx))

		if err != nil {
			// If already exists, update instead
			_, err = js.UpdateStream(&nats.StreamConfig{
				Name:      "KLINE",
				Subjects:  []string{"KLINE.>"},
				Storage:   nats.FileStorage,
				Retention: nats.LimitsPolicy,
				MaxAge:    1 * time.Hour,
				Replicas:  1,
			}, nats.Context(ctx))
			if err != nil {
				log.Fatal(err)
			}
		}

		log.Println("KLINE stream ready")
	},
}

func init() {
	rootCmd.AddCommand(playgroundCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// playgroundCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// playgroundCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
