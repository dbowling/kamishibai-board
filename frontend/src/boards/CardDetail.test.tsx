import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { CardDetail } from './CardDetail';
import type { Card, CardState } from '../lib/types';

function makeCard(overrides: Partial<Card> = {}, state: Partial<CardState> = {}): Card {
  return {
    id: 'card1',
    boardId: 'board1',
    title: 'Verify Backups',
    summary: 'Confirm last night’s backups completed.',
    cadence: 'daily',
    instructions: '<p>Check the <strong>backup</strong> dashboard.</p>',
    links: [
      { label: 'Grafana: backups', url: 'https://grafana.example.test/d/backups' },
    ],
    checklist: [
      { text: 'Check the backup job logs' },
      { text: 'Confirm the storage target has capacity' },
      { text: 'Re-run the backup manually' },
    ],
    sortOrder: 1,
    archived: false,
    period: {
      cadence: 'daily',
      key: '2026-09-03',
      start: '2026-09-03T04:00:00Z',
      end: '2099-01-01T05:00:00Z',
      label: 'Daily',
    },
    state: { status: 'not_started', periodKey: '2026-09-03', ...state },
    ...overrides,
  };
}

function renderDetail(card: Card, overrides: Record<string, unknown> = {}) {
  const props = {
    card,
    canRestore: false,
    onClose: vi.fn(),
    onStart: vi.fn(),
    onComplete: vi.fn(),
    onReopen: vi.fn(),
    onArchive: vi.fn(),
    onRestore: vi.fn(),
    ...overrides,
  } as Parameters<typeof CardDetail>[0];

  render(<CardDetail {...props} />);
  return props;
}

describe('CardDetail', () => {
  it('presents itself as a modal dialog labelled by the card title', () => {
    renderDetail(makeCard());

    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(screen.getByRole('heading', { name: 'Verify Backups' })).toBeInTheDocument();
  });

  it('renders the instructions, including their inline markup', () => {
    renderDetail(makeCard());

    // Scoped to the instructions section: "Check the..." also appears as a
    // checklist step, and an ambiguous query would match both.
    const instructions = screen.getByRole('region', { name: 'Instructions' });
    expect(instructions).toHaveTextContent('Check the backup dashboard.');
    // The <strong> survives sanitising, so emphasis written by the author is kept.
    expect(instructions.querySelector('strong')).toHaveTextContent('backup');
  });

  // Instructions are authored by teammates, so the renderer must not trust them.
  it('strips scripts out of the instructions', () => {
    renderDetail(
      makeCard({
        instructions: '<p>Safe text</p><script>window.__pwned = true;</script>',
      }),
    );

    expect(screen.getByText('Safe text')).toBeInTheDocument();
    expect(document.querySelector('script')).toBeNull();
    expect((window as unknown as { __pwned?: boolean }).__pwned).toBeUndefined();
  });

  it('renders links as external links with a safe rel', () => {
    renderDetail(makeCard());

    const link = screen.getByRole('link', { name: /Grafana: backups/ });
    expect(link).toHaveAttribute('href', 'https://grafana.example.test/d/backups');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', expect.stringContaining('noopener'));
    expect(link).toHaveAttribute('rel', expect.stringContaining('noreferrer'));
  });

  // A javascript: link would execute for every teammate who opened the card.
  it('drops links with an unsafe scheme', () => {
    renderDetail(
      makeCard({
        links: [
          { label: 'Bad', url: 'javascript:alert(1)' },
          { label: 'Good', url: 'https://example.test/ok' },
        ],
      }),
    );

    expect(screen.queryByRole('link', { name: /Bad/ })).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: /Good/ })).toBeInTheDocument();
  });

  it('lists the checklist steps with working labels', async () => {
    renderDetail(makeCard());

    const boxes = screen.getAllByRole('checkbox');
    expect(boxes).toHaveLength(3);

    const first = screen.getByLabelText('Check the backup job logs');
    expect(first).not.toBeChecked();
    await userEvent.click(first);
    expect(first).toBeChecked();
  });

  it('says that ticks are not saved, so nobody assumes otherwise', () => {
    renderDetail(makeCard());
    expect(screen.getByText(/not saved/i)).toBeInTheDocument();
  });

  it('passes the notes along when completing', async () => {
    const props = renderDetail(makeCard());

    await userEvent.type(screen.getByRole('textbox'), 'all green');
    await userEvent.click(screen.getByRole('button', { name: 'Mark done' }));

    expect(props.onComplete).toHaveBeenCalledWith(expect.objectContaining({ id: 'card1' }), 'all green');
  });

  it('closes on Escape', async () => {
    const props = renderDetail(makeCard());
    await userEvent.keyboard('{Escape}');
    expect(props.onClose).toHaveBeenCalled();
  });

  it('closes via the close button', async () => {
    const props = renderDetail(makeCard());
    await userEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(props.onClose).toHaveBeenCalled();
  });

  it('offers Reopen instead of Mark done when already complete', () => {
    renderDetail(makeCard({}, { status: 'done', completedBy: { id: 'u1', name: 'Dana', email: '' } }));

    expect(screen.getByRole('button', { name: 'Reopen' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Mark done' })).not.toBeInTheDocument();
  });

  describe('archived cards', () => {
    it('are read-only and explain that history is kept', () => {
      renderDetail(makeCard({ archived: true, archivedAt: '2026-09-01T12:00:00Z' }));

      expect(screen.queryByRole('button', { name: 'Mark done' })).not.toBeInTheDocument();
      expect(screen.queryByRole('button', { name: 'Archive' })).not.toBeInTheDocument();
      expect(screen.getByText(/history is\s+kept/i)).toBeInTheDocument();
    });

    it('tell a regular member that an admin can restore', () => {
      renderDetail(makeCard({ archived: true }), { canRestore: false });

      expect(screen.queryByRole('button', { name: 'Restore' })).not.toBeInTheDocument();
      expect(screen.getByText(/administrator can restore/i)).toBeInTheDocument();
    });

    it('offer Restore to an admin', async () => {
      const props = renderDetail(makeCard({ archived: true }), { canRestore: true });

      const restore = screen.getByRole('button', { name: 'Restore' });
      await userEvent.click(restore);
      expect(props.onRestore).toHaveBeenCalled();
    });

    it('hide the notes field, since nothing more can be recorded', () => {
      renderDetail(makeCard({ archived: true }));
      expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    });
  });

  it('archives on request', async () => {
    const props = renderDetail(makeCard());
    await userEvent.click(screen.getByRole('button', { name: 'Archive' }));
    expect(props.onArchive).toHaveBeenCalled();
  });
});
