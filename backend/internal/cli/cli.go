// Package cli provides the operator commands attached to the binary.
//
// They run through PocketBase's own cobra root command, which means the app is
// already bootstrapped and the database is open by the time a command executes.
package cli

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"

	"github.com/dbowling/kamishibai/backend/internal/config"
	"github.com/dbowling/kamishibai/backend/internal/rollup"
	"github.com/dbowling/kamishibai/backend/internal/seed"
)

// NewSeedCommand returns the `seed` command.
func NewSeedCommand(app core.App, cfg config.Config) *cobra.Command {
	var (
		reset    bool
		history  int
		password string
		rollups  bool
	)

	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Load repeatable demo data",
		Long: "Load a repeatable demo dataset: users, teams, boards, cards and " +
			"invented completion history.\n\n" +
			"Safe to run more than once. Records are matched by natural key (email, " +
			"team name, board name, card title) and updated in place rather than " +
			"duplicated, and the generated history is driven by a fixed seed so the " +
			"result is identical every time.\n\n" +
			"Every account uses the reserved .test domain, and --reset only removes " +
			"records matching the demo dataset's own keys.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Seeding writes to collections whose schema must already exist.
			if err := app.RunAllMigrations(); err != nil {
				return fmt.Errorf("apply migrations: %w", err)
			}

			seeder := seed.New(cfg.Calendar, seed.Options{
				Reset:          reset,
				HistoryPeriods: history,
				Password:       password,
				Rollups:        rollups,
			})

			result, err := seeder.Run(app)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if result.Deleted > 0 {
				fmt.Fprintf(out, "Removed %d previously seeded team(s) and user(s).\n", result.Deleted)
			}
			fmt.Fprintf(out, "Seeded %d users, %d teams, %d boards, %d cards.\n",
				result.Users, result.Teams, result.Boards, result.Cards)
			if result.Occurrences > 0 {
				fmt.Fprintf(out, "Generated %d historical occurrences across the last %d closed periods.\n",
					result.Occurrences, history)
			}
			if result.Rollups.Created > 0 || result.Rollups.Updated > 0 {
				fmt.Fprintf(out, "Reporting snapshots: %d created, %d updated.\n",
					result.Rollups.Created, result.Rollups.Updated)
			}
			fmt.Fprintf(out, "\nSign in as admin@example.test or dana@example.test with the password %q.\n", password)

			return nil
		},
	}

	cmd.Flags().BoolVar(&reset, "reset", false,
		"remove previously seeded teams and users first (only records matching the demo dataset)")
	cmd.Flags().IntVar(&history, "history", seed.DefaultHistoryPeriods,
		"how many closed periods of completion history to generate (0 for none)")
	cmd.Flags().StringVar(&password, "password", seed.DefaultPassword,
		"password for the seeded accounts")
	cmd.Flags().BoolVar(&rollups, "rollups", true,
		"compute reporting snapshots for the generated history")

	return cmd
}

// NewRollupCommand returns the `rollup` command, for recomputing reporting
// snapshots on demand.
//
// Useful after a backfill, after importing data, or to catch up a deployment that
// was down when a period closed. Safe to run at any time: snapshots are keyed by
// board, cadence and period, so re-running converges rather than duplicating.
func NewRollupCommand(app core.App, cfg config.Config) *cobra.Command {
	var lookback int

	cmd := &cobra.Command{
		Use:   "rollup",
		Short: "Recompute reporting snapshots for closed periods",
		Long: "Walk back over recently closed periods and make sure each has a " +
			"reporting snapshot.\n\n" +
			"Only periods that have fully elapsed are summarised; the period in " +
			"progress is left alone because work can still land in it.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.RunAllMigrations(); err != nil {
				return fmt.Errorf("apply migrations: %w", err)
			}

			report, err := rollup.NewService(cfg.Calendar).Run(app, lookback)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(),
				"Scanned %d board(s) over %d period(s): %d created, %d updated, %d skipped.\n",
				report.BoardsScanned, report.PeriodsChecked,
				report.Created, report.Updated, report.Skipped)

			return nil
		},
	}

	cmd.Flags().IntVar(&lookback, "lookback", cfg.RollupLookback,
		"how many closed periods back to check per cadence")

	return cmd
}
