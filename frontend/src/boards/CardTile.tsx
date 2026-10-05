import { useEffect, useRef, useState } from 'react';
import { cadenceLabel, cadenceReset, dateTime, statusLabel, timeUntil } from '../lib/format';
import type { Card, Status } from '../lib/types';

interface CardTileProps {
  card: Card;
  busy?: boolean;
  onOpen: (card: Card) => void;
  onStart: (card: Card) => void;
  onComplete: (card: Card) => void;
  onReopen: (card: Card) => void;
  /** The viewer's display zone for timestamps. Omit for the browser's. */
  timeZone?: string;
}

/**
 * A single card on the board.
 *
 * The physical kamishibai convention is a two-sided card: turned one way it shows
 * the work is outstanding, turned the other it shows it is done. The status is
 * carried by the face colour *and* by text, because colour alone would exclude
 * anybody who cannot distinguish red from green, which is a lot of people on a
 * typical team.
 */
export function CardTile({
  card,
  busy,
  onOpen,
  onStart,
  onComplete,
  onReopen,
  timeZone,
}: CardTileProps) {
  const { state } = card;
  const flipping = useFlipOnStatusChange(state.status);

  const headingId = `card-${card.id}-title`;
  const attribution = describeAttribution(card, timeZone);

  return (
    <li
      className={[
        'card',
        `card--${state.status}`,
        flipping ? 'card--flipping' : '',
        card.archived ? 'card--archived' : '',
        busy ? 'card--busy' : '',
      ]
        .filter(Boolean)
        .join(' ')}
    >
      {/* Grouping by the title lets a screen reader announce which card these
          controls belong to. */}
      <article className="card__inner" aria-labelledby={headingId}>
        <header className="card__header">
          <span className="card__cadence" title={cadenceReset(card.cadence)}>
            {cadenceLabel(card.cadence)}
          </span>
          <span className={`card__status card__status--${state.status}`}>
            {statusLabel(state.status)}
          </span>
        </header>

        <h3 className={'card__title'} id={headingId}>
          <button className="card__open" type="button" onClick={() => onOpen(card)}>
            {card.title}
          </button>
        </h3>

        {card.summary && <p className="card__summary">{card.summary}</p>}

        <dl className="card__meta">
          <div className="card__meta-row">
            <dt>Period</dt>
            <dd>
              {card.period.key}
              {state.status !== 'done' && (
                <span
                  className="card__deadline"
                  title={`Closes ${dateTime(card.period.end, timeZone)}`}
                >
                  {' '}
                  · {timeUntil(card.period.end)}
                </span>
              )}
            </dd>
          </div>
          {attribution && (
            <div className="card__meta-row">
              <dt>Who</dt>
              <dd>{attribution}</dd>
            </div>
          )}
        </dl>

        {card.archived ? (
          <p className="card__archived-note">
            Archived{card.archivedAt ? ` on ${dateTime(card.archivedAt, timeZone)}` : ''} · read-only
          </p>
        ) : (
          <div className="card__actions">
            {state.status === 'not_started' && (
              <>
                <button
                  className="button button--ghost"
                  type="button"
                  disabled={busy}
                  onClick={() => onStart(card)}
                >
                  Start
                </button>
                <button
                  className="button button--primary"
                  type="button"
                  disabled={busy}
                  onClick={() => onComplete(card)}
                >
                  Mark done
                </button>
              </>
            )}

            {state.status === 'in_progress' && (
              <button
                className="button button--primary"
                type="button"
                disabled={busy}
                onClick={() => onComplete(card)}
              >
                Mark done
              </button>
            )}

            {state.status === 'done' && (
              <button
                className="button button--ghost"
                type="button"
                disabled={busy}
                onClick={() => onReopen(card)}
              >
                Reopen
              </button>
            )}
          </div>
        )}
      </article>
    </li>
  );
}

/**
 * Adds a flip class for the length of the animation whenever the status changes.
 *
 * Skipped on the first render, so arriving at a board does not set every card
 * spinning, and skipped entirely when the user has asked for reduced motion.
 */
function useFlipOnStatusChange(status: Status): boolean {
  const [flipping, setFlipping] = useState(false);
  const previous = useRef<Status | null>(null);

  useEffect(() => {
    const changed = previous.current !== null && previous.current !== status;
    previous.current = status;
    if (!changed) return;

    if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return;

    setFlipping(true);
    const timer = window.setTimeout(() => setFlipping(false), 420);
    return () => window.clearTimeout(timer);
  }, [status]);

  return flipping;
}

/**
 * A short sentence naming who did what, or '' when nothing has happened yet.
 * Times are shown in `timeZone` (the viewer's display zone), defaulting to the
 * browser's.
 */
export function describeAttribution(card: Card, timeZone?: string): string {
  const { startedBy, completedBy, startedAt, completedAt } = card.state;

  if (card.state.status === 'done') {
    const who = completedBy?.name || completedBy?.email || 'someone';
    const when = dateTime(completedAt, timeZone);
    // Only mention the starter when it was a different person, otherwise the
    // sentence reads oddly ("done by Dana, started by Dana").
    const starter = startedBy && completedBy && startedBy.id !== completedBy.id ? startedBy.name : '';
    const base = `Done by ${who}${when ? ` · ${when}` : ''}`;
    return starter ? `${base} · started by ${starter}` : base;
  }

  if (card.state.status === 'in_progress') {
    const who = startedBy?.name || startedBy?.email || 'someone';
    const when = dateTime(startedAt, timeZone);
    return `Started by ${who}${when ? ` · ${when}` : ''}`;
  }

  return '';
}
