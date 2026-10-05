// Package config resolves runtime configuration from the environment.
//
// Everything has a working default so that `go run . serve` needs no setup on a
// developer machine, while a Kubernetes deployment can override each value.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dbowling/kamishibai/backend/internal/domain"
)

// Environment variable names.
const (
	EnvTimezone       = "KAMISHIBAI_TIMEZONE"
	EnvRollupCron     = "KAMISHIBAI_ROLLUP_CRON"
	EnvRollupLookback = "KAMISHIBAI_ROLLUP_LOOKBACK"
	EnvPublicDir      = "KAMISHIBAI_PUBLIC_DIR"
)

// Defaults.
const (
	// DefaultRollupCron runs ten minutes past every hour. Teams can have their
	// own timezone, and every cadence rolls over at local midnight, so some team's
	// midnight falls in almost every hour of the day. Running hourly means each
	// team's closed periods are rolled up within about an hour of its own
	// midnight rather than waiting for the instance default's. The job is an
	// idempotent upsert over a bounded window, so the extra runs are cheap.
	DefaultRollupCron = "10 * * * *"

	// DefaultRollupLookback is how many closed periods back the job checks for
	// a missing snapshot on each run. This is what makes the job self-healing:
	// if the process was down for a few days, the next run backfills instead of
	// leaving permanent holes in the reporting history.
	DefaultRollupLookback = 14

	// DefaultPublicDir is where the built frontend is served from.
	DefaultPublicDir = "./pb_public"
)

// Config is the resolved application configuration.
type Config struct {
	// Timezone is the instance default IANA name: the zone period boundaries are
	// evaluated in for any team that has not set its own.
	Timezone string

	// Calendar is built from Timezone. It is the instance default, not the
	// calendar for any particular team; use Calendars to resolve those.
	Calendar *domain.Calendar

	// Calendars resolves a team's timezone to a Calendar, falling back to
	// Calendar when the team's is empty.
	Calendars *domain.Calendars

	// RollupCron is the crontab expression for the rollup job, interpreted in
	// Timezone. The job covers every team's zone, not only that one.
	RollupCron string

	// RollupLookback is how many closed periods to check per run.
	RollupLookback int

	// PublicDir holds the static frontend bundle.
	PublicDir string
}

// FromEnv builds a Config from environment variables, falling back to defaults.
//
// It returns an error rather than silently correcting a bad value, because a
// mistyped timezone would otherwise quietly shift every period boundary.
func FromEnv() (Config, error) {
	cfg := Config{
		Timezone:       envOr(EnvTimezone, domain.DefaultTimezone),
		RollupCron:     envOr(EnvRollupCron, DefaultRollupCron),
		RollupLookback: DefaultRollupLookback,
		PublicDir:      envOr(EnvPublicDir, DefaultPublicDir),
	}

	cal, err := domain.LoadCalendar(cfg.Timezone)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", EnvTimezone, err)
	}
	cfg.Calendar = cal
	cfg.Calendars = domain.NewCalendars(cal)

	if raw := os.Getenv(EnvRollupLookback); raw != "" {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return Config{}, fmt.Errorf("%s: %q is not a number", EnvRollupLookback, raw)
		}
		if n < 1 {
			return Config{}, fmt.Errorf("%s: must be at least 1, got %d", EnvRollupLookback, n)
		}
		cfg.RollupLookback = n
	}

	return cfg, nil
}

// Default returns the configuration used when nothing is set, panicking only if
// the embedded timezone database is somehow unavailable. Handy in tests.
func Default() Config {
	cal, err := domain.LoadCalendar(domain.DefaultTimezone)
	if err != nil {
		panic(fmt.Sprintf("load default calendar: %v", err))
	}
	return Config{
		Timezone:       domain.DefaultTimezone,
		Calendar:       cal,
		Calendars:      domain.NewCalendars(cal),
		RollupCron:     DefaultRollupCron,
		RollupLookback: DefaultRollupLookback,
		PublicDir:      DefaultPublicDir,
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
