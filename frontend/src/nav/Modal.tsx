import { useEffect } from 'react';
import type { FormEvent, ReactNode } from 'react';

interface ModalProps {
  /** Heading text. It also names the dialog for assistive technology. */
  title: string;
  /** Used to link the heading to the dialog; must be unique on the page. */
  titleId: string;
  onClose: () => void;
  /** Called on Enter in a field or on the primary button. */
  onSubmit?: () => void;
  role?: 'dialog' | 'alertdialog';
  footer: ReactNode;
  children?: ReactNode;
}

/**
 * The shell the sidebar's small dialogs share: a centred modal with a labelled
 * heading, Escape and backdrop click to close, and focus handed back to whatever
 * opened it.
 *
 * The card forms use a side drawer because they are long. These are one or two
 * fields, so a centred box keeps them next to the thing being edited. Content
 * decides what gets initial focus via `autoFocus`.
 */
export function Modal({
  title,
  titleId,
  onClose,
  onSubmit,
  role = 'dialog',
  footer,
  children,
}: ModalProps) {
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [onClose]);

  // Remember what had focus (the button that opened the dialog) and give it back
  // on close, so a keyboard user is not dropped at the top of the page.
  useEffect(() => {
    const opener = document.activeElement;
    return () => {
      if (opener instanceof HTMLElement && opener.isConnected) opener.focus();
    };
  }, []);

  function submit(event: FormEvent) {
    event.preventDefault();
    onSubmit?.();
  }

  return (
    <div className="modal" role="presentation" onClick={onClose}>
      <form
        className="modal__panel"
        role={role}
        aria-modal="true"
        aria-labelledby={titleId}
        onClick={(event) => event.stopPropagation()}
        onSubmit={submit}
      >
        <h2 className="modal__title" id={titleId}>
          {title}
        </h2>
        <div className="modal__body">{children}</div>
        <footer className="modal__footer">{footer}</footer>
      </form>
    </div>
  );
}
