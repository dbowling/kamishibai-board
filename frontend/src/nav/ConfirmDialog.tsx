import type { ReactNode } from 'react';
import { Modal } from './Modal';

interface ConfirmDialogProps {
  title: string;
  /** Plain explanation of what will happen. */
  children: ReactNode;
  confirmLabel: string;
  /** Shown instead of the label while the action runs. */
  busy?: boolean;
  /** Style the confirm button as dangerous (archiving). */
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

/**
 * Asks before doing something that surprises people: archiving hides a team or
 * board from everyone, and moving a board changes who can see it.
 *
 * Cancel takes initial focus, so an Enter keypress that opened the dialog cannot
 * also confirm it.
 */
export function ConfirmDialog({
  title,
  children,
  confirmLabel,
  busy = false,
  danger = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  return (
    <Modal
      title={title}
      titleId="confirm-dialog-title"
      role="alertdialog"
      onClose={onCancel}
      onSubmit={onConfirm}
      footer={
        <>
          <button className="button button--quiet" type="button" onClick={onCancel} autoFocus>
            Cancel
          </button>
          <button
            className={danger ? 'button button--danger' : 'button button--primary'}
            type="submit"
            disabled={busy}
          >
            {confirmLabel}
          </button>
        </>
      }
    >
      <div className="modal__text">{children}</div>
    </Modal>
  );
}
