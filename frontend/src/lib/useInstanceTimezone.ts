import { useEffect, useState } from 'react';
import { api } from './api';

/**
 * The instance default zone, which a team without its own inherits.
 *
 * The server is the only thing that knows it (it comes from KAMISHIBAI_TIMEZONE),
 * and it reports it as the `timezone` of the team-less periods call. Null until
 * that arrives, or if it fails: callers fall back to wording that does not name a
 * zone rather than blocking on it.
 */
export function useInstanceTimezone(): string | null {
  const [zone, setZone] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    api
      .currentPeriods()
      .then((result) => {
        if (!cancelled) setZone(result.timezone);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  return zone;
}
