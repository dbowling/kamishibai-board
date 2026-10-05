// Package app wires the application together.
//
// Keeping the wiring in a function rather than inline in main means tests can
// build a fully configured app against a temporary database.
package app

import (
	"log/slog"
	"os"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/dbowling/kamishibai/backend/internal/api"
	"github.com/dbowling/kamishibai/backend/internal/config"
	"github.com/dbowling/kamishibai/backend/internal/hooks"
	"github.com/dbowling/kamishibai/backend/internal/rollup"
)

// RollupJobID identifies the scheduled rollup job.
const RollupJobID = "kamishibai-rollups"

// Setup registers hooks, routes and scheduled jobs on the app.
func Setup(app core.App, cfg config.Config) {
	hooks.Register(app)
	registerRoutes(app, cfg)
	registerCron(app, cfg)
}

func registerRoutes(app core.App, cfg config.Config) {
	handler := api.NewHandler(cfg)

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		handler.Register(se)

		// Serve the built frontend. indexFallback is on so client-side routes such
		// as /boards/abc resolve to index.html instead of 404ing. It only applies
		// to paths that do not match a real file, and PocketBase's own /api and
		// /_ routes are registered separately, so this cannot shadow them.
		se.Router.GET("/{path...}", apis.Static(os.DirFS(cfg.PublicDir), true))

		return se.Next()
	})
}

// registerCron schedules the reporting rollups.
//
// The job only ever summarises periods that have already closed, so it is not
// part of the flip mechanism: cards flip because the period key rolls over,
// whether or not this job ever runs. If it is late, reports briefly fall back to
// computing the window live.
func registerCron(app core.App, cfg config.Config) {
	app.OnBootstrap().BindFunc(func(be *core.BootstrapEvent) error {
		if err := be.Next(); err != nil {
			return err
		}

		// Align the scheduler with the instance default timezone, so a custom
		// schedule written in local terms means what it says rather than following
		// the container's clock, which is usually UTC. The default schedule is
		// hourly and so does not depend on this, but teams have their own zones and
		// each is handled inside the job by its own calendar.
		app.Cron().SetTimezone(cfg.Calendar.Location())

		service := rollup.NewServiceWithCalendars(cfg.Calendars)

		app.Cron().MustAdd(RollupJobID, cfg.RollupCron, func() {
			report, err := service.Run(app, cfg.RollupLookback)
			if err != nil {
				app.Logger().Error("rollup job failed",
					slog.String("job", RollupJobID),
					slog.String("error", err.Error()))
				return
			}

			app.Logger().Info("rollup job finished",
				slog.String("job", RollupJobID),
				slog.Int("boards", report.BoardsScanned),
				slog.Int("periodsChecked", report.PeriodsChecked),
				slog.Int("created", report.Created),
				slog.Int("updated", report.Updated),
				slog.Int("skipped", report.Skipped))
		})

		return nil
	})
}
