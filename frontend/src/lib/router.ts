import { useCallback, useEffect, useState } from 'react';

/**
 * A very small router.
 *
 * The app has three destinations, so a routing library would be more
 * configuration than code. This keeps real URLs (the backend serves index.html
 * for unmatched paths, so deep links and refreshes work) without the dependency.
 */

export type Route =
  | { name: 'home' }
  | { name: 'board'; boardId: string }
  | { name: 'report'; boardId: string }
  | { name: 'notFound'; path: string };

export function parseRoute(pathname: string): Route {
  const segments = pathname.split('/').filter(Boolean);

  if (segments.length === 0) {
    return { name: 'home' };
  }

  if (segments[0] === 'boards' && segments[1]) {
    const boardId = decodeURIComponent(segments[1]);

    if (segments.length === 2) {
      return { name: 'board', boardId };
    }
    if (segments.length === 3 && segments[2] === 'report') {
      return { name: 'report', boardId };
    }
  }

  return { name: 'notFound', path: pathname };
}

export function boardPath(boardId: string): string {
  return `/boards/${encodeURIComponent(boardId)}`;
}

export function reportPath(boardId: string): string {
  return `${boardPath(boardId)}/report`;
}

/** The board id the current route refers to, if any. */
export function routeBoardId(route: Route): string | null {
  return route.name === 'board' || route.name === 'report' ? route.boardId : null;
}

/**
 * Current route plus a navigate function.
 *
 * Listens for popstate so the browser's back and forward buttons behave, which is
 * the main thing people notice when an app rolls its own routing.
 */
export function useRouter(): { route: Route; navigate: (path: string, replace?: boolean) => void } {
  const [pathname, setPathname] = useState(() => window.location.pathname);

  useEffect(() => {
    const onPopState = () => setPathname(window.location.pathname);
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  const navigate = useCallback((path: string, replace = false) => {
    if (path === window.location.pathname) return;

    if (replace) {
      window.history.replaceState(null, '', path);
    } else {
      window.history.pushState(null, '', path);
    }
    setPathname(path);
  }, []);

  return { route: parseRoute(pathname), navigate };
}
