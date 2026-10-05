import { useState } from 'react';
import { api, errorMessage } from '../lib/api';
import { browserTimeZone } from '../lib/format';
import { TimeZoneSelect } from '../lib/TimeZoneSelect';
import { Modal } from '../nav/Modal';

interface TimeZoneDialogProps {
  userId: string;
  /** The saved preference; '' means the browser's zone. */
  current: string;
  onClose: () => void;
}

/**
 * Choose the zone times are displayed in.
 *
 * This is deliberately framed as display only. A team's periods roll over on the
 * team's clock whatever you pick here; this just changes how a timestamp such as
 * "Done by Mei at 09:12" reads for you.
 */
export function TimeZoneDialog({ userId, current, onClose }: TimeZoneDialogProps) {
  const [zone, setZone] = useState(current);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function save() {
    setError(null);
    setBusy(true);
    try {
      await api.updateDisplayTimeZone(userId, zone);
      onClose();
    } catch (cause) {
      setError(errorMessage(cause, 'Could not save your time zone.'));
      setBusy(false);
    }
  }

  return (
    <Modal
      title="Display time zone"
      titleId="timezone-dialog-title"
      onClose={onClose}
      onSubmit={() => void save()}
      footer={
        <>
          <button className="button button--quiet" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="button button--primary" type="submit" disabled={busy}>
            {busy ? 'Saving…' : 'Save'}
          </button>
        </>
      }
    >
      <TimeZoneSelect
        label="Show times in"
        value={zone}
        onChange={setZone}
        emptyLabel={`Browser default (${browserTimeZone()})`}
        hint="This only changes how times are displayed to you. It does not change when a team's periods start or end, or what counts as done."
      />

      {error && (
        <p className="login__error" role="alert">
          {error}
        </p>
      )}
    </Modal>
  );
}
