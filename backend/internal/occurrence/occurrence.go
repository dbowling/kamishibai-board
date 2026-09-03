// Package occurrence reads and writes the per-period state of cards.
//
// It is the only place that creates occurrence rows, and it enforces the two
// invariants the rest of the system depends on:
//
//   - A row exists only if somebody actually touched the card. "Not started" is
//     the absence of a row, which is what keeps the table small and makes the
//     board flip without any write.
//
//   - A row is only ever written for the period that is open right now,
//     according to the server clock. Closed periods are immutable, so the
//     reporting snapshots taken from them can be trusted.
package occurrence

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/dbowling/kamishibai/backend/internal/access"
	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// Sentinel errors, so callers (chiefly the HTTP layer) can map failures onto
// status codes without string matching.
var (
	// ErrForbidden means the user is not on the card's team.
	ErrForbidden = errors.New("not a member of this team")

	// ErrArchived means the card, its board or its team has been archived and is
	// therefore read-only.
	ErrArchived = errors.New("archived and read-only")

	// ErrCardNotFound means no such card exists.
	ErrCardNotFound = errors.New("card not found")

	// ErrAlreadyComplete means the card was already completed this period.
	ErrAlreadyComplete = errors.New("already completed for this period")

	// ErrNothingToReopen means there is no recorded work for this period.
	ErrNothingToReopen = errors.New("nothing recorded for this period")
)

// State is a card's status within a single period.
//
// Occurrence is nil when Status is not_started, which is the common case and
// costs no storage.
type State struct {
	CardID     string
	Cadence    domain.Cadence
	Period     domain.Period
	Status     domain.Status
	Occurrence *core.Record
}

// Service resolves and mutates card state.
type Service struct {
	cal *domain.Calendar
	now func() time.Time
}

// NewService returns a Service that evaluates periods using cal.
func NewService(cal *domain.Calendar) *Service {
	return &Service{cal: cal, now: time.Now}
}

// WithClock returns a copy of the service using a custom clock. Tests use it to
// pin "now" to a specific period; production always uses time.Now.
func (s *Service) WithClock(now func() time.Time) *Service {
	clone := *s
	clone.now = now
	return &clone
}

// Calendar exposes the calendar the service evaluates periods against.
func (s *Service) Calendar() *domain.Calendar { return s.cal }

// Now returns the service's current time.
func (s *Service) Now() time.Time { return s.now() }

// CurrentPeriod returns the period a card is currently in.
func (s *Service) CurrentPeriod(card *core.Record) (domain.Period, error) {
	cadence := domain.Cadence(card.GetString(schema.FieldCadence))
	period, err := s.cal.At(cadence, s.now())
	if err != nil {
		return domain.Period{}, fmt.Errorf("card %q: %w", card.Id, err)
	}
	return period, nil
}

// StateOf resolves the current state of a single card.
func (s *Service) StateOf(app core.App, card *core.Record) (State, error) {
	states, err := s.StatesFor(app, []*core.Record{card})
	if err != nil {
		return State{}, err
	}
	return states[card.Id], nil
}

// StatesFor resolves the current state of many cards using a single query
// against the occurrences table.
//
// Cards on one board can mix cadences, so they can be looking at different
// period keys at the same moment. Rather than querying per card, this collects
// the distinct keys in play (at most one per cadence, so five) and issues one
// `card IN (...) AND period_key IN (...)` lookup, served by the unique
// (card, period_key) index. Rendering a board is therefore one query regardless
// of how many cards it holds.
func (s *Service) StatesFor(app core.App, cards []*core.Record) (map[string]State, error) {
	states := make(map[string]State, len(cards))
	if len(cards) == 0 {
		return states, nil
	}

	now := s.now()
	cardIDs := make([]any, 0, len(cards))
	keySet := make(map[string]struct{}, len(domain.Cadences()))

	for _, card := range cards {
		cadence := domain.Cadence(card.GetString(schema.FieldCadence))
		period, err := s.cal.At(cadence, now)
		if err != nil {
			return nil, fmt.Errorf("card %q: %w", card.Id, err)
		}

		// Default every card to not-started. Anything we find in the database
		// upgrades it from here.
		states[card.Id] = State{
			CardID:  card.Id,
			Cadence: cadence,
			Period:  period,
			Status:  domain.StatusNotStarted,
		}
		cardIDs = append(cardIDs, card.Id)
		keySet[period.Key] = struct{}{}
	}

	keys := make([]any, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}

	var records []*core.Record
	err := app.RecordQuery(schema.Occurrences).
		AndWhere(dbx.In(schema.FieldCard, cardIDs...)).
		AndWhere(dbx.In(schema.FieldPeriodKey, keys...)).
		All(&records)
	if err != nil {
		return nil, fmt.Errorf("load occurrences: %w", err)
	}

	for _, rec := range records {
		state, ok := states[rec.GetString(schema.FieldCard)]
		if !ok {
			continue
		}
		// The two IN clauses form a cross product, so a daily card can match a
		// weekly key belonging to some other card. Only the row for this card's
		// own current period counts.
		if rec.GetString(schema.FieldPeriodKey) != state.Period.Key {
			continue
		}

		status, err := domain.ParseStatus(rec.GetString(schema.FieldStatus))
		if err != nil {
			return nil, fmt.Errorf("occurrence %q: %w", rec.Id, err)
		}
		state.Status = status
		state.Occurrence = rec
		states[state.CardID] = state
	}

	return states, nil
}

// Find returns the occurrence for a card and period, or nil if none exists.
//
// A nil result is the normal "not started" case, not an error.
func (s *Service) Find(app core.App, cardID, periodKey string) (*core.Record, error) {
	rec, err := app.FindFirstRecordByFilter(
		schema.Occurrences,
		"card = {:card} && period_key = {:key}",
		dbx.Params{"card": cardID, "key": periodKey},
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find occurrence: %w", err)
	}
	return rec, nil
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

// Start marks a card as picked up for the current period.
//
// Idempotent: starting an already-started card keeps the original starter, so
// attribution reflects who actually took it on.
func (s *Service) Start(app core.App, cardID string, user *core.Record, notes string) (*core.Record, error) {
	return s.mutate(app, cardID, user, notes, func(occ *core.Record, existed bool, now types.DateTime, userID string) error {
		if existed {
			switch domain.Status(occ.GetString(schema.FieldStatus)) {
			case domain.StatusDone:
				return ErrAlreadyComplete
			case domain.StatusInProgress:
				// Leave started_by alone: the first person to pick it up owns it.
				return nil
			}
		}

		occ.Set(schema.FieldStatus, string(domain.StatusInProgress))
		occ.Set(schema.FieldStartedBy, userID)
		occ.Set(schema.FieldStartedAt, now)
		return nil
	})
}

// Complete marks a card as done for the current period.
//
// Completing a card nobody started is allowed and attributes the start to the
// same person: plenty of triage tasks are quick enough that nobody bothers
// pressing Start first, and losing that record would be worse than inferring it.
func (s *Service) Complete(app core.App, cardID string, user *core.Record, notes string) (*core.Record, error) {
	return s.mutate(app, cardID, user, notes, func(occ *core.Record, existed bool, now types.DateTime, userID string) error {
		if existed && domain.Status(occ.GetString(schema.FieldStatus)) == domain.StatusDone {
			// Idempotent: preserve the original completer.
			return nil
		}

		if occ.GetString(schema.FieldStartedBy) == "" {
			occ.Set(schema.FieldStartedBy, userID)
			occ.Set(schema.FieldStartedAt, now)
		}

		occ.Set(schema.FieldStatus, string(domain.StatusDone))
		occ.Set(schema.FieldCompletedBy, userID)
		occ.Set(schema.FieldCompletedAt, now)
		return nil
	})
}

// Reopen undoes a completion for the current period, putting the card back to
// in-progress.
//
// The row is kept rather than deleted so the original start attribution
// survives. There is deliberately no way to erase an occurrence through this
// service: the audit trail of who did what is the point of the board.
func (s *Service) Reopen(app core.App, cardID string, user *core.Record, notes string) (*core.Record, error) {
	return s.mutate(app, cardID, user, notes, func(occ *core.Record, existed bool, now types.DateTime, userID string) error {
		if !existed {
			return ErrNothingToReopen
		}
		if domain.Status(occ.GetString(schema.FieldStatus)) != domain.StatusDone {
			return nil // already open
		}

		occ.Set(schema.FieldStatus, string(domain.StatusInProgress))
		occ.Set(schema.FieldCompletedBy, "")
		occ.Set(schema.FieldCompletedAt, "")
		return nil
	})
}

// transition applies a state change to an occurrence record that has already
// been located or freshly built.
type transition func(occ *core.Record, existed bool, now types.DateTime, userID string) error

// maxMutateAttempts bounds the retry loop used to resolve write races.
const maxMutateAttempts = 3

// mutate runs a transition against the card's current period inside a
// transaction, retrying if a concurrent writer wins the race to create the row.
//
// Two people pressing Start on the same card at the same instant both see no
// existing row and both try to insert. The unique (card, period_key) index lets
// exactly one succeed; the loser retries, now finds the row, and updates it
// instead. The constraint lives in the database rather than in a lock here,
// which is what makes this correct rather than merely unlikely to break.
func (s *Service) mutate(app core.App, cardID string, user *core.Record, notes string, fn transition) (*core.Record, error) {
	if user == nil {
		return nil, ErrForbidden
	}

	var lastErr error

	for attempt := 0; attempt < maxMutateAttempts; attempt++ {
		var saved *core.Record

		err := app.RunInTransaction(func(txApp core.App) error {
			rec, err := s.mutateOnce(txApp, cardID, user, notes, fn)
			if err != nil {
				return err
			}
			saved = rec
			return nil
		})

		if err == nil {
			return saved, nil
		}
		if !isUniqueViolation(err) {
			return nil, err
		}
		lastErr = err
	}

	return nil, fmt.Errorf("could not record the change after %d attempts: %w", maxMutateAttempts, lastErr)
}

func (s *Service) mutateOnce(app core.App, cardID string, user *core.Record, notes string, fn transition) (*core.Record, error) {
	card, err := app.FindRecordById(schema.Cards, cardID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCardNotFound
		}
		return nil, fmt.Errorf("load card: %w", err)
	}

	// Authorisation and archive checks happen here rather than in the HTTP
	// handler so that no future caller can skip them.
	team, err := access.LoadTeam(app, card)
	if err != nil {
		return nil, err
	}
	if !access.CanReadTeam(team, user) {
		return nil, ErrForbidden
	}

	board, err := app.FindRecordById(schema.Boards, card.GetString(schema.FieldBoard))
	if err != nil {
		return nil, fmt.Errorf("load board: %w", err)
	}

	// Archiving anything up the chain freezes the work beneath it.
	for label, record := range map[string]*core.Record{"team": team, "board": board, "card": card} {
		if access.IsArchived(record) {
			return nil, fmt.Errorf("%s is %w", label, ErrArchived)
		}
	}

	period, err := s.CurrentPeriod(card)
	if err != nil {
		return nil, err
	}

	occ, err := s.Find(app, card.Id, period.Key)
	if err != nil {
		return nil, err
	}

	existed := occ != nil
	if !existed {
		collection, err := app.FindCollectionByNameOrId(schema.Occurrences)
		if err != nil {
			return nil, fmt.Errorf("load occurrences collection: %w", err)
		}
		occ = core.NewRecord(collection)
		occ.Set(schema.FieldCard, card.Id)
		occ.Set(schema.FieldPeriodKey, period.Key)

		// Denormalised from the card so reporting never has to join back through
		// cards and boards. Derived server-side, never taken from the client.
		occ.Set(schema.FieldBoard, card.GetString(schema.FieldBoard))
		occ.Set(schema.FieldTeam, card.GetString(schema.FieldTeam))
		occ.Set(schema.FieldCadence, card.GetString(schema.FieldCadence))
	}

	now, err := types.ParseDateTime(s.now())
	if err != nil {
		return nil, fmt.Errorf("convert timestamp: %w", err)
	}

	if err := fn(occ, existed, now, user.Id); err != nil {
		return nil, err
	}

	// Notes are additive context on the occurrence; an empty submission leaves
	// any existing note untouched.
	if notes != "" {
		occ.Set(schema.FieldNotes, notes)
	}

	if err := app.Save(occ); err != nil {
		return nil, err
	}
	return occ, nil
}

// isUniqueViolation reports whether err is the unique-index conflict raised when
// two writers race to create the same occurrence.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// modernc.org/sqlite reports SQLITE_CONSTRAINT_UNIQUE (2067) with this text;
	// PocketBase may additionally wrap it in a validation error.
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "2067") ||
		strings.Contains(msg, "constraint failed")
}
