import { useCallback, useMemo, useState } from 'react';
import { api, errorMessage } from '../lib/api';
import { cadenceLabel, percent } from '../lib/format';
import { CADENCES } from '../lib/types';
import type { Cadence, Card } from '../lib/types';
import { useAuth } from '../auth/AuthProvider';
import { CardDetail } from './CardDetail';
import { CardTile } from './CardTile';
import { matchesQuery } from './filterCards';
import { NewCardDialog } from './NewCardDialog';
import { useBoardState } from './useBoardState';

interface BoardPageProps {
  boardId: string;
}

export function BoardPage({ boardId }: BoardPageProps) {
  const { isAdmin } = useAuth();
  const { state, error, loading, refresh, includeArchived, setIncludeArchived } =
    useBoardState(boardId);

  const [openCardId, setOpenCardId] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [busyCardId, setBusyCardId] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [announcement, setAnnouncement] = useState('');
  const [cadenceFilter, setCadenceFilter] = useState<Cadence | 'all'>('all');
  const [query, setQuery] = useState('');

  const cards = state?.cards ?? [];

  const visibleCards = useMemo(
    () =>
      cards.filter(
        (c) => (cadenceFilter === 'all' || c.cadence === cadenceFilter) && matchesQuery(c, query),
      ),
    [cards, cadenceFilter, query],
  );

  // Grouped by cadence so the board reads like a kamishibai wall: the things due
  // today together, the longer rhythms below.
  const grouped = useMemo(() => {
    return CADENCES.map((cadence) => ({
      cadence,
      cards: visibleCards.filter((card) => card.cadence === cadence),
    })).filter((group) => group.cards.length > 0);
  }, [visibleCards]);

  const openCard = useMemo(
    () => cards.find((card) => card.id === openCardId) ?? null,
    [cards, openCardId],
  );

  /**
   * Runs a mutation and reports the outcome.
   *
   * The board is refetched rather than patched locally, because a teammate may
   * have changed something else in the meantime and the server's view is the one
   * that counts.
   */
  const run = useCallback(
    async (card: Card, action: 'start' | 'complete' | 'reopen', notes?: string) => {
      setBusyCardId(card.id);
      setActionError(null);

      try {
        await api[action](card.id, notes);
        setAnnouncement(
          action === 'complete'
            ? `${card.title} marked done.`
            : action === 'start'
              ? `${card.title} started.`
              : `${card.title} reopened.`,
        );
        refresh();
      } catch (cause) {
        setActionError(errorMessage(cause, `Could not ${action} ${card.title}.`));
      } finally {
        setBusyCardId(null);
      }
    },
    [refresh],
  );

  const archive = useCallback(
    async (card: Card) => {
      setBusyCardId(card.id);
      setActionError(null);
      try {
        await api.archiveCard(card.id);
        setAnnouncement(`${card.title} archived.`);
        setOpenCardId(null);
        refresh();
      } catch (cause) {
        setActionError(errorMessage(cause, 'Could not archive that card.'));
      } finally {
        setBusyCardId(null);
      }
    },
    [refresh],
  );

  const restore = useCallback(
    async (card: Card) => {
      setBusyCardId(card.id);
      setActionError(null);
      try {
        await api.restoreCard(card.id);
        setAnnouncement(`${card.title} restored.`);
        setOpenCardId(null);
        refresh();
      } catch (cause) {
        setActionError(errorMessage(cause, 'Could not restore that card.'));
      } finally {
        setBusyCardId(null);
      }
    },
    [refresh],
  );

  if (error) {
    return (
      <div className="panel panel--error" role="alert">
        <h2>That board is not available</h2>
        <p>{error}</p>
      </div>
    );
  }

  if (!state) {
    return <div className="panel">{loading ? 'Loading the board…' : 'No board selected.'}</div>;
  }

  const { summary } = state;

  return (
    <div className="board">
      <header className="board__header">
        <div>
          <h2 className="board__title">{state.board.name}</h2>
          {state.board.description && (
            <p className="board__description">{state.board.description}</p>
          )}
        </div>

        <div className="board__tally" aria-label="Progress for the current periods">
          <div className="tally">
            <span className="tally__value">{percent(summary.completionRate)}</span>
            <span className="tally__label">complete</span>
          </div>
          <ul className="tally__breakdown">
            <li>
              <span className="dot dot--done" aria-hidden="true" /> {summary.done} done
            </li>
            <li>
              <span className="dot dot--in_progress" aria-hidden="true" /> {summary.inProgress} in
              progress
            </li>
            <li>
              <span className="dot dot--not_started" aria-hidden="true" /> {summary.notStarted} not
              started
            </li>
          </ul>
        </div>
      </header>

      <div className="board__toolbar">
        <div className="board__filters">
          <label className="field field--inline">
            <span className="field__label">Filter</span>
            <input
              className="field__input"
              type="search"
              placeholder="Title or summary"
              autoComplete="off"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Escape' && query !== '') setQuery('');
              }}
            />
          </label>

          <label className="field field--inline">
            <span className="field__label">Cadence</span>
            <select
              className="field__input"
              value={cadenceFilter}
              onChange={(event) => setCadenceFilter(event.target.value as Cadence | 'all')}
            >
              <option value="all">All</option>
              {CADENCES.map((cadence) => (
                <option key={cadence} value={cadence}>
                  {cadenceLabel(cadence)}
                </option>
              ))}
            </select>
          </label>

          <label className="checkbox">
            <input
              type="checkbox"
              checked={includeArchived}
              onChange={(event) => setIncludeArchived(event.target.checked)}
            />
            Show archived
          </label>
        </div>

        <div className="board__toolbar-actions">
          <button className="button button--quiet" type="button" onClick={refresh}>
            Refresh
          </button>
          {!state.board.archived && (
            <button
              className="button button--primary"
              type="button"
              onClick={() => setCreating(true)}
            >
              New card
            </button>
          )}
        </div>
      </div>

      <p className="board__timezone">
        Periods roll over at midnight, {state.timezone}. Everyone sees the same board.
      </p>

      {actionError && (
        <p className="panel panel--error" role="alert">
          {actionError}
        </p>
      )}

      {/* Announces the result of an action for screen reader users, who would
          otherwise get no feedback from a card silently changing colour. */}
      <p className="visually-hidden" role="status" aria-live="polite">
        {announcement}
      </p>

      {grouped.length === 0 ? (
        cards.length > 0 ? (
          // The board has cards but the filters hid them all, so offer a way back
          // rather than the "add the first card" prompt.
          <div className="panel">
            <p>
              {query.trim() !== ''
                ? `No cards match “${query.trim()}”.`
                : 'No cards match these filters.'}
            </p>
            <button
              className="button button--quiet"
              type="button"
              onClick={() => {
                setQuery('');
                setCadenceFilter('all');
              }}
            >
              Clear filters
            </button>
          </div>
        ) : (
          <div className="panel">
            <p>No cards yet.</p>
            {!state.board.archived && (
              <button className="button button--primary" type="button" onClick={() => setCreating(true)}>
                Add the first card
              </button>
            )}
          </div>
        )
      ) : (
        grouped.map((group) => (
          <section className="board__group" key={group.cadence} aria-labelledby={`group-${group.cadence}`}>
            <h3 className="board__group-title" id={`group-${group.cadence}`}>
              {cadenceLabel(group.cadence)}
              <span className="board__group-count">{group.cards.length}</span>
            </h3>
            <ul className="card-grid">
              {group.cards.map((card) => (
                <CardTile
                  key={card.id}
                  card={card}
                  busy={busyCardId === card.id}
                  onOpen={(c) => setOpenCardId(c.id)}
                  onStart={(c) => void run(c, 'start')}
                  onComplete={(c) => void run(c, 'complete')}
                  onReopen={(c) => void run(c, 'reopen')}
                />
              ))}
            </ul>
          </section>
        ))
      )}

      {openCard && (
        <CardDetail
          card={openCard}
          busy={busyCardId === openCard.id}
          canRestore={isAdmin}
          onClose={() => setOpenCardId(null)}
          onStart={(card, notes) => void run(card, 'start', notes)}
          onComplete={(card, notes) => {
            void run(card, 'complete', notes);
            setOpenCardId(null);
          }}
          onReopen={(card, notes) => void run(card, 'reopen', notes)}
          onArchive={(card) => void archive(card)}
          onRestore={(card) => void restore(card)}
        />
      )}

      {creating && (
        <NewCardDialog
          boardId={boardId}
          nextSortOrder={cards.length + 1}
          onClose={() => setCreating(false)}
          onCreated={() => {
            setCreating(false);
            setAnnouncement('Card created.');
            refresh();
          }}
        />
      )}
    </div>
  );
}
