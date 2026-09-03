import { useEffect } from 'react';
import { useAuth } from './auth/AuthProvider';
import { LoginPage } from './auth/LoginPage';
import { BoardPage } from './boards/BoardPage';
import { useBoards } from './boards/useBoards';
import { ReportPage } from './reports/ReportPage';
import { boardPath, reportPath, routeBoardId, useRouter } from './lib/router';

export function App() {
  const { user } = useAuth();

  if (!user) {
    return <LoginPage />;
  }
  return <Shell />;
}

function Shell() {
  const { user, signOut, isAdmin } = useAuth();
  const { teams, loading, error } = useBoards();
  const { route, navigate } = useRouter();

  const currentBoardId = routeBoardId(route);
  const allBoards = teams.flatMap((entry) => entry.boards);

  // Land on a board rather than an empty screen. Replace rather than push, so the
  // back button does not bounce through the redirect.
  useEffect(() => {
    if (route.name !== 'home' || allBoards.length === 0) return;
    const first = allBoards[0];
    if (first) navigate(boardPath(first.id), true);
  }, [route.name, allBoards, navigate]);

  return (
    <div className="shell">
      <a className="skip-link" href="#main">
        Skip to content
      </a>

      <header className="topbar">
        <div className="topbar__brand">
          <span className="topbar__mark" aria-hidden="true">
            紙
          </span>
          <span>
            Kamishibai
            <small>Triage board</small>
          </span>
        </div>

        <div className="topbar__user">
          <span className="topbar__name">
            {user?.name || user?.email}
            {isAdmin && <span className="badge badge--admin">admin</span>}
          </span>
          <button className="button button--quiet" type="button" onClick={signOut}>
            Sign out
          </button>
        </div>
      </header>

      <div className="layout">
        <nav className="sidebar" aria-label="Boards">
          {loading && <p className="sidebar__note">Loading…</p>}
          {error && (
            <p className="sidebar__note" role="alert">
              {error}
            </p>
          )}

          {!loading && teams.length === 0 && !error && (
            <p className="sidebar__note">
              You are not on any team yet. An administrator can add you to one.
            </p>
          )}

          {teams.map(({ team, boards }) => (
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

        <main className="main" id="main">
          {currentBoardId && (
            <div className="tabs" role="tablist" aria-label="Board views">
              <a
                className={route.name === 'board' ? 'tab is-active' : 'tab'}
                role="tab"
                aria-selected={route.name === 'board'}
                href={boardPath(currentBoardId)}
                onClick={(event) => {
                  if (event.metaKey || event.ctrlKey || event.shiftKey) return;
                  event.preventDefault();
                  navigate(boardPath(currentBoardId));
                }}
              >
                Board
              </a>
              <a
                className={route.name === 'report' ? 'tab is-active' : 'tab'}
                role="tab"
                aria-selected={route.name === 'report'}
                href={reportPath(currentBoardId)}
                onClick={(event) => {
                  if (event.metaKey || event.ctrlKey || event.shiftKey) return;
                  event.preventDefault();
                  navigate(reportPath(currentBoardId));
                }}
              >
                Reporting
              </a>
            </div>
          )}

          {route.name === 'board' && <BoardPage boardId={route.boardId} />}
          {route.name === 'report' && <ReportPage boardId={route.boardId} />}

          {route.name === 'home' && allBoards.length === 0 && !loading && (
            <div className="panel">
              <h2>Nothing to show yet</h2>
              <p>
                Once you are a member of a team with a board, it will appear in the list on the
                left.
              </p>
            </div>
          )}

          {route.name === 'notFound' && (
            <div className="panel">
              <h2>Page not found</h2>
              <p>
                <code>{route.path}</code> does not match anything.
              </p>
              <button className="button button--primary" type="button" onClick={() => navigate('/')}>
                Go to a board
              </button>
            </div>
          )}
        </main>
      </div>
    </div>
  );
}
