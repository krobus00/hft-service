package bootstrap

import (
	"database/sql"
	"errors"

	"github.com/guregu/null/v6"
	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/pressly/goose/v3"
	"github.com/spf13/cobra"
)

func StartMigrate(cmd *cobra.Command, args []string) {
	databaseName, _ := cmd.Flags().GetString("databaseName")
	actionType, _ := cmd.Flags().GetString("action")
	migrationName, _ := cmd.Flags().GetString("name")
	version, _ := cmd.Flags().GetInt64("version")

	migrationDir := "migration/postgresql/"

	migrationDir += databaseName

	var err error

	db, err := sql.Open("postgres", config.Env.Database[databaseName].DSN)
	util.ContinueOrFatal(err)
	err = goose.SetDialect("postgres")
	util.ContinueOrFatal(err)

	switch actionType {
	case "create":
		err = goose.Create(db, migrationDir, migrationName, "sql")
	case "up":
		err = goose.Up(db, migrationDir, goose.WithAllowMissing())
	case "up-by-one":
		err = goose.UpByOne(db, migrationDir, goose.WithAllowMissing())
	case "up-to":
		err = goose.UpTo(db, migrationDir, null.IntFrom(version).Int64, goose.WithAllowMissing())
	case "down":
		err = goose.Down(db, migrationDir, goose.WithAllowMissing())
	case "down-to":
		err = goose.DownTo(db, migrationDir, null.IntFrom(version).Int64, goose.WithAllowMissing())
	case "status":
		err = goose.Status(db, migrationDir)
	case "reset":
		err = goose.Reset(db, migrationDir, goose.WithAllowMissing())
		if err != nil {
			break
		}
		err = goose.Up(db, migrationDir, goose.WithAllowMissing())
	default:
		err = errors.New("invalid command")
	}

	util.ContinueOrFatal(err)
}
