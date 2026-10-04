import { useCallback, useEffect, useState } from 'react';
import { api, errorMessage } from '../lib/api';
import type { BoardRecord, TeamRecord } from '../lib/types';

export interface TeamWithBoards {
  team: TeamRecord;
  boards: BoardRecord[];
}

/** What the sidebar editor lists under "Archived". */
export interface ArchivedNav {
  /** Archived teams. Their boards are not listed: they come back with the team. */
  teams: TeamRecord[];
  /** Archived boards that belong to a team that is still active. */
  boards: { board: BoardRecord; team: TeamRecord }[];
}

interface UseBoards {
  teams: TeamWithBoards[];
  /** Always empty unless `includeArchived` was requested. */
  archived: ArchivedNav;
  loading: boolean;
  error: string | null;
  refresh: () => void;
}

const NO_ARCHIVED: ArchivedNav = { teams: [], boards: [] };

/**
 * Loads the teams the signed-in user can see, each with its active boards.
 *
 * Scoping is the server's job: the teams list rule only returns teams the caller
 * belongs to (or everything, for an admin), so this does not filter by
 * membership itself. Doing it here as well would be a second, weaker copy of the
 * rule.
 *
 * With `includeArchived` (admins editing the sidebar) it also returns the
 * archived teams and boards so they can be restored. Everyone else gets exactly
 * the data they always did.
 */
export function useBoards(includeArchived = false): UseBoards {
  const [teams, setTeams] = useState<TeamWithBoards[]>([]);
  const [archived, setArchived] = useState<ArchivedNav>(NO_ARCHIVED);
  // True only until the first answer. A refresh keeps showing the current data
  // rather than flashing "Loading…" over a sidebar the user is working in.
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  const refresh = useCallback(() => setNonce((n) => n + 1), []);

  useEffect(() => {
    let cancelled = false;

    (async () => {
      try {
        const teamRecords = await api.teams();
        const active = teamRecords.filter((team) => !team.archived_at);

        const loaded = await Promise.all(
          active.map(async (team) => ({
            team,
            boards: await api.boards(team.id, includeArchived ? { includeArchived: true } : undefined),
          })),
        );

        const withBoards = loaded.map(({ team, boards }) => ({
          team,
          boards: boards.filter((board) => !board.archived_at),
        }));

        if (!cancelled) {
          setTeams(withBoards);
          setArchived(
            includeArchived
              ? {
                  teams: teamRecords.filter((team) => team.archived_at),
                  boards: loaded.flatMap(({ team, boards }) =>
                    boards.filter((board) => board.archived_at).map((board) => ({ board, team })),
                  ),
                }
              : NO_ARCHIVED,
          );
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
  }, [nonce, includeArchived]);

  return { teams, archived, loading, error, refresh };
}
