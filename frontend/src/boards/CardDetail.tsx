import { useEffect, useMemo, useRef, useState } from 'react';
import { cadenceLabel, cadenceReset, dateTime, periodKeyLabel, statusLabel } from '../lib/format';
import { displayHost, safeUrl, sanitizeInstructions } from '../lib/sanitize';
import type { Card } from '../lib/types';
import { describeAttribution } from './CardTile';

interface CardDetailProps {
  card: Card;
  busy?: boolean;
  canRestore: boolean;
  onClose: () => void;
  onStart: (card: Card, notes: string) => void;
  onComplete: (card: Card, notes: string) => void;
  onReopen: (card: Card, notes: string) => void;
  onArchive: (card: Card) => void;
  onRestore: (card: Card) => void;
}

/**
 * The full card: instructions, quick links, and the checklist.
 *
 * This is where a card earns its keep. "Verify Backups" is only useful if the
 * person picking it up at 8am can see the dashboard link and the three things to
 * try when it has failed, without hunting through a wiki.
 */
export function CardDetail({
  card,
  busy,
  canRestore,
  onClose,
  onStart,
  onComplete,
  onReopen,
  onArchive,
  onRestore,
}: CardDetailProps) {
  const [notes, setNotes] = useState(card.state.notes ?? '');
  const dialogRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);

  // Instructions are rich text written by a teammate, so they are sanitised
  // before being inserted as HTML. Memoised because sanitising is not free and
  // the markup only changes when the card does.
  const instructions = useMemo(() => sanitizeInstructions(card.instructions), [card.instructions]);

  const links = useMemo(
    () =>
      card.links
        .map((link) => ({ ...link, href: safeUrl(link.url) }))
        .filter((link): link is typeof link & { href: string } => link.href !== null),
    [card.links],
  );

  useEffect(() => {
    setNotes(card.state.notes ?? '');
  }, [card.id, card.state.notes]);

  // Move focus into the panel so keyboard and screen reader users land here
  // rather than being left behind on the board.
  useEffect(() => {
    closeRef.current?.focus();
  }, []);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [onClose]);

  const titleId = `detail-${card.id}-title`;

  return (
    <div className="drawer" role="presentation" onClick={onClose}>
      <div
        className="drawer__panel"
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        onClick={(event) => event.stopPropagation()}
      >
        <header className="drawer__header">
          <div>
            <p className="drawer__eyebrow">
              <span title={cadenceReset(card.cadence)}>{cadenceLabel(card.cadence)}</span>
              {' · '}
              {periodKeyLabel(card.period.key)}
            </p>
            <h2 className="drawer__title" id={titleId}>
              {card.title}
            </h2>
            <p className={`drawer__status drawer__status--${card.state.status}`}>
              {statusLabel(card.state.status)}
              {describeAttribution(card) && <span> · {describeAttribution(card)}</span>}
            </p>
          </div>

          <button className="drawer__close" type="button" onClick={onClose} ref={closeRef}>
            <span aria-hidden="true">×</span>
            <span className="visually-hidden">Close</span>
          </button>
        </header>

        <div className="drawer__body">
          {card.summary && <p className="drawer__summary">{card.summary}</p>}

          {instructions && (
            <section className="drawer__section" aria-labelledby={`${titleId}-instructions`}>
              <h3 className="drawer__section-title" id={`${titleId}-instructions`}>
                Instructions
              </h3>
              {/* Sanitised immediately above; see sanitize.ts for the allowlist. */}
              <div
                className="drawer__prose"
                dangerouslySetInnerHTML={{ __html: instructions }}
              />
            </section>
          )}

          {links.length > 0 && (
            <section className="drawer__section" aria-labelledby={`${titleId}-links`}>
              <h3 className="drawer__section-title" id={`${titleId}-links`}>
                Links
              </h3>
              <ul className="link-list">
                {links.map((link) => (
                  <li key={`${link.href}-${link.label}`}>
                    {/* noreferrer as well as noopener: these point at internal
                        dashboards, and the referrer can leak board ids. */}
                    <a
                      className="link-list__link"
                      href={link.href}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {link.label || link.href}
                      {displayHost(link.href) && (
                        <span className="link-list__host">{displayHost(link.href)}</span>
                      )}
                    </a>
                  </li>
                ))}
              </ul>
            </section>
          )}

          {card.checklist.length > 0 && (
            <section className="drawer__section" aria-labelledby={`${titleId}-checklist`}>
              <h3 className="drawer__section-title" id={`${titleId}-checklist`}>
                Steps
              </h3>
              <Checklist cardId={card.id} items={card.checklist} />
              <p className="drawer__hint">
                Ticks are a scratch pad for this sitting and are not saved. What gets recorded is
                who started and completed the card.
              </p>
            </section>
          )}

          {!card.archived && (
            <section className="drawer__section" aria-labelledby={`${titleId}-notes`}>
              <h3 className="drawer__section-title" id={`${titleId}-notes`}>
                Notes
              </h3>
              <textarea
                className="field__input drawer__notes"
                id={`${titleId}-notes-input`}
                rows={3}
                value={notes}
                placeholder="Anything the next person should know."
                onChange={(event) => setNotes(event.target.value)}
              />
            </section>
          )}
        </div>

        <footer className="drawer__footer">
          {card.archived ? (
            <>
              <p className="drawer__archived">
                Archived{card.archivedAt ? ` on ${dateTime(card.archivedAt)}` : ''}. Its history is
                kept.
              </p>
              {canRestore ? (
                <button
                  className="button button--primary"
                  type="button"
                  disabled={busy}
                  onClick={() => onRestore(card)}
                >
                  Restore
                </button>
              ) : (
                <p className="drawer__hint">An administrator can restore this card.</p>
              )}
            </>
          ) : (
            <>
              <button
                className="button button--quiet"
                type="button"
                disabled={busy}
                onClick={() => onArchive(card)}
              >
                Archive
              </button>

              <div className="drawer__actions">
                {card.state.status === 'not_started' && (
                  <button
                    className="button button--ghost"
                    type="button"
                    disabled={busy}
                    onClick={() => onStart(card, notes)}
                  >
                    Start
                  </button>
                )}
                {card.state.status === 'done' ? (
                  <button
                    className="button button--ghost"
                    type="button"
                    disabled={busy}
                    onClick={() => onReopen(card, notes)}
                  >
                    Reopen
                  </button>
                ) : (
                  <button
                    className="button button--primary"
                    type="button"
                    disabled={busy}
                    onClick={() => onComplete(card, notes)}
                  >
                    Mark done
                  </button>
                )}
              </div>
            </>
          )}
        </footer>
      </div>
    </div>
  );
}

/**
 * The checklist.
 *
 * Ticks are local and intentionally not persisted. Per-step state was not asked
 * for, and storing it would mean inventing rules for what happens when the steps
 * themselves are edited. The card's own status is the thing of record.
 */
function Checklist({ cardId, items }: { cardId: string; items: { text: string }[] }) {
  const [checked, setChecked] = useState<Set<number>>(new Set());

  // Reset when switching cards, so ticks never bleed across.
  useEffect(() => {
    setChecked(new Set());
  }, [cardId]);

  return (
    <ul className="checklist">
      {items.map((item, index) => {
        const id = `checklist-${cardId}-${index}`;
        const isChecked = checked.has(index);

        return (
          <li className="checklist__item" key={id}>
            <input
              className="checklist__box"
              type="checkbox"
              id={id}
              checked={isChecked}
              onChange={() =>
                setChecked((previous) => {
                  const next = new Set(previous);
                  if (next.has(index)) {
                    next.delete(index);
                  } else {
                    next.add(index);
                  }
                  return next;
                })
              }
            />
            <label className={isChecked ? 'checklist__label is-checked' : 'checklist__label'} htmlFor={id}>
              {item.text}
            </label>
          </li>
        );
      })}
    </ul>
  );
}
