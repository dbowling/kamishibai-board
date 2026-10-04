import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ActivityHeatmap, summarise } from './ActivityHeatmap';
import type { ActivityDay, ActivityReport } from '../lib/types';

// The year comes from the data, not the clock: the heatmap opens on the latest
// year that has completions, so these tests never depend on today's date.
const DAYS: ActivityDay[] = [
  { date: '2026-09-03', cadence: 'daily', completed: 3 },
  { date: '2026-09-03', cadence: 'weekly', completed: 1 },
  { date: '2026-09-04', cadence: 'daily', completed: 2 },
];

function makeReport(days: ActivityDay[] = DAYS): ActivityReport {
  return {
    board: { id: 'board1', teamId: 'team1', name: 'Triage', description: '', archived: false },
    timezone: 'America/New_York',
    days,
    total: days.reduce((sum, day) => sum + day.completed, 0),
  };
}

const ELEMENT_ID = 'activity-heatmap-board1';

function cell(container: HTMLElement, heatDate: string): Element {
  const found = container.querySelector(`[data-heat-js-map-date="${heatDate}"]`);
  if (!found) throw new Error(`no map cell for ${heatDate}`);
  return found;
}

describe('ActivityHeatmap', () => {
  it('renders a Heat.js instance for the board', () => {
    const { container } = render(<ActivityHeatmap report={makeReport()} />);

    expect(window.$heat?.getIds()).toContain(ELEMENT_ID);
    expect(container.querySelector(`#${ELEMENT_ID}`)).toHaveClass('heat-js');
    expect(screen.getByText('Completions')).toBeInTheDocument();
  });

  it('plots the data', () => {
    const { container } = render(<ActivityHeatmap report={makeReport()} />);

    // A full year grid: one cell per day, so 365 or 366 depending on the year.
    const cells = container.querySelectorAll('[data-heat-js-map-date]');
    expect([365, 366]).toContain(cells.length);

    // Heat.js identifies a day as DD-MM-YYYY. The "All" type sums the cadences:
    // 3 daily + 1 weekly on the 3rd is 4, and 2 daily on the 4th. Landing on those
    // exact squares (rather than a day either side) is what proves the dates were
    // built as local calendar days and not shifted by UTC.
    expect(cell(container, '03-09-2026')).toHaveAttribute('data-heat-js-map-minimum', '3');
    expect(cell(container, '03-09-2026')).toHaveClass('day-color-2');
    expect(cell(container, '04-09-2026')).toHaveClass('day-color-1');
    expect(cell(container, '02-09-2026').className).not.toMatch(/day-color/);
    expect(cell(container, '05-09-2026').className).not.toMatch(/day-color/);

    expect(screen.getByText('6 completions across 2 days.')).toBeInTheDocument();
  });

  it('offers All and one toggle per cadence in the data', () => {
    const { container } = render(<ActivityHeatmap report={makeReport()} />);

    // The chart is aria-hidden on purpose (Heat.js draws unlabelled divs; the
    // summary sentence is the accessible content), so getByRole cannot see its
    // toggles. Query the markup Heat.js produces instead.
    const labels = Array.from(container.querySelectorAll('.map-types .type')).map(
      (button) => button.textContent,
    );
    expect(labels).toEqual(expect.arrayContaining(['All', 'Daily', 'Weekly']));
    expect(labels).not.toContain('Monthly');
  });

  // The toggles must actually filter, not just exist.
  it('narrows the map to one cadence when its toggle is clicked', async () => {
    const { container } = render(<ActivityHeatmap report={makeReport()} />);

    const weekly = Array.from(container.querySelectorAll('.map-types .type')).find(
      (button) => button.textContent === 'Weekly',
    );
    await userEvent.click(weekly!);

    // Only the weekly completion on the 3rd is left, and it is a single one.
    expect(cell(container, '03-09-2026')).toHaveClass('day-color-1');
    expect(cell(container, '04-09-2026').className).not.toMatch(/day-color/);
  });

  it('destroys the instance on unmount', () => {
    const { unmount } = render(<ActivityHeatmap report={makeReport()} />);
    expect(window.$heat?.getIds()).toContain(ELEMENT_ID);

    unmount();
    expect(window.$heat?.getIds()).not.toContain(ELEMENT_ID);
  });

  it('says so when there is nothing yet', () => {
    render(<ActivityHeatmap report={makeReport([])} />);
    expect(screen.getByText('No completions recorded yet.')).toBeInTheDocument();
  });
});

describe('summarise', () => {
  it('is singular for one completion on one day', () => {
    const report = makeReport([{ date: '2026-09-03', cadence: 'daily', completed: 1 }]);
    expect(summarise(report)).toBe('1 completion across 1 day.');
  });

  it('is plural otherwise', () => {
    expect(summarise(makeReport())).toBe('6 completions across 2 days.');
  });

  // Rows are per (date, cadence), so two cadences on one day is still one day.
  it('counts each calendar day once however many cadences finished on it', () => {
    const report = makeReport([
      { date: '2026-09-03', cadence: 'daily', completed: 1 },
      { date: '2026-09-03', cadence: 'weekly', completed: 1 },
    ]);
    expect(summarise(report)).toBe('2 completions across 1 day.');
  });

  it('says so when there is nothing yet', () => {
    expect(summarise(makeReport([]))).toBe('No completions recorded yet.');
  });
});
