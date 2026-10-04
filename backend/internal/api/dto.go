package api

import (
	"github.com/pocketbase/pocketbase/core"

	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/occurrence"
	"github.com/dbowling/kamishibai/backend/internal/rollup"
	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// The response shapes are declared explicitly rather than returning raw records.
// The board view needs card definitions and per-period state stitched together,
// and hand-rolling that on the client would mean duplicating the period logic
// there.

// periodDTO describes one period window.
type periodDTO struct {
	Cadence string `json:"cadence"`
	Key     string `json:"key"`
	Start   string `json:"start"`
	End     string `json:"end"`
	Label   string `json:"label"`
}

func newPeriodDTO(p domain.Period) periodDTO {
	return periodDTO{
		Cadence: string(p.Cadence),
		Key:     p.Key,
		Start:   p.Start.Format(timeLayout),
		End:     p.End.Format(timeLayout),
		Label:   p.Cadence.Label(),
	}
}

// timeLayout is RFC3339, so the browser can parse boundaries directly and see the
// offset the server used.
const timeLayout = "2006-01-02T15:04:05Z07:00"

// userDTO is the minimal public shape of a user, used for attribution.
type userDTO struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func newUserDTO(record *core.Record) *userDTO {
	if record == nil {
		return nil
	}
	return &userDTO{
		ID:    record.Id,
		Name:  record.GetString(schema.FieldName),
		Email: record.GetString("email"),
	}
}

// stateDTO is a card's status for the period it is currently in.
type stateDTO struct {
	Status    string `json:"status"`
	PeriodKey string `json:"periodKey"`

	StartedBy   *userDTO `json:"startedBy,omitempty"`
	StartedAt   string   `json:"startedAt,omitempty"`
	CompletedBy *userDTO `json:"completedBy,omitempty"`
	CompletedAt string   `json:"completedAt,omitempty"`
	Notes       string   `json:"notes,omitempty"`
}

// linkDTO and checklistItemDTO mirror the JSON stored on a card. They are decoded
// and re-encoded rather than passed through so that malformed stored data
// surfaces as an empty list instead of breaking the client.
type linkDTO struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type checklistItemDTO struct {
	Text string `json:"text"`
}

// cardDTO is a card definition plus its current state.
type cardDTO struct {
	ID           string             `json:"id"`
	BoardID      string             `json:"boardId"`
	Title        string             `json:"title"`
	Summary      string             `json:"summary"`
	Cadence      string             `json:"cadence"`
	Instructions string             `json:"instructions"`
	Links        []linkDTO          `json:"links"`
	Checklist    []checklistItemDTO `json:"checklist"`
	SortOrder    int                `json:"sortOrder"`
	Archived     bool               `json:"archived"`
	ArchivedAt   string             `json:"archivedAt,omitempty"`
	Period       periodDTO          `json:"period"`
	State        stateDTO           `json:"state"`
}

// boardStateResponse is everything needed to draw a board in one request.
type boardStateResponse struct {
	Board    boardDTO             `json:"board"`
	Periods  map[string]periodDTO `json:"periods"`
	Cards    []cardDTO            `json:"cards"`
	Timezone string               `json:"timezone"`
	ServerAt string               `json:"serverAt"`
	Summary  boardSummaryDTO      `json:"summary"`
}

type boardDTO struct {
	ID          string `json:"id"`
	TeamID      string `json:"teamId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Archived    bool   `json:"archived"`
}

func newBoardDTO(record *core.Record) boardDTO {
	return boardDTO{
		ID:          record.Id,
		TeamID:      record.GetString(schema.FieldTeam),
		Name:        record.GetString(schema.FieldName),
		Description: record.GetString(schema.FieldDescription),
		Archived:    !record.GetDateTime(schema.FieldArchivedAt).IsZero(),
	}
}

// boardSummaryDTO is the live "where are we right now" tally. It is computed on
// the fly because the current period has not closed and so has no rollup.
type boardSummaryDTO struct {
	Total          int     `json:"total"`
	Done           int     `json:"done"`
	InProgress     int     `json:"inProgress"`
	NotStarted     int     `json:"notStarted"`
	CompletionRate float64 `json:"completionRate"`
}

// currentPeriodsResponse lets the client ask the server which period each cadence
// is in, instead of reimplementing the boundary rules in the browser.
type currentPeriodsResponse struct {
	Timezone string               `json:"timezone"`
	ServerAt string               `json:"serverAt"`
	Periods  map[string]periodDTO `json:"periods"`
}

// reportResponse carries both the frozen history and the live current period.
type reportResponse struct {
	Board    boardDTO         `json:"board"`
	Cadence  string           `json:"cadence"`
	Timezone string           `json:"timezone"`
	Series   []reportPointDTO `json:"series"`
	Current  *reportPointDTO  `json:"current,omitempty"`
	Cards    []cardReportDTO  `json:"cards"`
	Totals   reportTotalsDTO  `json:"totals"`
}

// activityResponse feeds the completions heatmap: how many cards were finished on
// each calendar day, split by cadence so one cadence can be viewed on its own.
type activityResponse struct {
	Board    boardDTO         `json:"board"`
	Timezone string           `json:"timezone"`
	Days     []activityDayDTO `json:"days"`
	Total    int              `json:"total"`
}

// activityDayDTO is one (day, cadence) bucket. Date is YYYY-MM-DD in the board's
// timezone: the browser must not re-bucket timestamps itself.
type activityDayDTO struct {
	Date      string `json:"date"`
	Cadence   string `json:"cadence"`
	Completed int    `json:"completed"`
}

// reportPointDTO is one period's numbers. Source records whether the figures were
// read from a frozen snapshot or computed live, which matters because only the
// former is immutable.
type reportPointDTO struct {
	PeriodKey      string  `json:"periodKey"`
	PeriodStart    string  `json:"periodStart"`
	PeriodEnd      string  `json:"periodEnd"`
	Total          int     `json:"total"`
	Done           int     `json:"done"`
	InProgress     int     `json:"inProgress"`
	NotStarted     int     `json:"notStarted"`
	CompletionRate float64 `json:"completionRate"`
	Source         string  `json:"source"`
}

const (
	sourceRollup = "rollup"
	sourceLive   = "live"
)

func newReportPointFromSnapshot(snap rollup.Snapshot, source string) reportPointDTO {
	return reportPointDTO{
		PeriodKey:      snap.Period.Key,
		PeriodStart:    snap.Period.Start.Format(timeLayout),
		PeriodEnd:      snap.Period.End.Format(timeLayout),
		Total:          snap.TotalCards,
		Done:           snap.Done,
		InProgress:     snap.InProgress,
		NotStarted:     snap.NotStarted,
		CompletionRate: snap.CompletionRate,
		Source:         source,
	}
}

// cardReportDTO answers "what is the completion rate for this particular task".
type cardReportDTO struct {
	CardID         string  `json:"cardId"`
	Title          string  `json:"title"`
	Cadence        string  `json:"cadence"`
	Archived       bool    `json:"archived"`
	Expected       int     `json:"expected"`
	Done           int     `json:"done"`
	CompletionRate float64 `json:"completionRate"`
}

// reportTotalsDTO aggregates the whole requested window.
type reportTotalsDTO struct {
	Periods        int     `json:"periods"`
	Expected       int     `json:"expected"`
	Done           int     `json:"done"`
	CompletionRate float64 `json:"completionRate"`
}

// mutationResponse is returned by start/complete/reopen so the client can update
// a card without refetching the whole board.
type mutationResponse struct {
	CardID string    `json:"cardId"`
	Period periodDTO `json:"period"`
	State  stateDTO  `json:"state"`
}

// newStateDTO renders an occurrence state, resolving attribution through the
// supplied user lookup.
func newStateDTO(state occurrence.State, users map[string]*core.Record) stateDTO {
	dto := stateDTO{
		Status:    string(state.Status),
		PeriodKey: state.Period.Key,
	}

	occ := state.Occurrence
	if occ == nil {
		return dto
	}

	dto.Notes = occ.GetString(schema.FieldNotes)

	if id := occ.GetString(schema.FieldStartedBy); id != "" {
		dto.StartedBy = newUserDTO(users[id])
	}
	if ts := occ.GetDateTime(schema.FieldStartedAt); !ts.IsZero() {
		dto.StartedAt = ts.Time().Format(timeLayout)
	}
	if id := occ.GetString(schema.FieldCompletedBy); id != "" {
		dto.CompletedBy = newUserDTO(users[id])
	}
	if ts := occ.GetDateTime(schema.FieldCompletedAt); !ts.IsZero() {
		dto.CompletedAt = ts.Time().Format(timeLayout)
	}

	return dto
}
