package bootstrap

import (
	"context"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/infrastructure"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/service/indicator"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func StartRecalculateIndicators(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	batchSize, _ := cmd.Flags().GetInt("batch-size")
	if batchSize <= 0 {
		batchSize = 1000
	}

	db, err := infrastructure.NewPostgresConnection(ctx, config.Env.Database["market_data"])
	util.ContinueOrFatal(err)
	defer db.Close()

	service := indicator.NewService(
		nil,
		repository.NewMarketKlineRepository(db),
		repository.NewIndicatorConfigRepository(db),
		repository.NewIndicatorResultRepository(db),
	)

	total := 0
	for {
		processed, err := service.RecalculateMissing(ctx, batchSize)
		util.ContinueOrFatal(err)
		total += processed
		logrus.WithFields(logrus.Fields{"processed": processed, "total": total}).Info("recalculated missing indicators")
		if processed < batchSize {
			return
		}
	}
}
