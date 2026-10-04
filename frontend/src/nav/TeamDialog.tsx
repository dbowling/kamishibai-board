import { useState } from 'react';
import { api, errorMessage } from '../lib/api';
import type { TeamRecord } from '../lib/types';
import { Modal } from './Modal';

interface TeamDialogProps {
  /** The team being edited; omit to create one. */
  team?: TeamRecord;
  /** The sort_order a new team takes, so it lands last. Ignored when editing. */
  nextSortOrder?: number;
  onClose: () => void;
  onSaved: () => void;
}

/** Create a team, or rename and re-describe an existing one. Admin only. */
export function TeamDialog({ team, nextSortOrder, onClose, onSaved }: TeamDialogProps) {
  const [name, setName] = useState(team?.name ?? '');
  const [description, setDescription] = useState(team?.description ?? '');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const editing = Boolean(team);

  async function save() {
    setError(null);
    setBusy(true);
    try {
      if (team) {
        await api.updateTeam(team.id, { name: name.trim(), description: description.trim() });
      } else {
        await api.createTeam(name.trim(), description.trim(), nextSortOrder);
      }
      onSaved();
    } catch (cause) {
      setError(errorMessage(cause, editing ? 'Could not save the team.' : 'Could not create the team.'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      title={editing ? 'Edit team' : 'New team'}
      titleId="team-dialog-title"
      onClose={onClose}
      onSubmit={() => void save()}
      footer={
        <>
          <button className="button button--quiet" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="button button--primary" type="submit" disabled={busy || !name.trim()}>
            {busy ? 'Saving…' : editing ? 'Save team' : 'Create team'}
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
          placeholder="Platform"
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
        <span className="field__hint">Optional. Only admins see it.</span>
      </label>

      {error && (
        <p className="login__error" role="alert">
          {error}
        </p>
      )}
    </Modal>
  );
}
