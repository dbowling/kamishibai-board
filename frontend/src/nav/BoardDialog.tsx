import { useState } from 'react';
import { api, errorMessage } from '../lib/api';
import type { BoardRecord, TeamRecord } from '../lib/types';
import { Modal } from './Modal';

/** What the dialog hands back when the Team select was changed on an existing board. */
export interface MoveRequest {
  board: BoardRecord;
  toTeamId: string;
  /** Name and description changes to apply with the move, if any were made. */
  edits: { name: string; description: string } | null;
}

interface BoardDialogProps {
  /** Creating: the team the new board goes in. */
  team?: TeamRecord;
  /** Editing: the board and the active teams it could move to. */
  board?: BoardRecord;
  teams?: TeamRecord[];
  /** The sort_order a new board takes, so it lands last. Ignored when editing. */
  nextSortOrder?: number;
  onClose: () => void;
  onSaved: () => void;
  /**
   * Changing the team is a move, which the caller must confirm and perform. When
   * the Team select was changed this is called instead of saving anything, and
   * the dialog closes; the caller applies any `edits` after the user agrees.
   */
  onMoveRequested?: (request: MoveRequest) => void;
}

/** Create a board in a team, or edit a board's name, description and team. Admin only. */
export function BoardDialog({
  team,
  board,
  teams = [],
  nextSortOrder,
  onClose,
  onSaved,
  onMoveRequested,
}: BoardDialogProps) {
  const [name, setName] = useState(board?.name ?? '');
  const [description, setDescription] = useState(board?.description ?? '');
  const [teamId, setTeamId] = useState(board?.team ?? team?.id ?? '');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const editing = Boolean(board);
  const moving = editing && board !== undefined && teamId !== board.team;

  async function save() {
    setError(null);
    const trimmed = { name: name.trim(), description: description.trim() };

    if (board && moving) {
      const changed = trimmed.name !== board.name || trimmed.description !== board.description;
      onMoveRequested?.({ board, toTeamId: teamId, edits: changed ? trimmed : null });
      return;
    }

    setBusy(true);
    try {
      if (board) {
        await api.updateBoard(board.id, trimmed);
      } else {
        await api.createBoard(teamId, trimmed.name, trimmed.description, nextSortOrder);
      }
      onSaved();
    } catch (cause) {
      setError(errorMessage(cause, editing ? 'Could not save the board.' : 'Could not create the board.'));
    } finally {
      setBusy(false);
    }
  }

  const primary = busy ? 'Saving…' : !editing ? 'Create board' : moving ? 'Continue' : 'Save board';

  return (
    <Modal
      title={editing ? 'Edit board' : `New board in ${team?.name ?? 'team'}`}
      titleId="board-dialog-title"
      onClose={onClose}
      onSubmit={() => void save()}
      footer={
        <>
          <button className="button button--quiet" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="button button--primary" type="submit" disabled={busy || !name.trim()}>
            {primary}
          </button>
        </>
      }
    >
      <label className="field">
        <span className="field__label">Name</span>
        <input
          className="field__input"
          autoFocus
          required
          maxLength={200}
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder="Platform triage"
        />
      </label>

      <label className="field">
        <span className="field__label">Description</span>
        <input
          className="field__input"
          maxLength={500}
          value={description}
          onChange={(event) => setDescription(event.target.value)}
        />
        <span className="field__hint">Optional. Shown at the top of the board.</span>
      </label>

      {editing && (
        <label className="field">
          <span className="field__label">Team</span>
          <select
            className="field__input"
            value={teamId}
            onChange={(event) => setTeamId(event.target.value)}
          >
            {teams.map((option) => (
              <option key={option.id} value={option.id}>
                {option.name}
              </option>
            ))}
          </select>
          <span className="field__hint">
            {moving
              ? 'You will be asked to confirm: the board moves with its cards and history.'
              : 'Choose another team to move the board there.'}
          </span>
        </label>
      )}

      {error && (
        <p className="login__error" role="alert">
          {error}
        </p>
      )}
    </Modal>
  );
}
