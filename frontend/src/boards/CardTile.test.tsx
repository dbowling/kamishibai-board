import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { CardTile, describeAttribution } from './CardTile';
import type { Card, CardState, Status } from '../lib/types';

function makeCard(overrides: Partial<Card> = {}, state: Partial<CardState> = {}): Card {
  return {
    id: 'card1',
    boardId: 'board1',
    title: 'Verify Backups',
    summary: 'Confirm last night’s backups completed.',
    cadence: 'daily',
    instructions: '<p>Check the dashboard.</p>',
    links: [],
    checklist: [],
    sortOrder: 1,
    archived: false,
    period: {
      cadence: 'daily',
      key: '2026-09-03',
      start: '2026-09-03T04:00:00Z',
      end: '2099-01-01T05:00:00Z',
      label: 'Daily',
    },
    state: {
      status: 'not_started',
      periodKey: '2026-09-03',
      ...state,
    },
    ...overrides,
  };
}

function renderTile(card: Card, handlers: Partial<Parameters<typeof CardTile>[0]> = {}) {
  const props = {
    card,
    onOpen: vi.fn(),
    onStart: vi.fn(),
    onComplete: vi.fn(),
    onReopen: vi.fn(),
    ...handlers,
  };
  render(<CardTile {...props} />);
  return props;
}

describe('CardTile', () => {
  it('shows the title, cadence and period', () => {
    renderTile(makeCard());

    expect(screen.getByRole('button', { name: 'Verify Backups' })).toBeInTheDocument();
    expect(screen.getByText('Daily')).toBeInTheDocument();
    expect(screen.getByText(/2026-09-03/)).toBeInTheDocument();
  });

  // Status must never be conveyed by colour alone.
  it('states the status in text', () => {
    renderTile(makeCard());
    expect(screen.getByText('Not started')).toBeInTheDocument();
  });

  it('offers Start and Mark done when not started', () => {
    renderTile(makeCard());

    expect(screen.getByRole('button', { name: 'Start' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Mark done' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Reopen' })).not.toBeInTheDocument();
  });

  it('offers only Mark done when in progress', () => {
    renderTile(makeCard({}, { status: 'in_progress', startedBy: user('Dana') }));

    expect(screen.queryByRole('button', { name: 'Start' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Mark done' })).toBeInTheDocument();
  });

  it('offers only Reopen when done', () => {
    renderTile(makeCard({}, { status: 'done', completedBy: user('Dana') }));

    expect(screen.getByRole('button', { name: 'Reopen' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Mark done' })).not.toBeInTheDocument();
  });

  it('calls the handlers', async () => {
    const props = renderTile(makeCard());

    await userEvent.click(screen.getByRole('button', { name: 'Start' }));
    expect(props.onStart).toHaveBeenCalledOnce();

    await userEvent.click(screen.getByRole('button', { name: 'Mark done' }));
    expect(props.onComplete).toHaveBeenCalledOnce();

    await userEvent.click(screen.getByRole('button', { name: 'Verify Backups' }));
    expect(props.onOpen).toHaveBeenCalledOnce();
  });

  it('disables the actions while a request is in flight', () => {
    renderTile(makeCard(), { busy: true });
    expect(screen.getByRole('button', { name: 'Start' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Mark done' })).toBeDisabled();
  });

  it('shows attribution once someone has acted', () => {
    renderTile(makeCard({}, { status: 'done', completedBy: user('Dana Ops') }));
    expect(screen.getByText(/Done by Dana Ops/)).toBeInTheDocument();
  });

  // An archived card is history: readable, but no longer actionable.
  it('replaces the actions with a read-only note when archived', () => {
    renderTile(makeCard({ archived: true, archivedAt: '2026-09-01T12:00:00Z' }));

    expect(screen.queryByRole('button', { name: 'Start' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Mark done' })).not.toBeInTheDocument();
    expect(screen.getByText(/read-only/)).toBeInTheDocument();
  });

  it('shows how long is left while the work is outstanding', () => {
    renderTile(makeCard());
    expect(screen.getByText(/left/)).toBeInTheDocument();
  });

  it('drops the countdown once the work is done', () => {
    renderTile(makeCard({}, { status: 'done', completedBy: user('Dana') }));
    expect(screen.queryByText(/left$/)).not.toBeInTheDocument();
  });
});

describe('describeAttribution', () => {
  it('is empty when nothing has happened', () => {
    expect(describeAttribution(makeCard())).toBe('');
  });

  it('names the starter while in progress', () => {
    const card = makeCard({}, { status: 'in_progress', startedBy: user('Raj') });
    expect(describeAttribution(card)).toContain('Started by Raj');
  });

  it('writes the time in the viewer display zone', () => {
    const card = makeCard(
      {},
      { status: 'done', completedBy: user('Mei'), completedAt: '2026-10-05T20:00:00-04:00' },
    );
    // The same completion, 20:00 in New York and 09:00 the next day in Tokyo.
    expect(describeAttribution(card, 'America/New_York')).toMatch(/(08:00 PM|20:00)/);
    expect(describeAttribution(card, 'Asia/Tokyo')).toContain('09:00');
  });

  it('names the completer when done', () => {
    const card = makeCard({}, { status: 'done', completedBy: user('Mei') });
    expect(describeAttribution(card)).toContain('Done by Mei');
  });

  // Both are tracked separately, so both are shown when they differ.
  it('mentions the starter when a different person finished it', () => {
    const card = makeCard(
      {},
      {
        status: 'done',
        startedBy: user('Raj', 'u2'),
        completedBy: user('Mei', 'u3'),
      },
    );
    const text = describeAttribution(card);
    expect(text).toContain('Done by Mei');
    expect(text).toContain('started by Raj');
  });

  it('does not repeat one person twice', () => {
    const card = makeCard(
      {},
      { status: 'done', startedBy: user('Mei', 'u3'), completedBy: user('Mei', 'u3') },
    );
    const text = describeAttribution(card);
    expect(text).toContain('Done by Mei');
    expect(text).not.toContain('started by');
  });

  it('falls back to a placeholder when the name is missing', () => {
    const card = makeCard(
      {},
      { status: 'done', completedBy: { id: 'u9', name: '', email: '' } },
    );
    expect(describeAttribution(card)).toContain('someone');
  });
});

function user(name: string, id = 'u1') {
  return { id, name, email: `${name.split(' ')[0]?.toLowerCase()}@example.test` };
}

describe('status coverage', () => {
  // Guards against a new status being added without the tile learning about it.
  const statuses: Status[] = ['not_started', 'in_progress', 'done'];

  it('renders every status without crashing', () => {
    for (const status of statuses) {
      const { unmount } = render(
        <CardTile
          card={makeCard({}, { status })}
          onOpen={vi.fn()}
          onStart={vi.fn()}
          onComplete={vi.fn()}
          onReopen={vi.fn()}
        />,
      );
      unmount();
    }
  });
});
