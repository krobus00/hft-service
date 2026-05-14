package bootstrap

import (
	"context"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/infrastructure"
	"github.com/krobus00/hft-service/internal/service/notification"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/spf13/cobra"
)

func StartNotificationService(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nc, js, err := infrastructure.NewJetstream()
	util.ContinueOrFatal(err)

	notificationService := notification.NewService(js)

	err = notificationService.JetstreamEventSubscribe(ctx)
	util.ContinueOrFatal(err)

	wait := gracefulShutdown(ctx, config.Env.GracefulShutdownTimeout, map[string]operation{
		"nats connection": func(ctx context.Context) error {
			cancel()
			return infrastructure.CloseJetstream(nc)
		},
	})

	<-wait
}
