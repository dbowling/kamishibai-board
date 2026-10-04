// Package api exposes the custom HTTP endpoints.
//
// Ordinary reads and writes of teams, boards and cards go through PocketBase's
// generated collection API, guarded by the rules in the migrations. This package
// covers the five things that API cannot do safely or efficiently:
//
//  1. Recording work. start/complete/reopen must derive the period from the
//     server clock and stamp attribution from the authenticated request, so
//     occurrences are closed to direct client writes entirely.
//
//  2. Reading a board. Drawing a board means combining card definitions with the
//     occurrence rows for whichever periods those cards are currently in. Doing
//     that client-side would mean reimplementing the period rules in the browser
//     and issuing a request per card.
//
//  3. Reporting. Stitching frozen rollups together with the still-open current
//     period, and deriving per-card rates.
//
//  4. Activity. Counting completions per calendar day in the board's timezone,
//     which SQLite cannot do and the browser must not.
//
//  5. Sidebar structure. Reordering teams and boards touches many rows that must
//     change together, and moving a board between teams has to carry its cards,
//     occurrences and rollups with it. Both are admin-only and transactional, so
//     they cannot be assembled safely from individual collection writes. The
//     logic lives in internal/navigation; the handlers here only translate.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/dbowling/kamishibai/backend/internal/access"
	"github.com/dbowling/kamishibai/backend/internal/config"
	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/navigation"
	"github.com/dbowling/kamishibai/backend/internal/occurrence"
	"github.com/dbowling/kamishibai/backend/internal/rollup"
	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// BasePath prefixes every custom route.
const BasePath = "/api/kamishibai"

// Handler owns the custom endpoints.
type Handler struct {
	cfg        config.Config
	occurrence *occurrence.Service
	rollup     *rollup.Service
}

// NewHandler builds a Handler from the application configuration.
func NewHandler(cfg config.Config) *Handler {
	return &Handler{
		cfg:        cfg,
		occurrence: occurrence.NewService(cfg.Calendar),
		rollup:     rollup.NewService(cfg.Calendar),
	}
}

// Register mounts the routes on the serve event.
func (h *Handler) Register(se *core.ServeEvent) {
	// Every route requires an authenticated user of the `users` collection.
	// Attribution is meaningless without one.
	group := se.Router.Group(BasePath).Bind(apis.RequireAuth(schema.Users))

	group.GET("/periods/current", h.currentPeriods)
	group.GET("/boards/{boardId}/state", h.boardState)
	group.GET("/boards/{boardId}/report", h.boardReport)
	group.GET("/boards/{boardId}/activity", h.boardActivity)

	group.POST("/cards/{cardId}/start", h.startCard)
	group.POST("/cards/{cardId}/complete", h.completeCard)
	group.POST("/cards/{cardId}/reopen", h.reopenCard)

	// Admin-only. The checks live in internal/navigation, not in the handlers.
	group.POST("/navigation/order", h.setNavigationOrder)
	group.POST("/boards/{boardId}/move", h.moveBoard)
}

// ---------------------------------------------------------------------------
// Periods
// ---------------------------------------------------------------------------

// currentPeriods reports which period each cadence is currently in.
//
// The server is the single source of truth for period keys. The frontend renders
// what it is told rather than recomputing ISO weeks and daylight-saving
// boundaries in the browser, where it could disagree with the backend.
func (h *Handler) currentPeriods(e *core.RequestEvent) error {
	periods, err := h.periodMap()
	if err != nil {
		return e.InternalServerError("Could not resolve the current periods.", err)
	}

	return e.JSON(http.StatusOK, currentPeriodsResponse{
		Timezone: h.cfg.Timezone,
		ServerAt: h.occurrence.Now().Format(timeLayout),
		Periods:  periods,
	})
}

func (h *Handler) periodMap() (map[string]periodDTO, error) {
	now := h.occurrence.Now()
	out := make(map[string]periodDTO, len(domain.Cadences()))

	for _, cadence := range domain.Cadences() {
		p, err := h.cfg.Calendar.At(cadence, now)
		if err != nil {
			return nil, err
		}
		out[string(cadence)] = newPeriodDTO(p)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Board state
// ---------------------------------------------------------------------------

// boardState returns a board, its cards and each card's current status.
func (h *Handler) boardState(e *core.RequestEvent) error {
	board, err := h.loadReadableBoard(e)
	if err != nil {
		return err
	}

	includeArchived := e.Request.URL.Query().Get("includeArchived") == "true"

	cards, err := h.loadBoardCards(e.App, board.Id, includeArchived)
	if err != nil {
		return e.InternalServerError("Could not load the cards for this board.", err)
	}

	// One query for the whole board, regardless of card count.
	states, err := h.occurrence.StatesFor(e.App, cards)
	if err != nil {
		return e.InternalServerError("Could not resolve card state.", err)
	}

	users, err := h.resolveUsers(e.App, states)
	if err != nil {
		return e.InternalServerError("Could not resolve attribution.", err)
	}

	periods, err := h.periodMap()
	if err != nil {
		return e.InternalServerError("Could not resolve the current periods.", err)
	}

	response := boardStateResponse{
		Board:    newBoardDTO(board),
		Periods:  periods,
		Cards:    make([]cardDTO, 0, len(cards)),
		Timezone: h.cfg.Timezone,
		ServerAt: h.occurrence.Now().Format(timeLayout),
	}

	for _, card := range cards {
		state := states[card.Id]
		dto := cardDTO{
			ID:           card.Id,
			BoardID:      card.GetString(schema.FieldBoard),
			Title:        card.GetString(schema.FieldTitle),
			Summary:      card.GetString(schema.FieldSummary),
			Cadence:      card.GetString(schema.FieldCadence),
			Instructions: card.GetString(schema.FieldInstructions),
			Links:        decodeLinks(card),
			Checklist:    decodeChecklist(card),
			SortOrder:    card.GetInt(schema.FieldSortOrder),
			Archived:     access.IsArchived(card),
			Period:       newPeriodDTO(state.Period),
			State:        newStateDTO(state, users),
		}
		if ts := card.GetDateTime(schema.FieldArchivedAt); !ts.IsZero() {
			dto.ArchivedAt = ts.Time().Format(timeLayout)
		}

		response.Cards = append(response.Cards, dto)

		// The live tally only counts active cards: an archived card is no longer
		// expected work.
		if !dto.Archived {
			response.Summary.Total++
			switch state.Status {
			case domain.StatusDone:
				response.Summary.Done++
			case domain.StatusInProgress:
				response.Summary.InProgress++
			default:
				response.Summary.NotStarted++
			}
		}
	}

	if response.Summary.Total > 0 {
		response.Summary.CompletionRate = float64(response.Summary.Done) / float64(response.Summary.Total)
	}

	return e.JSON(http.StatusOK, response)
}

// loadBoardCards fetches a board's cards in display order.
func (h *Handler) loadBoardCards(app core.App, boardID string, includeArchived bool) ([]*core.Record, error) {
	exprs := []dbx.Expression{dbx.HashExp{schema.FieldBoard: boardID}}
	if !includeArchived {
		exprs = append(exprs, dbx.NewExp(
			schema.FieldArchivedAt+" = '' OR "+schema.FieldArchivedAt+" IS NULL"))
	}

	var cards []*core.Record
	query := app.RecordQuery(schema.Cards)
	for _, expr := range exprs {
		query = query.AndWhere(expr)
	}
	if err := query.OrderBy(schema.FieldSortOrder+" ASC", schema.FieldCreated+" ASC").All(&cards); err != nil {
		return nil, err
	}
	return cards, nil
}

// resolveUsers batch-loads every user referenced by the given states.
//
// Attribution names are needed for the board, and fetching them one at a time
// would undo the single-query win from StatesFor.
func (h *Handler) resolveUsers(app core.App, states map[string]occurrence.State) (map[string]*core.Record, error) {
	ids := make(map[string]struct{})
	for _, state := range states {
		if state.Occurrence == nil {
			continue
		}
		for _, field := range []string{schema.FieldStartedBy, schema.FieldCompletedBy} {
			if id := state.Occurrence.GetString(field); id != "" {
				ids[id] = struct{}{}
			}
		}
	}
	return h.loadUsers(app, ids)
}

func (h *Handler) loadUsers(app core.App, ids map[string]struct{}) (map[string]*core.Record, error) {
	out := make(map[string]*core.Record, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	list := make([]string, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}

	records, err := app.FindRecordsByIds(schema.Users, list)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		out[record.Id] = record
	}
	return out, nil
}

// decodeLinks reads the links JSON off a card, tolerating malformed data.
func decodeLinks(card *core.Record) []linkDTO {
	out := []linkDTO{}
	raw := card.Get(schema.FieldLinks)
	if raw == nil {
		return out
	}
	if err := card.UnmarshalJSONField(schema.FieldLinks, &out); err != nil {
		return []linkDTO{}
	}
	if out == nil {
		return []linkDTO{}
	}
	return out
}

// decodeChecklist reads the checklist JSON off a card.
//
// It accepts either a list of objects or a bare list of strings, because a plain
// string list is the obvious thing to write by hand when seeding or scripting.
func decodeChecklist(card *core.Record) []checklistItemDTO {
	out := []checklistItemDTO{}

	if err := card.UnmarshalJSONField(schema.FieldChecklist, &out); err == nil && out != nil {
		return out
	}

	var plain []string
	if err := card.UnmarshalJSONField(schema.FieldChecklist, &plain); err == nil {
		converted := make([]checklistItemDTO, 0, len(plain))
		for _, text := range plain {
			converted = append(converted, checklistItemDTO{Text: text})
		}
		return converted
	}

	return []checklistItemDTO{}
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

// mutationRequest is the optional body accepted by start/complete/reopen.
//
// Note what is absent: no period key, no status, no user id. All three are
// determined by the server, which is what makes the audit trail trustworthy.
type mutationRequest struct {
	Notes string `json:"notes"`
}

func (h *Handler) startCard(e *core.RequestEvent) error {
	return h.mutate(e, h.occurrence.Start)
}

func (h *Handler) completeCard(e *core.RequestEvent) error {
	return h.mutate(e, h.occurrence.Complete)
}

func (h *Handler) reopenCard(e *core.RequestEvent) error {
	return h.mutate(e, h.occurrence.Reopen)
}

type mutateFunc func(app core.App, cardID string, user *core.Record, notes string) (*core.Record, error)

func (h *Handler) mutate(e *core.RequestEvent, fn mutateFunc) error {
	cardID := e.Request.PathValue("cardId")
	if cardID == "" {
		return e.BadRequestError("Missing card id.", nil)
	}

	var body mutationRequest
	// An empty body is normal: notes are optional.
	if e.Request.Body != nil {
		_ = json.NewDecoder(e.Request.Body).Decode(&body)
	}

	if _, err := fn(e.App, cardID, e.Auth, body.Notes); err != nil {
		return mapServiceError(e, err)
	}

	card, err := e.App.FindRecordById(schema.Cards, cardID)
	if err != nil {
		return e.InternalServerError("Could not reload the card.", err)
	}

	state, err := h.occurrence.StateOf(e.App, card)
	if err != nil {
		return e.InternalServerError("Could not resolve card state.", err)
	}

	users, err := h.resolveUsers(e.App, map[string]occurrence.State{card.Id: state})
	if err != nil {
		return e.InternalServerError("Could not resolve attribution.", err)
	}

	return e.JSON(http.StatusOK, mutationResponse{
		CardID: card.Id,
		Period: newPeriodDTO(state.Period),
		State:  newStateDTO(state, users),
	})
}

// mapServiceError translates the occurrence service's sentinel errors into HTTP
// responses, so the client gets a useful status code and message.
func mapServiceError(e *core.RequestEvent, err error) error {
	switch {
	case errors.Is(err, occurrence.ErrCardNotFound):
		return e.NotFoundError("No such card.", err)
	case errors.Is(err, occurrence.ErrForbidden):
		return e.ForbiddenError("You are not a member of this card's team.", err)
	case errors.Is(err, occurrence.ErrArchived):
		return e.BadRequestError("This card is archived and read-only.", err)
	case errors.Is(err, occurrence.ErrAlreadyComplete):
		return e.BadRequestError("This card is already complete for the current period. Reopen it first.", err)
	case errors.Is(err, occurrence.ErrNothingToReopen):
		return e.BadRequestError("There is nothing recorded for the current period.", err)
	default:
		return e.InternalServerError("Could not record the change.", err)
	}
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------

// setNavigationOrder persists a drag-and-drop reordering of teams and boards.
func (h *Handler) setNavigationOrder(e *core.RequestEvent) error {
	var body navigation.Order
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("The request body must be valid JSON.", err)
	}

	if err := navigation.SetOrder(e.App, e.Auth, body); err != nil {
		return mapNavigationError(e, err)
	}
	return e.NoContent(http.StatusNoContent)
}

// moveBoard re-homes a board, with its history, onto another team.
func (h *Handler) moveBoard(e *core.RequestEvent) error {
	boardID := e.Request.PathValue("boardId")
	if boardID == "" {
		return e.BadRequestError("Missing board id.", nil)
	}

	var body moveBoardRequest
	// A missing or malformed body is reported by the service as a missing target
	// team, after the admin check, so it cannot be used to probe as a non-admin.
	if e.Request.Body != nil {
		_ = json.NewDecoder(e.Request.Body).Decode(&body)
	}

	result, err := navigation.MoveBoard(e.App, e.Auth, boardID, body.Team)
	if err != nil {
		return mapNavigationError(e, err)
	}

	return e.JSON(http.StatusOK, moveBoardResponse{
		BoardID: result.BoardID,
		TeamID:  result.TeamID,
		Moved: movedCountsDTO{
			Cards:       result.Cards,
			Occurrences: result.Occurrences,
			Rollups:     result.Rollups,
		},
	})
}

// mapNavigationError translates the navigation service's errors into responses.
func mapNavigationError(e *core.RequestEvent, err error) error {
	var invalid *navigation.InvalidError
	switch {
	case errors.Is(err, navigation.ErrForbidden):
		return e.ForbiddenError("Only an admin can change the navigation.", err)
	case errors.Is(err, navigation.ErrBoardNotFound):
		return e.NotFoundError("No such board.", err)
	case errors.As(err, &invalid):
		return e.BadRequestError(invalid.Message, err)
	default:
		return e.InternalServerError("Could not update the navigation.", err)
	}
}

// ---------------------------------------------------------------------------
// Reporting
// ---------------------------------------------------------------------------

// defaultReportPeriods is how many closed periods a report covers by default.
const (
	defaultReportPeriods = 12
	maxReportPeriods     = 120
)

// boardReport returns the completion history for one board and cadence.
//
// Closed periods come from the frozen rollups. The period still in progress has
// no snapshot by definition, so it is computed live and tagged as such.
func (h *Handler) boardReport(e *core.RequestEvent) error {
	board, err := h.loadReadableBoard(e)
	if err != nil {
		return err
	}

	query := e.Request.URL.Query()

	cadence, err := domain.ParseCadence(query.Get("cadence"))
	if err != nil {
		return e.BadRequestError("Provide a valid cadence: daily, weekly, monthly, quarterly or annual.", err)
	}

	periods := defaultReportPeriods
	if raw := query.Get("periods"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return e.BadRequestError("The periods parameter must be a positive number.", err)
		}
		periods = min(n, maxReportPeriods)
	}

	now := h.occurrence.Now()

	closed, err := h.cfg.Calendar.ClosedBefore(cadence, now, periods)
	if err != nil {
		return e.InternalServerError("Could not resolve the reporting window.", err)
	}

	response := reportResponse{
		Board:    newBoardDTO(board),
		Cadence:  string(cadence),
		Timezone: h.cfg.Timezone,
		Series:   make([]reportPointDTO, 0, len(closed)+1),
	}

	// Closed periods, oldest first for charting.
	for i := len(closed) - 1; i >= 0; i-- {
		period := closed[i]

		stored, err := h.rollup.Find(e.App, board.Id, cadence, period.Key)
		if err != nil {
			return e.InternalServerError("Could not read the reporting history.", err)
		}

		if stored != nil {
			response.Series = append(response.Series, reportPointDTO{
				PeriodKey:      period.Key,
				PeriodStart:    period.Start.Format(timeLayout),
				PeriodEnd:      period.End.Format(timeLayout),
				Total:          stored.GetInt(schema.FieldTotalCards),
				Done:           stored.GetInt(schema.FieldDoneCount),
				InProgress:     stored.GetInt(schema.FieldInProgressCount),
				NotStarted:     stored.GetInt(schema.FieldNotStartedCount),
				CompletionRate: stored.GetFloat(schema.FieldCompletionRate),
				Source:         sourceRollup,
			})
			continue
		}

		// No snapshot yet, most likely because the rollup job has not caught up.
		// Compute it live so the report is never misleadingly empty, and label the
		// source so the client can tell the difference.
		snap, err := h.rollup.Compute(e.App, board, cadence, period)
		if err != nil {
			return e.InternalServerError("Could not compute the reporting window.", err)
		}
		if snap.Empty() {
			continue
		}
		response.Series = append(response.Series, newReportPointFromSnapshot(snap, sourceLive))
	}

	// The period in progress.
	currentPeriod, err := h.cfg.Calendar.At(cadence, now)
	if err != nil {
		return e.InternalServerError("Could not resolve the current period.", err)
	}
	currentSnap, err := h.rollup.Compute(e.App, board, cadence, currentPeriod)
	if err != nil {
		return e.InternalServerError("Could not compute the current period.", err)
	}
	if !currentSnap.Empty() {
		point := newReportPointFromSnapshot(currentSnap, sourceLive)
		response.Current = &point
	}

	// Per-card rates over the same window, including the open period so the
	// numbers agree with what the board is showing.
	window := append([]domain.Period{}, closed...)
	window = append(window, currentPeriod)

	response.Cards, err = h.cardReports(e.App, board, cadence, window)
	if err != nil {
		return e.InternalServerError("Could not compute per-card statistics.", err)
	}

	response.Totals = reportTotalsDTO{Periods: len(window)}
	for _, card := range response.Cards {
		response.Totals.Expected += card.Expected
		response.Totals.Done += card.Done
	}
	if response.Totals.Expected > 0 {
		response.Totals.CompletionRate = float64(response.Totals.Done) / float64(response.Totals.Expected)
	}

	return e.JSON(http.StatusOK, response)
}

// dayLayout is the date-only form of an activity bucket, e.g. 2026-09-03.
const dayLayout = "2006-01-02"

// boardActivity returns how many cards were completed on each calendar day.
//
// Unlike boardReport this reads the occurrences themselves rather than rollups:
// a rollup is a per-period snapshot, and a heatmap needs the day each completion
// actually happened, including inside periods that are still open.
func (h *Handler) boardActivity(e *core.RequestEvent) error {
	board, err := h.loadReadableBoard(e)
	if err != nil {
		return err
	}

	// Two columns rather than whole records: occurrences grow without bound, but
	// the aggregate is at most a few rows per day, so there is no reason to
	// hydrate every row just to read a timestamp and a cadence.
	//
	// Filtering on status as well as completed_at matters. Reopen clears
	// completed_at today, but status is the source of truth for "done", so a
	// reopened card can never be counted even if that ever stopped being true.
	var rows []struct {
		CompletedAt types.DateTime `db:"completed_at"`
		Cadence     string         `db:"cadence"`
	}
	err = e.App.DB().
		Select(schema.FieldCompletedAt, schema.FieldCadence).
		From(schema.Occurrences).
		Where(dbx.HashExp{
			schema.FieldBoard:  board.Id,
			schema.FieldStatus: string(domain.StatusDone),
		}).
		AndWhere(dbx.NewExp(schema.FieldCompletedAt + " != ''")).
		All(&rows)
	if err != nil {
		return e.InternalServerError("Could not read the completion history.", err)
	}

	// Bucketed here rather than in SQL: SQLite has no timezone database, and the
	// server already owns the calendar, so the day boundaries (and DST) agree with
	// every other period the app reports.
	loc := h.cfg.Calendar.Location()
	type bucket struct{ date, cadence string }
	counts := make(map[bucket]int)
	for _, row := range rows {
		if row.CompletedAt.IsZero() {
			continue
		}
		key := bucket{row.CompletedAt.Time().In(loc).Format(dayLayout), row.Cadence}
		counts[key]++
	}

	// An empty, non-nil slice so a quiet board serialises as [] rather than null,
	// which the client would otherwise have to special-case.
	response := activityResponse{
		Board:    newBoardDTO(board),
		Timezone: h.cfg.Timezone,
		Days:     []activityDayDTO{},
	}
	for key, completed := range counts {
		response.Days = append(response.Days, activityDayDTO{
			Date:      key.date,
			Cadence:   key.cadence,
			Completed: completed,
		})
		response.Total += completed
	}

	// Map order is random, so sort for a stable response: by day, then in the
	// same cadence order the rest of the API uses.
	order := make(map[string]int, len(domain.Cadences()))
	for i, cadence := range domain.Cadences() {
		order[string(cadence)] = i
	}
	sort.Slice(response.Days, func(i, j int) bool {
		a, b := response.Days[i], response.Days[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		return order[a.Cadence] < order[b.Cadence]
	})

	return e.JSON(http.StatusOK, response)
}

// cardReports computes the per-card completion rate across a set of periods.
//
// One query fetches every completed occurrence in the window; the expected count
// is derived in Go from when each card existed. Doing it per card would mean two
// queries per card per report.
func (h *Handler) cardReports(app core.App, board *core.Record, cadence domain.Cadence, window []domain.Period) ([]cardReportDTO, error) {
	var cards []*core.Record
	err := app.RecordQuery(schema.Cards).
		AndWhere(dbx.HashExp{
			schema.FieldBoard:   board.Id,
			schema.FieldCadence: string(cadence),
		}).
		OrderBy(schema.FieldSortOrder+" ASC", schema.FieldCreated+" ASC").
		All(&cards)
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return []cardReportDTO{}, nil
	}

	keys := make([]any, 0, len(window))
	for _, p := range window {
		keys = append(keys, p.Key)
	}
	cardIDs := make([]any, 0, len(cards))
	for _, c := range cards {
		cardIDs = append(cardIDs, c.Id)
	}

	var done []*core.Record
	err = app.RecordQuery(schema.Occurrences).
		AndWhere(dbx.In(schema.FieldCard, cardIDs...)).
		AndWhere(dbx.In(schema.FieldPeriodKey, keys...)).
		AndWhere(dbx.HashExp{schema.FieldStatus: string(domain.StatusDone)}).
		All(&done)
	if err != nil {
		return nil, err
	}

	doneByCard := make(map[string]int, len(cards))
	for _, occ := range done {
		doneByCard[occ.GetString(schema.FieldCard)]++
	}

	out := make([]cardReportDTO, 0, len(cards))
	for _, card := range cards {
		created := card.GetDateTime(schema.FieldCreated).Time()
		archivedAt := card.GetDateTime(schema.FieldArchivedAt)

		// A card is only accountable for periods it actually existed during.
		expected := 0
		for _, p := range window {
			if !created.Before(p.End) {
				continue // created after the period ended
			}
			if !archivedAt.IsZero() && !archivedAt.Time().After(p.Start) {
				continue // archived before the period began
			}
			expected++
		}

		report := cardReportDTO{
			CardID:   card.Id,
			Title:    card.GetString(schema.FieldTitle),
			Cadence:  card.GetString(schema.FieldCadence),
			Archived: access.IsArchived(card),
			Expected: expected,
			Done:     doneByCard[card.Id],
		}
		if expected > 0 {
			report.CompletionRate = float64(report.Done) / float64(expected)
		}
		out = append(out, report)
	}

	// Worst performers first: the point of the report is to find what is being
	// skipped.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CompletionRate < out[j].CompletionRate
	})

	return out, nil
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// loadReadableBoard resolves the {boardId} path parameter and checks the caller
// may read it.
//
// Archived boards are readable so that history and reporting survive archival.
func (h *Handler) loadReadableBoard(e *core.RequestEvent) (*core.Record, error) {
	boardID := e.Request.PathValue("boardId")
	if boardID == "" {
		return nil, e.BadRequestError("Missing board id.", nil)
	}

	board, err := e.App.FindRecordById(schema.Boards, boardID)
	if err != nil {
		return nil, e.NotFoundError("No such board.", err)
	}

	team, err := access.LoadTeam(e.App, board)
	if err != nil {
		return nil, e.InternalServerError("Could not resolve the board's team.", err)
	}
	if !access.CanReadTeam(team, e.Auth) {
		// Deliberately 404 rather than 403: revealing that a board exists on
		// another team is itself a small leak.
		return nil, e.NotFoundError("No such board.", nil)
	}

	return board, nil
}
