# Managing the sidebar

The left-hand sidebar lists teams and, under each, its boards. Administrators can
change all of it from the app: create, rename, archive and restore teams and
boards, reorder them, and move a board to another team. Everyone else sees the
read-only list.

## For administrators

Sign in as an admin and click **Edit** at the top of the sidebar. The list turns
into an editor; click **Done** to go back to the normal links.

### Reordering

Every team and board has a drag handle (the dotted grip on its left).

- **Teams** reorder among themselves.
- **Boards** reorder within their team.
- Drag a board onto another team's boards (or onto an empty team's "Drag one here"
  box) to move it. See [Moving a board](#moving-a-board).

With the keyboard: focus a handle, press Space to pick it up, use the arrow keys,
and press Space to drop (Escape cancels). The change is saved straight away. If the
server refuses it, the list snaps back and a message says why.

### Teams and boards

| To do this | Use |
| --- | --- |
| Add a team | **+ Add team** at the bottom |
| Rename or re-describe a team | the pencil next to its name |
| Archive a team | the archive icon next to its name (asks first) |
| Add a board | **+ Add board** under the team |
| Edit a board's name, description or team | the pencil next to the board |
| Archive a board | the archive icon next to the board (asks first) |

New teams and boards appear last. Boards must have a name that is unique within
their team.

### Archiving and restoring

Archiving hides a team or board from everyone's sidebar. Nothing is deleted: a
board keeps its cards and history. Archiving a team hides its boards too, and they
come back with it.

Open **Archived** at the bottom of the editor to see archived teams and boards,
and press **Restore** to bring one back. Boards of an archived team are not listed
separately; restore the team.

If you archive the board you are looking at, the app takes you to the first
remaining board.

### Moving a board

A board can be moved to another team by dragging it, or by changing **Team** in the
board's edit dialog (the keyboard-friendly way). You are always asked to confirm:

- the board's cards, completion history and reports move with it, and
- members of the old team lose access to the board unless they are also on the
  new team.

If the two teams use different time zones (a team with none uses the instance
default), the confirmation also warns that the board's periods will roll over in the
new team's zone from then on. Past records and reports keep the boundaries they were
recorded with, so reports show a seam at the move.

A move is refused if the board is archived, either team is archived, or the new
team already has an active board with the same name.

## For developers

### Components (`frontend/src/nav/`)

| File | Role |
| --- | --- |
| `Sidebar.tsx` | The read-only list, plus the admin-only Edit toggle. Swaps in the editor |
| `SidebarEditor.tsx` | Edit mode: dnd-kit drag and drop, optimistic updates with rollback, owns the dialogs and the Archived section |
| `order.ts` | Pure logic: turn a finished drag into a new layout, detect a cross-team move, build `saveOrder` payloads |
| `TeamDialog.tsx`, `BoardDialog.tsx` | Create / edit forms. Changing a board's team calls `onMoveRequested` instead of saving. The team form carries the team's time zone (empty inherits the instance default) and warns about a report seam when an existing team's zone is changed |
| `ConfirmDialog.tsx`, `Modal.tsx` | Shared confirm box and modal shell (Escape, labelled, focus returned to the opener) |

`useBoards(includeArchived)` loads the data. Only an admin in edit mode passes
`true`, which also returns archived teams and boards; for everyone else the
requests are unchanged.

Drag and drop uses `@dnd-kit/core` and `@dnd-kit/sortable`, with a pointer sensor
and a keyboard sensor (`sortableKeyboardCoordinates`). Teams and boards share one
id space in dnd-kit, so ids are prefixed (`team:`, `board:`, `drop:`; see
`order.ts`). Collision detection is filtered so teams only sort among teams and
boards among boards and empty-team drop zones.

### API methods (`src/lib/api.ts`)

`teams()` (sorted by `sort_order,name`), `boards(teamId, {includeArchived})`,
`createTeam`, `updateTeam`, `archiveTeam`, `restoreTeam`, `createBoard` (takes a
`sortOrder`), `updateBoard`, `archiveBoard`, `restoreBoard`, `moveBoard`,
`saveOrder`. The collection calls use PocketBase's generated API; archive writes
`archived_at` and restore clears it, as for cards.

### The two endpoints

- `POST /api/kamishibai/navigation/order` with
  `{ "teams": [ids], "boards": { "<teamId>": [ids] } }` sets `sort_order` to each
  id's index. Both keys are optional. Every board must already belong to the team
  it is listed under, so this endpoint never moves a board. 204 on success.
- `POST /api/kamishibai/boards/{id}/move` with `{ "team": "<teamId>" }` moves the
  board. It answers `{ boardId, teamId, moved: { cards, occurrences, rollups } }`.

After a drag across teams the editor calls `moveBoard`, then `saveOrder` for the
target team to place the board where it was dropped (the endpoint puts it last).

### Why a move is an endpoint

Cards, occurrences and rollups each store their own copy of the team id, which is
what the access rules check. Changing only the board's `team` would leave its
history visible to the old team. The endpoint re-points all of them in one
transaction, and the generic update on boards and cards still refuses a team
change.

### Permissions

| Action | Who |
| --- | --- |
| See the sidebar | Members of a team (admins see all teams) |
| Create / update / archive / restore teams and boards | Admins only, enforced by the API rules |
| Reorder, move | Admins only (403 otherwise) |

The Edit button is hidden for non-admins, but that is a convenience; the server is
what refuses. See [Permissions](permissions.md).

### Tests

`order.test.ts` covers the drag logic and `dialogs.test.tsx` the dialogs. Stories
in `src/nav/` and `src/App.stories.tsx` cover edit mode end to end, including a
keyboard reorder, with the in-memory backend in `src/stories/fakeBackend.ts`
mirroring the server's rules for moves and ordering.
