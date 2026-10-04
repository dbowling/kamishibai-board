import { useEffect, useState } from 'react';
import { api, errorMessage } from '../lib/api';
import type { ActivityReport } from '../lib/types';
import { ActivityHeatmap } from './ActivityHeatmap';

interface ActivityPanelProps {
  boardId: string;
}

/**
 * The completions heatmap and everything around it.
 *
 * It owns its own fetch so the report page only has to mount it. The heatmap is
 * independent of the cadence and period selectors above it (it always shows every
 * day on record), so it is deliberately not refetched when those change.
 */
export function ActivityPanel({ boardId }: ActivityPanelProps) {
  const [report, setReport] = useState<ActivityReport | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);

    api
      .activity(boardId)
      .then((next) => {
        if (!cancelled) {
          setReport(next);
          setError(null);
        }
      })
      .catch((cause: unknown) => {
        if (!cancelled) {
          setReport(null);
          setError(errorMessage(cause, 'Could not load the activity heatmap.'));
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [boardId]);

  return (
    <section className="panel" aria-labelledby="activity-heading">
      <h3 id="activity-heading" className="report__section-title">
        Completions by day
      </h3>

      {error && (
        <p className="panel panel--error" role="alert">
          {error}
        </p>
      )}

      {loading && !report && <p className="report__note">Loading activity…</p>}

      {report && (
        <>
          <ActivityHeatmap report={report} />
          <p className="report__note">
            Each square is one calendar day in the board's timezone ({report.timezone}). Switch
            between all completions and a single cadence with the toggles under the chart.
          </p>
        </>
      )}
    </section>
  );
}
