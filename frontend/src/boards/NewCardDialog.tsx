import { useEffect, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { api, errorMessage } from '../lib/api';
import { cadenceLabel, cadenceReset } from '../lib/format';
import { safeUrl } from '../lib/sanitize';
import { CADENCES } from '../lib/types';
import type { Cadence } from '../lib/types';

interface NewCardDialogProps {
  boardId: string;
  nextSortOrder: number;
  onClose: () => void;
  onCreated: () => void;
}

/**
 * Create a card. Any member of the team may do this, which is why the form is one
 * click away from the board rather than hidden in an admin area.
 *
 * Links and steps are entered one per line: a plain textarea is faster to fill in
 * than a repeater widget, and these lists are short.
 */
export function NewCardDialog({ boardId, nextSortOrder, onClose, onCreated }: NewCardDialogProps) {
  const [title, setTitle] = useState('');
  const [summary, setSummary] = useState('');
  const [cadence, setCadence] = useState<Cadence>('daily');
  const [instructions, setInstructions] = useState('');
  const [linksText, setLinksText] = useState('');
  const [checklistText, setChecklistText] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const titleRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    titleRef.current?.focus();
  }, []);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [onClose]);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setError(null);

    const links = parseLinks(linksText);
    if (links.rejected.length > 0) {
      setError(
        `These links were not accepted (only http and https are allowed): ${links.rejected.join(', ')}`,
      );
      return;
    }

    setBusy(true);
    try {
      await api.createCard({
        boardId,
        title: title.trim(),
        summary: summary.trim(),
        cadence,
        // Wrapped in a paragraph so the stored value is valid HTML for the editor
        // field, and escaped so a typed angle bracket cannot inject markup.
        instructions: toParagraphs(instructions),
        links: links.accepted,
        checklist: parseChecklist(checklistText),
        sortOrder: nextSortOrder,
      });
      onCreated();
    } catch (cause) {
      setError(errorMessage(cause, 'Could not create the card.'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="drawer" role="presentation" onClick={onClose}>
      <form
        className="drawer__panel"
        role="dialog"
        aria-modal="true"
        aria-labelledby="new-card-title"
        onClick={(event) => event.stopPropagation()}
        onSubmit={onSubmit}
      >
        <header className="drawer__header">
          <h2 className="drawer__title" id="new-card-title">
            New card
          </h2>
          <button className="drawer__close" type="button" onClick={onClose}>
            <span aria-hidden="true">×</span>
            <span className="visually-hidden">Close</span>
          </button>
        </header>

        <div className="drawer__body">
          <label className="field">
            <span className="field__label">Title</span>
            <input
              className="field__input"
              ref={titleRef}
              required
              maxLength={200}
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              placeholder="Verify Backups"
            />
          </label>

          <label className="field">
            <span className="field__label">Summary</span>
            <input
              className="field__input"
              maxLength={500}
              value={summary}
              onChange={(event) => setSummary(event.target.value)}
              placeholder="Confirm last night's backups completed."
            />
            <span className="field__hint">One line, shown on the card face.</span>
          </label>

          <label className="field">
            <span className="field__label">Cadence</span>
            <select
              className="field__input"
              value={cadence}
              onChange={(event) => setCadence(event.target.value as Cadence)}
            >
              {CADENCES.map((option) => (
                <option key={option} value={option}>
                  {cadenceLabel(option)}
                </option>
              ))}
            </select>
            <span className="field__hint">{cadenceReset(cadence)}</span>
          </label>

          <label className="field">
            <span className="field__label">Instructions</span>
            <textarea
              className="field__input"
              rows={5}
              value={instructions}
              onChange={(event) => setInstructions(event.target.value)}
              placeholder={'What to check, and what "good" looks like.'}
            />
          </label>

          <label className="field">
            <span className="field__label">Links</span>
            <textarea
              className="field__input"
              rows={3}
              value={linksText}
              onChange={(event) => setLinksText(event.target.value)}
              placeholder={'Grafana: backups | https://grafana.example.test/d/backups'}
            />
            <span className="field__hint">
              One per line, as <code>Label | https://url</code>.
            </span>
          </label>

          <label className="field">
            <span className="field__label">Steps</span>
            <textarea
              className="field__input"
              rows={4}
              value={checklistText}
              onChange={(event) => setChecklistText(event.target.value)}
              placeholder={'Check the backup job logs\nConfirm storage capacity'}
            />
            <span className="field__hint">One per line.</span>
          </label>

          {error && (
            <p className="login__error" role="alert">
              {error}
            </p>
          )}
        </div>

        <footer className="drawer__footer">
          <button className="button button--quiet" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="button button--primary" type="submit" disabled={busy || !title.trim()}>
            {busy ? 'Creating…' : 'Create card'}
          </button>
        </footer>
      </form>
    </div>
  );
}

/**
 * Parses `Label | url` lines, separating out anything with an unsafe scheme so the
 * form can say which lines it refused rather than silently dropping them.
 */
export function parseLinks(text: string): {
  accepted: { label: string; url: string }[];
  rejected: string[];
} {
  const accepted: { label: string; url: string }[] = [];
  const rejected: string[] = [];

  for (const rawLine of text.split('\n')) {
    const line = rawLine.trim();
    if (!line) continue;

    const separator = line.lastIndexOf('|');
    const label = separator === -1 ? '' : line.slice(0, separator).trim();
    const url = separator === -1 ? line : line.slice(separator + 1).trim();

    const safe = safeUrl(url);
    if (!safe) {
      rejected.push(url);
      continue;
    }
    accepted.push({ label: label || safe, url: safe });
  }

  return { accepted, rejected };
}

/** Splits a textarea into checklist items, dropping blank lines. */
export function parseChecklist(text: string): { text: string }[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => ({ text: line }));
}

/**
 * Converts plain text into simple paragraph markup, escaping it on the way.
 *
 * The instructions column is an HTML editor field, but this form takes plain
 * text. Escaping here means a title containing `<` is shown literally rather than
 * being stored as markup and later sanitised away.
 */
export function toParagraphs(text: string): string {
  const trimmed = text.trim();
  if (!trimmed) return '';

  return trimmed
    .split(/\n{2,}/)
    .map((block) => `<p>${escapeHtml(block).replace(/\n/g, '<br>')}</p>`)
    .join('');
}

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}
