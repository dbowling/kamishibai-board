import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ActivityPanel } from './ActivityPanel';
import { api } from '../lib/api';
import type { ActivityReport } from '../lib/types';

// errorMessage lives in the same module as api and should stay real; only the
// network call is replaced.
vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api: { activity: vi.fn() },
}));

const REPORT: ActivityReport = {
  board: { id: 'board1', teamId: 'team1', name: 'Triage', description: '', archived: false },
  timezone: 'America/New_York',
  days: [
    { date: '2026-09-03', cadence: 'daily', completed: 3 },
    { date: '2026-09-03', cadence: 'weekly', completed: 1 },
    { date: '2026-09-04', cadence: 'daily', completed: 2 },
  ],
  total: 6,
};

describe('ActivityPanel', () => {
  it('loads the activity for the board and shows it', async () => {
    vi.mocked(api.activity).mockResolvedValue(REPORT);

    render(<ActivityPanel boardId="board1" />);

    expect(screen.getByRole('heading', { name: 'Completions by day' })).toBeInTheDocument();
    expect(await screen.findByText('6 completions across 2 days.')).toBeInTheDocument();
    expect(api.activity).toHaveBeenCalledWith('board1');
    expect(screen.getByText(/America\/New_York/)).toBeInTheDocument();
  });

  it('shows a loading note until the data arrives', async () => {
    vi.mocked(api.activity).mockResolvedValue(REPORT);

    render(<ActivityPanel boardId="board1" />);

    expect(screen.getByText('Loading activity…')).toBeInTheDocument();
    await screen.findByText('6 completions across 2 days.');
    expect(screen.queryByText('Loading activity…')).not.toBeInTheDocument();
  });

  it('reports a failure to load', async () => {
    vi.mocked(api.activity).mockRejectedValue({ response: { message: 'nope' } });

    render(<ActivityPanel boardId="board1" />);

    expect(await screen.findByRole('alert')).toHaveTextContent('nope');
    expect(screen.queryByText(/completions across/)).not.toBeInTheDocument();
  });
});
