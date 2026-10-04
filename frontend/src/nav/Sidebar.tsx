import type { ArchivedNav, TeamWithBoards } from '../boards/useBoards';
import { boardPath } from '../lib/router';
import { SidebarEditor } from './SidebarEditor';

interface SidebarProps {
  teams: TeamWithBoards[];
  archived: ArchivedNav;
  loading: boolean;
  error: string | null;
  currentBoardId: string | null;
  navigate: (path: string, replace?: boolean) => void;
  /** Only admins get the Edit toggle. */
  isAdmin: boolean;
  editing: boolean;
  onEditingChange: (editing: boolean) => void;
  /** Called after the editor changed something, so the data is refetched. */
  onChanged: () => void;
}

/**
 * The left-hand list of teams and boards.
 *
 * Everyone sees the read-only list of links. Admins also get an Edit toggle that
 * swaps the list for SidebarEditor. The server enforces the permissions; hiding
 * the button just avoids offering actions that would be refused.
 */
export function Sidebar({
  teams,
  archived,
  loading,
  error,
  currentBoardId,
  navigate,
  isAdmin,
  editing,
  onEditingChange,
  onChanged,
}: SidebarProps) {
  const showEditor = isAdmin && editing;

  return (
    <nav className="sidebar" aria-label="Boards">
      {isAdmin && (
        <div className="sidebar__toolbar">
          <button
            className="button button--quiet"
            type="button"
            aria-pressed={editing}
            onClick={() => onEditingChange(!editing)}
          >
            {editing ? 'Done' : 'Edit'}
          </button>
        </div>
      )}

      {loading && <p className="sidebar__note">Loading…</p>}
      {error && (
        <p className="sidebar__note" role="alert">
          {error}
        </p>
      )}

      {showEditor && !loading && !error && (
        <SidebarEditor teams={teams} archived={archived} onChanged={onChanged} />
      )}

      {!showEditor && !loading && teams.length === 0 && !error && (
        <p className="sidebar__note">
          You are not on any team yet. An administrator can add you to one.
        </p>
      )}

      {!showEditor &&
        teams.map(({ team, boards }) => (
          <section className="sidebar__team" key={team.id}>
            <h2 className="sidebar__team-name">{team.name}</h2>

            {boards.length === 0 ? (
              <p className="sidebar__note">No boards yet.</p>
            ) : (
              <ul className="sidebar__boards">
                {boards.map((board) => {
                  const active = board.id === currentBoardId;
                  return (
                    <li key={board.id}>
                      <a
                        className={active ? 'sidebar__link is-active' : 'sidebar__link'}
                        href={boardPath(board.id)}
                        aria-current={active ? 'page' : undefined}
                        onClick={(event) => {
                          // Keep it a real link so middle-click and
                          // open-in-new-tab behave, but navigate in-app on a
                          // plain left click.
                          if (event.metaKey || event.ctrlKey || event.shiftKey) return;
                          event.preventDefault();
                          navigate(boardPath(board.id));
                        }}
                      >
                        {board.name}
                      </a>
                    </li>
                  );
                })}
              </ul>
            )}
          </section>
        ))}
    </nav>
  );
}
