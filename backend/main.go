// Command kamishibai is the Kamishibai triage board backend.
//
// It is PocketBase used as a Go framework, which gives one static binary that
// carries the schema migrations, the REST and realtime API, the scheduled
// rollup job, the seeding CLI and the static frontend.
//
//	go run . serve        start the server (applies pending migrations first)
//	go run . migrate up   apply migrations without serving
//	go run . seed         load repeatable demo data
//	go run . rollup       recompute reporting snapshots on demand
package main

import (
	"log"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/pocketbase/pocketbase/tools/osutils"

	"github.com/dbowling/kamishibai/backend/internal/app"
	"github.com/dbowling/kamishibai/backend/internal/cli"
	"github.com/dbowling/kamishibai/backend/internal/config"

	// Registers every schema migration. Without this import the binary would
	// come up against an empty database.
	_ "github.com/dbowling/kamishibai/backend/migrations"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	pb := pocketbase.New()

	// `migrate` subcommands. Automigrate only during `go run`, so that schema
	// edits made in the local dashboard are captured as migration files instead
	// of drifting; it stays off for real builds.
	migratecmd.MustRegister(pb, pb.RootCmd, migratecmd.Config{
		Automigrate:  osutils.IsProbablyGoRun(),
		TemplateLang: migratecmd.TemplateLangGo,
	})

	// Business logic: hooks, custom endpoints and the rollup schedule.
	app.Setup(pb, cfg)

	// Operator commands.
	pb.RootCmd.AddCommand(cli.NewSeedCommand(pb, cfg))
	pb.RootCmd.AddCommand(cli.NewRollupCommand(pb, cfg))

	if err := pb.Start(); err != nil {
		log.Fatal(err)
	}
}
