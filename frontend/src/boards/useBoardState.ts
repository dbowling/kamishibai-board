import { useCallback, useEffect, useRef, useState } from 'react';
import { api, errorMessage } from '../lib/api';
import { msUntilNextBoundary } from '../lib/format';
import { pb } from '../lib/pocketbase';
import type { BoardState } from '../lib/types';

interface UseBoardState {
  state: BoardState | null;
  error: string | null;
  loading: boolean;
  refresh: () => void;
  includeArchived: boolean;
  setIncludeArchived: (value: boolean) => void;
}

/**
 * Loads a board and keeps it current.
 *
 * Two things keep the board honest without anybody pressing reload:
 *
 *   - A realtime subscription to the occurrences collection, so a teammate
 *     completing a card updates everyone's board. This is what makes the shared
 *     board actually shared rather than five private copies.
 *
 *   - A timer set to the next period boundary. Cards flip because the period key
 *     rolls over on the server, and nothing pushes an event when that happens
 *     (that is the whole point of the lazy model: no write occurs). So the client
 *     works out when the soonest boundary is and refetches then.
 */
export function useBoardState(boardId: string | null): UseBoardState {
  const [state, setState] = useState<BoardState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [includeArchived, setIncludeArchived] = useState(false);
  const [nonce, setNonce] = useState(0);

  const refresh = useCallback(() => setNonce((n) => n + 1), []);

  // Tracks the latest request so a slow response for a board the user has since
  // navigated away from cannot overwrite the current one.
  const requestRef = useRef(0);

  useEffect(() => {
    if (!boardId) {
      setState(null);
      return;
    }

    const request = ++requestRef.current;
    let cancelled = false;

    setLoading(true);
    api
      .boardState(boardId, { includeArchived })
      .then((next) => {
        if (cancelled || request !== requestRef.current) return;
        setState(next);
        setError(null);
      })
      .catch((cause: unknown) => {
        if (cancelled || request !== requestRef.current) return;
        setState(null);
        setError(errorMessage(cause, 'Could not load this board.'));
      })
      .finally(() => {
        if (!cancelled && request === requestRef.current) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [boardId, includeArchived, nonce]);

  // Realtime: refetch when any occurrence on this board changes.
  useEffect(() => {
    if (!boardId) return;

    let disposed = false;
    let unsubscribe: (() => void) | undefined;

    pb.collection('occurrences')
      .subscribe('*', (event) => {
        const record = event.record as unknown as { board?: string } | undefined;
        // Occurrences carry a denormalised board id, so filtering is a field read
        // rather than a lookup.
        if (record?.board === boardId) {
          refresh();
        }
      })
      .then((dispose) => {
        if (disposed) {
          void dispose();
          return;
        }
        unsubscribe = dispose;
      })
      .catch(() => {
        // A failed subscription degrades to manual refresh, which is not worth
        // interrupting the user over.
      });

    return () => {
      disposed = true;
      if (unsubscribe) void unsubscribe();
    };
  }, [boardId, refresh]);

  // Flip the board when a period rolls over.
  useEffect(() => {
    if (!state) return;

    const periods = Object.values(state.periods).filter((p) => p !== undefined);
    const ms = msUntilNextBoundary(periods);
    if (ms === null) return;

    // A second of slack so the server has definitely crossed the boundary,
    // otherwise a clock skew of milliseconds would refetch the period that is
    // just about to end.
    const timer = window.setTimeout(refresh, ms + 1_000);
    return () => window.clearTimeout(timer);
  }, [state, refresh]);

  return { state, error, loading, refresh, includeArchived, setIncludeArchived };
}
