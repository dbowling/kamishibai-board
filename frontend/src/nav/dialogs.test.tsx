import '@testing-library/jest-dom/vitest';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { BoardRecord, TeamRecord } from '../lib/types';
import { BoardDialog } from './BoardDialog';
import { ConfirmDialog } from './ConfirmDialog';
import { TeamDialog } from './TeamDialog';

vi.mock('../lib/api', async (importActual) => {
  const actual = await importActual<typeof import('../lib/api')>();
  return {
    ...actual,
    api: {
      createTeam: vi.fn(),
      updateTeam: vi.fn(),
      createBoard: vi.fn(),
      updateBoard: vi.fn(),
    },
  };
});

const { api } = await import('../lib/api');

const team: TeamRecord = {
  id: 't1',
  name: 'Platform',
  description: 'Infra',
  members: [],
  sort_order: 1,
  archived_at: '',
};
const other: TeamRecord = { ...team, id: 't2', name: 'Support', sort_order: 2 };
const board: BoardRecord = {
  id: 'b1',
  team: 't1',
  name: 'Triage',
  description: '',
  sort_order: 1,
  archived_at: '',
};

beforeEach(() => {
  vi.mocked(api.createTeam).mockResolvedValue(team);
  vi.mocked(api.updateTeam).mockResolvedValue(team);
  vi.mocked(api.createBoard).mockResolvedValue(board);
  vi.mocked(api.updateBoard).mockResolvedValue(board);
});

describe('ConfirmDialog', () => {
  it('is a labelled modal that focuses Cancel and confirms on click', async () => {
    const onConfirm = vi.fn();
    const onCancel = vi.fn();
    render(
      <ConfirmDialog title="Archive X?" confirmLabel="Archive" onConfirm={onConfirm} onCancel={onCancel}>
        <p>Careful.</p>
      </ConfirmDialog>,
    );

    expect(screen.getByRole('alertdialog', { name: 'Archive X?' })).toHaveAttribute('aria-modal', 'true');
    expect(screen.getByRole('button', { name: 'Cancel' })).toHaveFocus();

    await userEvent.click(screen.getByRole('button', { name: 'Archive' }));
    expect(onConfirm).toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();
  });

  it('cancels on Escape', async () => {
    const onCancel = vi.fn();
    render(
      <ConfirmDialog title="T" confirmLabel="Go" onConfirm={vi.fn()} onCancel={onCancel}>
        x
      </ConfirmDialog>,
    );
    await userEvent.keyboard('{Escape}');
    expect(onCancel).toHaveBeenCalled();
  });
});

describe('TeamDialog', () => {
  it('creates a team with the sort order it was given', async () => {
    const onSaved = vi.fn();
    render(<TeamDialog nextSortOrder={4} onClose={vi.fn()} onSaved={onSaved} />);

    const create = screen.getByRole('button', { name: 'Create team' });
    expect(create).toBeDisabled();
    await userEvent.type(screen.getByLabelText('Name'), '  Ops ');
    await userEvent.click(create);

    await waitFor(() => expect(api.createTeam).toHaveBeenCalledWith('Ops', '', 4));
    expect(onSaved).toHaveBeenCalled();
  });

  it('edits an existing team', async () => {
    render(<TeamDialog team={team} onClose={vi.fn()} onSaved={vi.fn()} />);

    const name = screen.getByLabelText('Name');
    expect(name).toHaveValue('Platform');
    await userEvent.clear(name);
    await userEvent.type(name, 'Platform 2');
    await userEvent.click(screen.getByRole('button', { name: 'Save team' }));

    await waitFor(() =>
      expect(api.updateTeam).toHaveBeenCalledWith('t1', { name: 'Platform 2', description: 'Infra' }),
    );
  });

  it('shows the server message and stays open on failure', async () => {
    vi.mocked(api.createTeam).mockRejectedValue({ response: { message: 'name: must be unique' } });
    const onSaved = vi.fn();
    render(<TeamDialog onClose={vi.fn()} onSaved={onSaved} />);

    await userEvent.type(screen.getByLabelText('Name'), 'Dup');
    await userEvent.click(screen.getByRole('button', { name: 'Create team' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('name: must be unique');
    expect(onSaved).not.toHaveBeenCalled();
  });

  it('closes on Escape', async () => {
    const onClose = vi.fn();
    render(<TeamDialog onClose={onClose} onSaved={vi.fn()} />);
    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalled();
  });
});

describe('BoardDialog', () => {
  it('creates a board in the given team, placed last', async () => {
    const onSaved = vi.fn();
    render(<BoardDialog team={team} nextSortOrder={3} onClose={vi.fn()} onSaved={onSaved} />);

    expect(screen.getByRole('dialog', { name: 'New board in Platform' })).toBeInTheDocument();
    expect(screen.queryByLabelText(/^Team/)).not.toBeInTheDocument();
    await userEvent.type(screen.getByLabelText('Name'), 'Runbook');
    await userEvent.click(screen.getByRole('button', { name: 'Create board' }));

    await waitFor(() => expect(api.createBoard).toHaveBeenCalledWith('t1', 'Runbook', '', 3));
    expect(onSaved).toHaveBeenCalled();
  });

  it('saves a rename without a move when the team is unchanged', async () => {
    const onMoveRequested = vi.fn();
    render(
      <BoardDialog
        board={board}
        teams={[team, other]}
        onClose={vi.fn()}
        onSaved={vi.fn()}
        onMoveRequested={onMoveRequested}
      />,
    );

    await userEvent.type(screen.getByLabelText('Name'), ' v2');
    await userEvent.click(screen.getByRole('button', { name: 'Save board' }));

    await waitFor(() =>
      expect(api.updateBoard).toHaveBeenCalledWith('b1', { name: 'Triage v2', description: '' }),
    );
    expect(onMoveRequested).not.toHaveBeenCalled();
  });

  it('hands a team change to the caller instead of saving it', async () => {
    const onMoveRequested = vi.fn();
    render(
      <BoardDialog
        board={board}
        teams={[team, other]}
        onClose={vi.fn()}
        onSaved={vi.fn()}
        onMoveRequested={onMoveRequested}
      />,
    );

    await userEvent.selectOptions(screen.getByLabelText(/^Team/), 't2');
    await userEvent.click(screen.getByRole('button', { name: 'Continue' }));

    expect(onMoveRequested).toHaveBeenCalledWith({ board, toTeamId: 't2', edits: null });
    expect(api.updateBoard).not.toHaveBeenCalled();
  });

  it('passes pending edits along with the move', async () => {
    const onMoveRequested = vi.fn();
    render(
      <BoardDialog
        board={board}
        teams={[team, other]}
        onClose={vi.fn()}
        onSaved={vi.fn()}
        onMoveRequested={onMoveRequested}
      />,
    );

    await userEvent.type(screen.getByLabelText('Name'), '!');
    await userEvent.selectOptions(screen.getByLabelText(/^Team/), 't2');
    await userEvent.click(screen.getByRole('button', { name: 'Continue' }));

    expect(onMoveRequested).toHaveBeenCalledWith({
      board,
      toTeamId: 't2',
      edits: { name: 'Triage!', description: '' },
    });
  });
});
