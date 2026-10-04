import { useEffect, useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  pointerWithin,
  useDroppable,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import type {
  Announcements,
  CollisionDetection,
  DragEndEvent,
  DragStartEvent,
} from '@dnd-kit/core';
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import type { ArchivedNav } from '../boards/useBoards';
import { api, errorMessage } from '../lib/api';
import type { BoardRecord, TeamRecord } from '../lib/types';
import { BoardDialog } from './BoardDialog';
import type { MoveRequest } from './BoardDialog';
import { ConfirmDialog } from './ConfirmDialog';
import { TeamDialog } from './TeamDialog';
import {
  boardItemId,
  boardOrderPayload,
  dropBoard,
  dropItemId,
  moveBoardToEnd,
  nextSortOrder,
  parseItemId,
  reorderTeams,
  teamItemId,
  teamOrderPayload,
} from './order';
import type { Layout } from './order';

interface SidebarEditorProps {
  teams: Layout;
  archived: ArchivedNav;
  /** Called after anything changed on the server, so the sidebar data is refetched. */
  onChanged: () => void;
}

/** A move waiting for the admin's confirmation. */
interface PendingMove {
  boardId: string;
  boardName: string;
  fromTeamId: string;
  toTeamId: string;
  /** The layout to show (and persist) once confirmed. */
  layout: Layout;
  edits: MoveRequest['edits'];
}

type Dialog =
  | { kind: 'newTeam' }
  | { kind: 'editTeam'; team: TeamRecord }
  | { kind: 'newBoard'; team: TeamRecord }
  | { kind: 'editBoard'; board: BoardRecord }
  | { kind: 'archiveTeam'; team: TeamRecord }
  | { kind: 'archiveBoard'; board: BoardRecord }
  | { kind: 'move'; move: PendingMove };

/**
 * The sidebar in edit mode: reorder by drag and drop, move boards between teams,
 * and create, rename, archive and restore teams and boards. Admin only.
 *
 * It works on a local copy of the layout so a drop takes effect immediately; if
 * the server refuses, the copy is rolled back and the reason is shown. Dragging
 * works with a pointer or the keyboard (focus a handle, press Space, use the
 * arrow keys, press Space again). The board dialog's Team select is the other
 * keyboard-friendly way to move a board.
 */
export function SidebarEditor({ teams, archived, onChanged }: SidebarEditorProps) {
  const [layout, setLayout] = useState<Layout>(teams);
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Server data wins whenever it arrives: after every refresh, and after a
  // rollback has been re-fetched.
  useEffect(() => setLayout(teams), [teams]);

  const sensors = useSensors(
    // A small distance so clicking a handle is not a zero-length drag.
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  const closeDialog = () => setDialog(null);

  /** Runs a server change, then refreshes. Errors are shown rather than thrown. */
  async function run(action: () => Promise<unknown>, fallback: string) {
    setError(null);
    setBusy(true);
    try {
      await action();
      setDialog(null);
      onChanged();
    } catch (cause) {
      setDialog(null);
      setError(errorMessage(cause, fallback));
    } finally {
      setBusy(false);
    }
  }

  /** Shows `next` straight away and saves it; puts `layout` back if that fails. */
  async function applyOrder(next: Layout, save: () => Promise<unknown>) {
    const previous = layout;
    setError(null);
    setLayout(next);
    try {
      await save();
      onChanged();
    } catch (cause) {
      setLayout(previous);
      setError(errorMessage(cause, 'Could not save the new order.'));
    }
  }

  function onDragEnd(event: DragEndEvent) {
    const active = parseItemId(event.active.id);
    const over = event.over ? parseItemId(event.over.id) : null;
    if (!active || !over) return;

    if (active.kind === 'team') {
      if (over.kind !== 'team') return;
      const next = reorderTeams(layout, active.id, over.id);
      if (next) void applyOrder(next, () => api.saveOrder(teamOrderPayload(next)));
      return;
    }

    if (active.kind !== 'board') return;

    // When changing team, released on the lower half of a board means "after it".
    const dragged = event.active.rect.current.translated;
    const target = event.over?.rect;
    const after =
      dragged !== null && target !== undefined
        ? dragged.top + dragged.height / 2 > target.top + target.height / 2
        : false;

    const result = dropBoard(
      layout,
      active.id,
      over.kind === 'board' ? { kind: 'board', id: over.id, after } : { kind: 'team', id: over.id },
    );
    if (!result) return;

    if (result.move) {
      requestMove(result.move.boardId, result.move.fromTeamId, result.move.toTeamId, result.layout, null);
    } else {
      const next = result.layout;
      const teamId = layout.find((entry) => entry.boards.some((b) => b.id === active.id))?.team.id;
      if (teamId) void applyOrder(next, () => api.saveOrder(boardOrderPayload(next, [teamId])));
    }
  }

  function requestMove(
    boardId: string,
    fromTeamId: string,
    toTeamId: string,
    nextLayout: Layout,
    edits: MoveRequest['edits'],
  ) {
    const board = layout.flatMap((entry) => entry.boards).find((b) => b.id === boardId);
    if (!board) return;
    setDialog({
      kind: 'move',
      move: {
        boardId,
        boardName: edits?.name ?? board.name,
        fromTeamId,
        toTeamId,
        layout: nextLayout,
        edits,
      },
    });
  }

  function onMoveRequested({ board, toTeamId, edits }: MoveRequest) {
    const result = moveBoardToEnd(layout, board.id, toTeamId);
    if (!result?.move) {
      setDialog(null);
      return;
    }
    requestMove(board.id, board.team, toTeamId, result.layout, edits);
  }

  async function confirmMove(move: PendingMove) {
    const previous = layout;
    setDialog(null);
    setError(null);
    setLayout(move.layout);
    try {
      if (move.edits) await api.updateBoard(move.boardId, move.edits);
      await api.moveBoard(move.boardId, move.toTeamId);
      // The endpoint puts the board last; this places it where it was dropped.
      await api.saveOrder(boardOrderPayload(move.layout, [move.toTeamId]));
    } catch (cause) {
      setLayout(previous);
      setError(errorMessage(cause, 'Could not move the board.'));
    }
    // Refresh either way: if the move succeeded and the reorder did not, the
    // server's version is the truth.
    onChanged();
  }

  const teamName = (id: string) => layout.find((entry) => entry.team.id === id)?.team.name ?? 'team';
  const boardName = (id: string) =>
    layout.flatMap((entry) => entry.boards).find((board) => board.id === id)?.name ?? 'board';
  const label = (raw: string | number) => {
    const parsed = parseItemId(raw);
    if (!parsed) return 'item';
    if (parsed.kind === 'team') return `team ${teamName(parsed.id)}`;
    if (parsed.kind === 'board') return `board ${boardName(parsed.id)}`;
    return `team ${teamName(parsed.id)}`;
  };

  // Screen reader narration while dragging. dnd-kit's defaults would read out the
  // prefixed ids ("board:abc"), which means nothing to anyone.
  const announcements: Announcements = {
    onDragStart: ({ active }) => `Picked up ${label(active.id)}.`,
    onDragOver: ({ active, over }) =>
      over ? `${label(active.id)} is over ${label(over.id)}.` : `${label(active.id)} is not over a drop target.`,
    onDragEnd: ({ active, over }) =>
      over ? `Dropped ${label(active.id)} on ${label(over.id)}.` : `Dropped ${label(active.id)}.`,
    onDragCancel: ({ active }) => `Cancelled moving ${label(active.id)}.`,
  };

  const [dragging, setDragging] = useState(false);
  const onDragStart = (_event: DragStartEvent) => setDragging(true);

  const activeTeams = layout.map((entry) => entry.team);
  const hasArchived = archived.teams.length > 0 || archived.boards.length > 0;

  return (
    <div className={dragging ? 'sidebar-edit is-dragging' : 'sidebar-edit'}>
      {error && (
        <p className="sidebar__note sidebar__note--error" role="alert">
          {error}
        </p>
      )}

      <DndContext
        id="sidebar-editor"
        sensors={sensors}
        collisionDetection={collide}
        accessibility={{ announcements }}
        onDragStart={onDragStart}
        onDragEnd={(event) => {
          setDragging(false);
          onDragEnd(event);
        }}
        onDragCancel={() => setDragging(false)}
      >
        <SortableContext
          items={layout.map((entry) => teamItemId(entry.team.id))}
          strategy={verticalListSortingStrategy}
        >
          {layout.map(({ team, boards }) => (
            <SortableTeam
              key={team.id}
              team={team}
              onEdit={() => setDialog({ kind: 'editTeam', team })}
              onArchive={() => setDialog({ kind: 'archiveTeam', team })}
            >
              <SortableContext
                items={boards.map((board) => boardItemId(board.id))}
                strategy={verticalListSortingStrategy}
              >
                {boards.length === 0 ? (
                  <EmptyTeam teamId={team.id} />
                ) : (
                  <ul className="sidebar__boards">
                    {boards.map((board) => (
                      <SortableBoard
                        key={board.id}
                        board={board}
                        onEdit={() => setDialog({ kind: 'editBoard', board })}
                        onArchive={() => setDialog({ kind: 'archiveBoard', board })}
                      />
                    ))}
                  </ul>
                )}
              </SortableContext>

              <button
                className="button button--quiet sidebar-edit__add"
                type="button"
                aria-label={`Add board to ${team.name}`}
                onClick={() => setDialog({ kind: 'newBoard', team })}
              >
                + Add board
              </button>
            </SortableTeam>
          ))}
        </SortableContext>
      </DndContext>

      {layout.length === 0 && <p className="sidebar__note">No teams yet.</p>}

      <button
        className="button sidebar-edit__add-team"
        type="button"
        onClick={() => setDialog({ kind: 'newTeam' })}
      >
        + Add team
      </button>

      <details className="sidebar-archived">
        <summary>Archived</summary>
        {!hasArchived && <p className="sidebar__note">Nothing is archived.</p>}

        {archived.teams.length > 0 && (
          <>
            <h3 className="sidebar-archived__heading">Teams</h3>
            <ul className="sidebar-archived__list">
              {archived.teams.map((team) => (
                <li className="sidebar-archived__item" key={team.id}>
                  <span className="sidebar-archived__name">{team.name}</span>
                  <button
                    className="button button--quiet"
                    type="button"
                    aria-label={`Restore team ${team.name}`}
                    disabled={busy}
                    onClick={() => void run(() => api.restoreTeam(team.id), 'Could not restore the team.')}
                  >
                    Restore
                  </button>
                </li>
              ))}
            </ul>
          </>
        )}

        {archived.boards.length > 0 && (
          <>
            <h3 className="sidebar-archived__heading">Boards</h3>
            <ul className="sidebar-archived__list">
              {archived.boards.map(({ board, team }) => (
                <li className="sidebar-archived__item" key={board.id}>
                  <span className="sidebar-archived__name">
                    {board.name}
                    <small>{team.name}</small>
                  </span>
                  <button
                    className="button button--quiet"
                    type="button"
                    aria-label={`Restore board ${board.name}`}
                    disabled={busy}
                    onClick={() =>
                      void run(() => api.restoreBoard(board.id), 'Could not restore the board.')
                    }
                  >
                    Restore
                  </button>
                </li>
              ))}
            </ul>
          </>
        )}
      </details>

      {dialog?.kind === 'newTeam' && (
        <TeamDialog
          nextSortOrder={nextSortOrder(layout.map((entry) => entry.team.sort_order))}
          onClose={closeDialog}
          onSaved={() => {
            closeDialog();
            onChanged();
          }}
        />
      )}

      {dialog?.kind === 'editTeam' && (
        <TeamDialog
          team={dialog.team}
          onClose={closeDialog}
          onSaved={() => {
            closeDialog();
            onChanged();
          }}
        />
      )}

      {dialog?.kind === 'newBoard' && (
        <BoardDialog
          team={dialog.team}
          nextSortOrder={nextSortOrder(
            layout.find((entry) => entry.team.id === dialog.team.id)?.boards.map((b) => b.sort_order) ?? [],
          )}
          onClose={closeDialog}
          onSaved={() => {
            closeDialog();
            onChanged();
          }}
        />
      )}

      {dialog?.kind === 'editBoard' && (
        <BoardDialog
          board={dialog.board}
          teams={activeTeams}
          onClose={closeDialog}
          onMoveRequested={onMoveRequested}
          onSaved={() => {
            closeDialog();
            onChanged();
          }}
        />
      )}

      {dialog?.kind === 'archiveTeam' && (
        <ConfirmDialog
          title={`Archive ${dialog.team.name}?`}
          confirmLabel="Archive team"
          danger
          busy={busy}
          onCancel={closeDialog}
          onConfirm={() =>
            void run(() => api.archiveTeam(dialog.team.id), 'Could not archive the team.')
          }
        >
          <p>
            The team and all of its boards disappear from everyone&rsquo;s sidebar. Nothing is
            deleted, and you can restore it from Archived.
          </p>
        </ConfirmDialog>
      )}

      {dialog?.kind === 'archiveBoard' && (
        <ConfirmDialog
          title={`Archive ${dialog.board.name}?`}
          confirmLabel="Archive board"
          danger
          busy={busy}
          onCancel={closeDialog}
          onConfirm={() =>
            void run(() => api.archiveBoard(dialog.board.id), 'Could not archive the board.')
          }
        >
          <p>
            The board disappears from the sidebar and becomes read-only. Its cards and history are
            kept, and you can restore it from Archived.
          </p>
        </ConfirmDialog>
      )}

      {dialog?.kind === 'move' && (
        <ConfirmDialog
          title={`Move ${dialog.move.boardName} to ${teamName(dialog.move.toTeamId)}?`}
          confirmLabel="Move board"
          busy={busy}
          onCancel={closeDialog}
          onConfirm={() => void confirmMove(dialog.move)}
        >
          <p>
            The board&rsquo;s cards and history move with it. Members of{' '}
            <strong>{teamName(dialog.move.fromTeamId)}</strong> lose access to the board unless they
            are also on <strong>{teamName(dialog.move.toTeamId)}</strong>.
          </p>
        </ConfirmDialog>
      )}
    </div>
  );
}

function SortableTeam({
  team,
  onEdit,
  onArchive,
  children,
}: {
  team: TeamRecord;
  onEdit: () => void;
  onArchive: () => void;
  children: ReactNode;
}) {
  const sortable = useSortable({ id: teamItemId(team.id) });
  const style: CSSProperties = {
    transform: CSS.Transform.toString(sortable.transform),
    transition: sortable.transition,
  };
  return (
    <section
      className={
        sortable.isDragging
          ? 'sidebar__team sidebar-edit__team is-dragging'
          : 'sidebar__team sidebar-edit__team'
      }
      ref={sortable.setNodeRef}
      style={style}
      aria-labelledby={`team-heading-${team.id}`}
    >
      <div className="sidebar-edit__row sidebar-edit__row--team">
        <button
          className="drag-handle"
          type="button"
          ref={sortable.setActivatorNodeRef}
          {...sortable.attributes}
          {...sortable.listeners}
          aria-label={`Reorder team ${team.name}`}
        >
          <span aria-hidden="true">⋮⋮</span>
        </button>
        <h2 className="sidebar__team-name sidebar-edit__name" id={`team-heading-${team.id}`}>{team.name}</h2>
        <RowButton glyph="✎" label={`Edit team ${team.name}`} onClick={onEdit} />
        <RowButton glyph="🗄" label={`Archive team ${team.name}`} onClick={onArchive} />
      </div>
      {children}
    </section>
  );
}

function SortableBoard({
  board,
  onEdit,
  onArchive,
}: {
  board: BoardRecord;
  onEdit: () => void;
  onArchive: () => void;
}) {
  const sortable = useSortable({ id: boardItemId(board.id) });
  const style: CSSProperties = {
    transform: CSS.Transform.toString(sortable.transform),
    transition: sortable.transition,
  };
  return (
    <li
      className={sortable.isDragging ? 'sidebar-edit__row is-dragging' : 'sidebar-edit__row'}
      ref={sortable.setNodeRef}
      style={style}
    >
      <button
        className="drag-handle"
        type="button"
        ref={sortable.setActivatorNodeRef}
        {...sortable.attributes}
        {...sortable.listeners}
        aria-label={`Reorder board ${board.name}`}
      >
        <span aria-hidden="true">⋮⋮</span>
      </button>
      <span className="sidebar-edit__name">{board.name}</span>
      <RowButton glyph="✎" label={`Edit board ${board.name}`} onClick={onEdit} />
      <RowButton glyph="🗄" label={`Archive board ${board.name}`} onClick={onArchive} />
    </li>
  );
}

/** Where a board can be dropped into a team that has none. */
function EmptyTeam({ teamId }: { teamId: string }) {
  const { setNodeRef, isOver } = useDroppable({ id: dropItemId(teamId) });
  return (
    <p
      className={isOver ? 'sidebar__note sidebar-edit__empty is-over' : 'sidebar__note sidebar-edit__empty'}
      ref={setNodeRef}
    >
      No boards yet. Drag one here.
    </p>
  );
}

function RowButton({ glyph, label, onClick }: { glyph: string; label: string; onClick: () => void }) {
  return (
    <button className="icon-button" type="button" aria-label={label} title={label} onClick={onClick}>
      <span aria-hidden="true">{glyph}</span>
    </button>
  );
}

/**
 * Which drop target a drag is over.
 *
 * Teams only ever sort among teams, and boards among boards and empty teams.
 * Without the filter a dragged board would often be "over" its team's own
 * wrapper, which is not a position anyone meant. The pointer decides when there
 * is one; the keyboard has no pointer, so it falls back to the nearest centre.
 */
const collide: CollisionDetection = (args) => {
  const activeKind = parseItemId(args.active.id)?.kind;
  const droppableContainers = args.droppableContainers.filter((container) => {
    const kind = parseItemId(container.id)?.kind;
    return activeKind === 'team' ? kind === 'team' : kind === 'board' || kind === 'drop';
  });
  const scoped = { ...args, droppableContainers };

  const hits = pointerWithin(scoped);
  if (hits.length > 0) return hits;
  return closestCenter(scoped);
};
