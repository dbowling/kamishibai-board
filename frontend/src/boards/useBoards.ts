import { useCallback, useEffect, useState } from 'react';
import { api, errorMessage } from '../lib/api';
import type { BoardRecord, TeamRecord } from '../lib/types';

export interface TeamWithBoards {
  team: TeamRecord;
  boards: BoardRecord[];
}

interface UseBoards {
  teams: TeamWithBoards[];
  loading: boolean;
  error: string | null;
  refresh: () => void;
}

/**
 * Loads the teams the signed-in user can see, each with its active boards.
 *
 * Scoping is the server's job: the teams list rule only returns teams the caller
 * belongs to (or everything, for an admin), so this does not filter by
 * membership itself. Doing it here as well would be a second, weaker copy of the
 * rule.
 */
export function useBoards(): UseBoards {
  const [teams, setTeams] = useState<TeamWithBoards[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  const refresh = useCallback(() => setNonce((n) => n + 1), []);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);

    (async () => {
      try {
        const teamRecords = await api.teams();
        const active = teamRecords.filter((team) => !team.archived_at);

        const withBoards = await Promise.all(
          active.map(async (team) => ({
            team,
            boards: await api.boards(team.id),
          })),
        );

        if (!cancelled) {
          setTeams(withBoards);
          setError(null);
        }
      } catch (cause) {
        if (!cancelled) {
          setError(errorMessage(cause, 'Could not load your teams.'));
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [nonce]);

  return { teams, loading, error, refresh };
}
