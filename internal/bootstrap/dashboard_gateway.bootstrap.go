package bootstrap

import (
	"context"
	"fmt"

	"github.com/krobus00/hft-service/internal/config"
	httpHandler "github.com/krobus00/hft-service/internal/handler/dashboard/http"
	"github.com/krobus00/hft-service/internal/infrastructure"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/service/dashboard"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func StartDashboardGateway(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dashboardDB, err := infrastructure.NewPostgresConnection(ctx, config.Env.Database["dashboard"])
	util.ContinueOrFatal(err)
	infrastructure.StartPostgresHealthCheck(ctx, dashboardDB, config.Env.Database["dashboard"].PingInterval)

	authRepo := repository.NewDashboardAuthRepository(dashboardDB)
	authService := dashboard.NewDashboardAuthService(authRepo)
	authHandler := httpHandler.NewDashboardAuthHTTPHandler(authService)

	httpMux := infrastructure.NewAPIServeMux()
	authHandler.Register(httpMux)

	httpPort := fmt.Sprintf(":%s", config.Env.Port["dashboard_gateway_http"])
	httpServer := infrastructure.NewHTTPServerWithConfig(infrastructure.HTTPServerConfig{
		Addr:            httpPort,
		ShutdownTimeout: config.Env.GracefulShutdownTimeout,
	}, httpMux)

	go func() {
		err := httpServer.Start()
		if err != nil {
			logrus.Error(err)
		}
	}()
	logrus.Info(fmt.Sprintf("dashboard http server started on %s", httpPort))

	wait := gracefulShutdown(ctx, config.Env.GracefulShutdownTimeout, map[string]operation{
		"dashboard database": func(ctx context.Context) error {
			cancel()
			return dashboardDB.Close()
		},
		"http": func(ctx context.Context) error {
			return httpServer.Shutdown(ctx)
		},
	})

	<-wait
}
