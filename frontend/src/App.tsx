import { useEffect, useState } from 'react';
import { useAuth } from './auth/AuthProvider';
import { LoginPage } from './auth/LoginPage';
import { TimeZoneDialog } from './auth/TimeZoneDialog';
import { BoardPage } from './boards/BoardPage';
import { useBoards } from './boards/useBoards';
import { Sidebar } from './nav/Sidebar';
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
  // Admin-only: whether the sidebar is in edit mode. Archived teams and boards are
  // only fetched while it is, so everyone else's requests are unchanged.
  const [editing, setEditing] = useState(false);
  const [choosingZone, setChoosingZone] = useState(false);
  const { teams, archived, loading, error, refresh } = useBoards(isAdmin && editing);
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

  // An admin can archive or move away the board they are looking at. Rather than
  // leave the page showing a board that is no longer listed, go home, which then
  // lands on the first board that is.
  useEffect(() => {
    if (!isAdmin || !editing || loading || !currentBoardId) return;
    if (!allBoards.some((board) => board.id === currentBoardId)) navigate('/', true);
  }, [isAdmin, editing, loading, currentBoardId, allBoards, navigate]);

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
          <button
            className="button button--quiet"
            type="button"
            onClick={() => setChoosingZone(true)}
          >
            Time zone
          </button>
          <button className="button button--quiet" type="button" onClick={signOut}>
            Sign out
          </button>
        </div>
      </header>

      {choosingZone && user && (
        <TimeZoneDialog
          userId={user.id}
          current={user.timezone ?? ''}
          onClose={() => setChoosingZone(false)}
        />
      )}

      <div className="layout">
        <Sidebar
          teams={teams}
          archived={archived}
          loading={loading}
          error={error}
          currentBoardId={currentBoardId}
          navigate={navigate}
          isAdmin={isAdmin}
          editing={editing}
          onEditingChange={setEditing}
          onChanged={refresh}
        />

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
