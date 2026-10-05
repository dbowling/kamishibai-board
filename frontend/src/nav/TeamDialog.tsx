import { useState } from 'react';
import { api, errorMessage } from '../lib/api';
import { TimeZoneSelect } from '../lib/TimeZoneSelect';
import type { TeamRecord } from '../lib/types';
import { Modal } from './Modal';

interface TeamDialogProps {
  /** The team being edited; omit to create one. */
  team?: TeamRecord;
  /** The sort_order a new team takes, so it lands last. Ignored when editing. */
  nextSortOrder?: number;
  /** The instance default zone, which an empty team zone inherits. Named in the select. */
  defaultTimezone?: string | null;
  onClose: () => void;
  onSaved: () => void;
}

/** Create a team, or rename and re-describe an existing one. Admin only. */
export function TeamDialog({
  team,
  nextSortOrder,
  defaultTimezone,
  onClose,
  onSaved,
}: TeamDialogProps) {
  const [name, setName] = useState(team?.name ?? '');
  const [description, setDescription] = useState(team?.description ?? '');
  const [timezone, setTimezone] = useState(team?.timezone ?? '');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const editing = Boolean(team);
  // Only an edit that actually changes the zone needs the warning: a new team has
  // no history to leave a seam in.
  const zoneChanged = team !== undefined && timezone !== (team.timezone ?? '');

  async function save() {
    setError(null);
    setBusy(true);
    try {
      if (team) {
        await api.updateTeam(team.id, {
          name: name.trim(),
          description: description.trim(),
          timezone,
        });
      } else {
        await api.createTeam(name.trim(), description.trim(), nextSortOrder, timezone);
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

      <TimeZoneSelect
        label="Time zone"
        value={timezone}
        onChange={setTimezone}
        emptyLabel={defaultTimezone ? `Instance default (${defaultTimezone})` : 'Instance default'}
        hint="The clock this team's periods roll over on: when a day, week or month starts and ends, and which day a completion counts for."
      />

      {zoneChanged && (
        <p className="field__warning" role="status">
          Changing the time zone moves when this team&rsquo;s periods roll over from now on. Past
          records and reports keep the boundaries they were recorded with, so reports will show a
          seam where the zone changed.
        </p>
      )}

      {error && (
        <p className="login__error" role="alert">
          {error}
        </p>
      )}
    </Modal>
  );
}
