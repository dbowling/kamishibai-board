// Package schema centralises the collection names, field names and API rules
// used across migrations, hooks, seeding and reporting.
//
// Referring to these constants rather than raw strings means a rename shows up
// as a compile error instead of a silent runtime miss.
package schema

// Collection names.
const (
	// Users is PocketBase's built-in auth collection, extended by our
	// migrations with a Role field.
	Users = "users"

	Teams       = "teams"
	Boards      = "boards"
	Cards       = "cards"
	Occurrences = "occurrences"
	Rollups     = "report_rollups"
)

// Roles. Anything that is not RoleAdmin is treated as a regular user, so an
// absent or unrecognised role fails closed.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// RoleValues lists the selectable roles.
func RoleValues() []string { return []string{RoleUser, RoleAdmin} }

// Field names shared by several collections.
const (
	FieldCreated = "created"
	FieldUpdated = "updated"

	// Soft delete. Records are archived, never deleted: ArchivedAt is the
	// timestamp and ArchivedBy records who did it.
	FieldArchivedAt = "archived_at"
	FieldArchivedBy = "archived_by"

	FieldCreatedBy = "created_by"
	FieldSortOrder = "sort_order"

	FieldName        = "name"
	FieldTitle       = "title"
	FieldDescription = "description"
	FieldTeam        = "team"
	FieldBoard       = "board"
	FieldCard        = "card"
	FieldCadence     = "cadence"
	FieldRole        = "role"
	FieldMembers     = "members"

	// FieldTimezone is an IANA zone name on both teams and users, with different
	// meanings. On a team it is the operational zone its periods are evaluated in
	// (empty inherits the instance default). On a user it is display-only (empty
	// means "use the browser's zone").
	FieldTimezone = "timezone"
)

// Card fields.
const (
	FieldSummary      = "summary"
	FieldInstructions = "instructions"
	FieldLinks        = "links"
	FieldChecklist    = "checklist"
)

// Occurrence fields.
const (
	FieldPeriodKey   = "period_key"
	FieldStatus      = "status"
	FieldStartedBy   = "started_by"
	FieldStartedAt   = "started_at"
	FieldCompletedBy = "completed_by"
	FieldCompletedAt = "completed_at"
	FieldNotes       = "notes"
)

// Rollup fields.
const (
	FieldPeriodStart     = "period_start"
	FieldPeriodEnd       = "period_end"
	FieldTotalCards      = "total_cards"
	FieldDoneCount       = "done_count"
	FieldInProgressCount = "in_progress_count"
	FieldNotStartedCount = "not_started_count"
	FieldCompletionRate  = "completion_rate"
	FieldClosedAt        = "closed_at"
)

// ---------------------------------------------------------------------------
// API rules
// ---------------------------------------------------------------------------
//
// A note on how access control is layered here:
//
//   - Read access is granted to members of the owning team, and to admins
//     everywhere. Admins are implicitly on every team by role, so team
//     membership is never materialised for them.
//   - Archived records stay readable so that history and reporting remain
//     intact. The UI filters them out of the active board.
//   - Delete rules are nil throughout, which in PocketBase means "superusers
//     only". That is what enforces the archive-never-delete requirement at the
//     API layer rather than trusting the client.

const (
	// Authenticated is any signed-in user.
	Authenticated = `@request.auth.id != ""`

	// AdminOnly restricts an action to users holding the admin role.
	AdminOnly = `@request.auth.id != "" && @request.auth.role = "admin"`

	// TeamMember matches members of the team the record belongs to, via a direct
	// `team` relation on the record.
	TeamMember = `@request.auth.id != "" && (@request.auth.role = "admin" || team.members.id ?= @request.auth.id)`

	// OwnTeam matches members of a `teams` record itself, where the member list
	// is a field on the record rather than on a relation.
	OwnTeam = `@request.auth.id != "" && (@request.auth.role = "admin" || members.id ?= @request.auth.id)`

	// CardTeamMember matches members of the team owning the card an occurrence
	// belongs to. Occurrences also carry a denormalised `team` relation, so
	// TeamMember would work too; this exists for rules written against the card
	// path explicitly.
	CardTeamMember = `@request.auth.id != "" && (@request.auth.role = "admin" || card.team.members.id ?= @request.auth.id)`

	// BoardTeamMember matches members of the team that owns the record's board,
	// without consulting the record's own team field.
	//
	// This is the create rule for cards. A card's team is derived from its board
	// by a hook, so the client neither sends it nor should be trusted with it, but
	// create rules are evaluated against the submitted body, before that hook has
	// run. A rule written against `team` would therefore reject every card that
	// correctly omitted it, and would only pass if the client supplied the team
	// itself, which is exactly what we do not want to rely on. Authorising from
	// the board is both safe and honest: the board is the field the client really
	// is choosing.
	BoardTeamMember = `@request.auth.id != "" && (@request.auth.role = "admin" || board.team.members.id ?= @request.auth.id)`
)

// A note on `members.id ?= ...` rather than `members ?= ...`:
//
// members is a multi-relation. Comparing the field itself against a user id
// silently matches nothing; the path has to be traversed to the related record's
// id. Both spellings parse and neither errors, so the wrong one fails closed and
// looks exactly like "this user is not on the team". The behavioural tests in
// internal/api are what catch it, which is why those assert real HTTP outcomes
// rather than just comparing rule strings.
