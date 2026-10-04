import { useEffect, useMemo, useState } from 'react';
import { api, errorMessage } from '../lib/api';
import { cadenceLabel, percent, periodKeyLabel, periodKeyShortLabel } from '../lib/format';
import { CADENCES } from '../lib/types';
import type { Cadence, Report } from '../lib/types';
import { ActivityPanel } from './ActivityPanel';

interface ReportPageProps {
  boardId: string;
}

/**
 * Completion history for a board.
 *
 * The per-period and per-task figures are charted as labelled tables with
 * proportional bars rather than a canvas: the numbers stay readable to a screen
 * reader and need no charting library. The visual bar is decorative and the figure
 * next to it is the real content.
 *
 * The one exception is the completions-by-day heatmap, which is a calendar grid
 * that a table would show badly, so it is drawn by Heat.js (see ActivityPanel). It
 * is isolated to that panel and carries a written summary for screen readers.
 */
export function ReportPage({ boardId }: ReportPageProps) {
  const [cadence, setCadence] = useState<Cadence>('daily');
  const [periods, setPeriods] = useState(12);
  const [report, setReport] = useState<Report | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);

    api
      .report(boardId, cadence, periods)
      .then((next) => {
        if (!cancelled) {
          setReport(next);
          setError(null);
        }
      })
      .catch((cause: unknown) => {
        if (!cancelled) {
          setReport(null);
          setError(errorMessage(cause, 'Could not load the report.'));
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [boardId, cadence, periods]);

  // The open period is shown alongside the closed ones, but flagged, because its
  // numbers can still change.
  const series = useMemo(() => {
    if (!report) return [];
    return report.current ? [...report.series, report.current] : report.series;
  }, [report]);

  return (
    <div className="report">
      <header className="report__header">
        <h2 className="report__title">
          Reporting{report ? ` · ${report.board.name}` : ''}
        </h2>

        <div className="report__controls">
          <label className="field field--inline">
            <span className="field__label">Cadence</span>
            <select
              className="field__input"
              value={cadence}
              onChange={(event) => setCadence(event.target.value as Cadence)}
            >
              {CADENCES.map((option) => (
                <option key={option} value={option}>
                  {cadenceLabel(option)}
                </option>
              ))}
            </select>
          </label>

          <label className="field field--inline">
            <span className="field__label">Periods</span>
            <select
              className="field__input"
              value={periods}
              onChange={(event) => setPeriods(Number(event.target.value))}
            >
              {[6, 12, 24, 52].map((option) => (
                <option key={option} value={option}>
                  {option}
                </option>
              ))}
            </select>
          </label>
        </div>
      </header>

      {error && (
        <p className="panel panel--error" role="alert">
          {error}
        </p>
      )}

      {/* Outside the conditional below: the heatmap does not depend on the
          selected cadence, so it must show even when that cadence has no rows. */}
      <ActivityPanel boardId={boardId} />

      {loading && !report && <p className="panel">Loading the report…</p>}

      {report && series.length === 0 && (
        <p className="panel">
          Nothing recorded yet for {cadenceLabel(cadence).toLowerCase()} cards on this board.
        </p>
      )}

      {report && series.length > 0 && (
        <>
          <section className="panel" aria-labelledby="totals-heading">
            <h3 id="totals-heading" className="report__section-title">
              Across the window
            </h3>
            <dl className="report__totals">
              <div>
                <dt>Completion</dt>
                <dd className="report__big">{percent(report.totals.completionRate)}</dd>
              </div>
              <div>
                <dt>Completed</dt>
                <dd>{report.totals.done}</dd>
              </div>
              <div>
                <dt>Expected</dt>
                <dd>{report.totals.expected}</dd>
              </div>
              <div>
                <dt>Periods</dt>
                <dd>{report.totals.periods}</dd>
              </div>
            </dl>
          </section>

          <section className="panel" aria-labelledby="by-period-heading">
            <h3 id="by-period-heading" className="report__section-title">
              By period
            </h3>
            <table className="table">
              <caption className="visually-hidden">
                Completion rate per {cadenceLabel(cadence).toLowerCase()} period
              </caption>
              <thead>
                <tr>
                  <th scope="col">Period</th>
                  <th scope="col">Complete</th>
                  <th scope="col">Done</th>
                  <th scope="col">In progress</th>
                  <th scope="col">Missed</th>
                  <th scope="col">Cards</th>
                </tr>
              </thead>
              <tbody>
                {series.map((point) => (
                  <tr key={point.periodKey} className={point.source === 'live' ? 'is-live' : ''}>
                    <th scope="row" title={periodKeyLabel(point.periodKey)}>
                      {periodKeyShortLabel(point.periodKey)}
                      {point.source === 'live' && (
                        <span className="badge badge--live" title="Still open, or not yet snapshotted">
                          live
                        </span>
                      )}
                    </th>
                    <td>
                      <span className="bar" aria-hidden="true">
                        <span
                          className="bar__fill"
                          style={{ width: `${Math.round(point.completionRate * 100)}%` }}
                        />
                      </span>
                      <span className="bar__value">{percent(point.completionRate)}</span>
                    </td>
                    <td>{point.done}</td>
                    <td>{point.inProgress}</td>
                    <td>{point.notStarted}</td>
                    <td>{point.total}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            <p className="report__note">
              Rows marked <strong>live</strong> were computed on request. Closed periods are read
              from frozen snapshots and cannot change.
            </p>
          </section>

          <section className="panel" aria-labelledby="by-card-heading">
            <h3 id="by-card-heading" className="report__section-title">
              By task
            </h3>
            <table className="table">
              <caption className="visually-hidden">Completion rate per card</caption>
              <thead>
                <tr>
                  <th scope="col">Card</th>
                  <th scope="col">Complete</th>
                  <th scope="col">Done</th>
                  <th scope="col">Expected</th>
                </tr>
              </thead>
              <tbody>
                {report.cards.map((card) => (
                  <tr key={card.cardId}>
                    <th scope="row">
                      {card.title}
                      {card.archived && <span className="badge">archived</span>}
                    </th>
                    <td>
                      <span className="bar" aria-hidden="true">
                        <span
                          className="bar__fill"
                          style={{ width: `${Math.round(card.completionRate * 100)}%` }}
                        />
                      </span>
                      <span className="bar__value">{percent(card.completionRate)}</span>
                    </td>
                    <td>{card.done}</td>
                    <td>{card.expected}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            <p className="report__note">
              Least reliable first. A card only counts for periods it existed during, so adding a
              card does not make past months look worse.
            </p>
          </section>
        </>
      )}
    </div>
  );
}
