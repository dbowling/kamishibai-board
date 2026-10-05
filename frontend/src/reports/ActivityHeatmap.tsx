import { useEffect, useRef } from 'react';
import { cadenceLabel } from '../lib/format';
import { CADENCES } from '../lib/types';
import type { ActivityReport } from '../lib/types';
import { heat } from './heatjs';
import type { HeatBindingOptions } from './heatjs';

interface ActivityHeatmapProps {
  report: ActivityReport;
}

/** The type that sums every cadence. Heat.js needs a name for it. */
const ALL = 'All';

/**
 * How many completions a day needs to reach each colour step.
 *
 * Heat.js's built-in thresholds are 10, 15, 20 and 25, which suit page-view
 * counts. A board finishes a handful of cards a day, so with the defaults every
 * square would stay blank and a day with work on it would look the same as an
 * idle one. These start at one so any completion shows, and top out where a busy
 * day is plainly busier than an ordinary one.
 */
const COLOR_RANGES = [1, 3, 5, 8].map((minimum, index) => ({
  id: `0${index + 1}`,
  name: `Day Color ${index + 1}`,
  minimum,
  cssClassName: `day-color-${index + 1}`,
  tooltipText: `${minimum} or more completions`,
  visible: true,
}));

/**
 * Completions per calendar day, drawn by Heat.js.
 *
 * Pure presentation: the server has already bucketed each completion into a day
 * in the owning team's timezone, so nothing here re-buckets timestamps or does date
 * arithmetic beyond turning a YYYY-MM-DD string into a Date.
 *
 * Heat.js draws plain divs with hover tooltips and no text alternative, so the
 * chart is hidden from assistive technology and the summary sentence underneath
 * is the real content.
 */
export function ActivityHeatmap({ report }: ActivityHeatmapProps) {
  const ref = useRef<HTMLDivElement>(null);

  // Heat.js keys every instance on the element's id, so it must have one before
  // render. PocketBase ids are alphanumeric, which makes this a valid id.
  const id = `activity-heatmap-${report.board.id}`;

  useEffect(() => {
    const element = ref.current;
    if (!element) return;

    // A chart that fails to draw must never take the report page down with it.
    // The summary paragraph is still there, so log and carry on.
    let rendered = false;
    try {
      const api = heat();
      api.render(element, buildOptions(report));
      rendered = true;

      // Every type has to exist before any date is added to it: updateDate
      // ignores a type it has not heard of. Refreshes are deferred to the end so
      // the chart is drawn once, not once per data point.
      const types = typesFor(report);
      for (const type of types) {
        api.addType(id, type, false);
      }

      const { all, byCadence } = aggregate(report);
      for (const [date, count] of all) {
        api.updateDate(id, toLocalDate(date), count, ALL, false);
      }
      for (const [label, counts] of byCadence) {
        for (const [date, count] of counts) {
          api.updateDate(id, toLocalDate(date), count, label, false);
        }
      }

      api.switchType(id, ALL);
      api.refresh(id);
    } catch (cause) {
      console.error('Could not draw the activity heatmap.', cause);
    }

    return () => {
      // React StrictMode runs effects twice in development, and the report can be
      // replaced while mounted. Heat.js restores the element to its pre-render
      // state on destroy, so render, destroy, render is safe. If render itself
      // threw there may be nothing to destroy.
      if (!rendered) return;
      try {
        heat().destroy(id);
      } catch (cause) {
        console.error('Could not tear down the activity heatmap.', cause);
      }
    };
  }, [report, id]);

  return (
    <div className="heatmap">
      <div id={id} ref={ref} aria-hidden="true" />
      <p className="report__note">{summarise(report)}</p>
    </div>
  );
}

/** One sentence that carries what the chart shows, for people who cannot see it. */
export function summarise(report: ActivityReport): string {
  if (report.total === 0) return 'No completions recorded yet.';

  // `days` has one row per (date, cadence), so count distinct dates.
  const dayCount = new Set(report.days.map((day) => day.date)).size;
  const completions = report.total === 1 ? 'completion' : 'completions';
  const days = dayCount === 1 ? 'day' : 'days';
  return `${report.total} ${completions} across ${dayCount} ${days}.`;
}

function buildOptions(report: ActivityReport): HeatBindingOptions {
  return {
    defaultYear: defaultYear(report),
    defaultView: 'map',
    showOnlyDataForYearsAvailable: true,
    // One board, one chart: the side menu is for juggling several instances.
    sideMenu: { enabled: false },
    title: {
      text: 'Completions',
      showYearSelectionDropDown: true,
      showCurrentYearButton: true,
      // The server owns the data, so Heat.js's own refresh, import, export,
      // configuration and clear controls are all off: each would let the chart
      // drift from what the server said, or leave the chart in a state the page
      // cannot reproduce.
      showRefreshButton: false,
      showExportButton: false,
      showImportButton: false,
      showConfigurationButton: false,
      showClearButton: false,
    },
    guide: {
      enabled: true,
      colorRangeTogglesEnabled: true,
      showLessAndMoreLabels: true,
      // Types are the cadences; people must not be able to invent or delete them.
      allowTypeAdding: false,
      allowTypeRemoving: false,
    },
    yearlyStatistics: { enabled: true },
    colorRanges: COLOR_RANGES,
    views: {
      map: { enabled: true, showMonthNames: true, showDayNames: true, highlightCurrentDay: true },
      chart: { enabled: true },
      days: { enabled: true },
      months: { enabled: true },
      line: { enabled: false },
      colorRanges: { enabled: false },
    },
  };
}

/**
 * The year to open on: the latest one with data, so the chart is never blank
 * when the board has history.
 *
 * It is taken from the data rather than the clock so the result is deterministic
 * (and testable). The clock is only the fallback for an empty board.
 */
function defaultYear(report: ActivityReport): number {
  let latest = 0;
  for (const day of report.days) {
    const year = Number(day.date.slice(0, 4));
    if (Number.isFinite(year) && year > latest) latest = year;
  }
  return latest > 0 ? latest : new Date().getFullYear();
}

/**
 * The types to register: "All", then one per cadence that appears in the data,
 * in the app's usual cadence order. Heat.js only shows its type toggles when more
 * than one type exists, so "All" is always present.
 */
function typesFor(report: ActivityReport): string[] {
  const present = new Set(report.days.map((day) => day.cadence));
  return [ALL, ...CADENCES.filter((cadence) => present.has(cadence)).map(cadenceLabel)];
}

/** Per-date totals across every cadence, and per-date counts for each cadence label. */
function aggregate(report: ActivityReport) {
  const all = new Map<string, number>();
  const byCadence = new Map<string, Map<string, number>>();

  for (const day of report.days) {
    all.set(day.date, (all.get(day.date) ?? 0) + day.completed);

    const label = cadenceLabel(day.cadence);
    const counts = byCadence.get(label) ?? new Map<string, number>();
    counts.set(day.date, (counts.get(day.date) ?? 0) + day.completed);
    byCadence.set(label, counts);
  }

  return { all, byCadence };
}

/**
 * A YYYY-MM-DD string as a local-time Date.
 *
 * Heat.js buckets with the local getFullYear/getMonth/getDate. `new Date('2026-09-03')`
 * is UTC midnight, which is still 2 September anywhere west of Greenwich, so the
 * square would land on the wrong day. Building from parts keeps the calendar date
 * the server sent.
 */
function toLocalDate(date: string): Date {
  const [year = 0, month = 1, day = 1] = date.split('-').map(Number);
  return new Date(year, month - 1, day);
}
