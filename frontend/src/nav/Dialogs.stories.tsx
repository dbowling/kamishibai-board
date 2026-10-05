import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, fn, mocked, waitFor } from 'storybook/test';
import { api } from '../lib/api';
import { failure } from '../stories/fakeBackend';
import { BOARDS_BY_TEAM, TEAMS } from '../stories/fixtures';
import { BoardDialog } from './BoardDialog';
import { ConfirmDialog } from './ConfirmDialog';
import { TeamDialog } from './TeamDialog';

const meta = {
  title: 'Navigation/Dialogs',
  parameters: {
    layout: 'fullscreen',
    docs: { story: { inline: false, iframeHeight: 500 } },
  },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const platform = TEAMS[0]!;
const support = TEAMS[1]!;
const triage = BOARDS_BY_TEAM.t1![0]!;

const teamSaved = fn();

export const NewTeam: Story = {
  render: () => <TeamDialog nextSortOrder={3} onClose={fn()} onSaved={teamSaved} />,
  play: async ({ canvas, userEvent }) => {
    await userEvent.type(await canvas.findByLabelText(/^Name/), 'Security');
    await userEvent.click(canvas.getByRole('button', { name: 'Create team' }));
    await waitFor(() => expect(api.createTeam).toHaveBeenCalledWith('Security', '', 3, ''));
    await waitFor(() => expect(teamSaved).toHaveBeenCalled());
  },
};

export const EditTeamZoneWarning: Story = {
  render: () => <TeamDialog team={platform} defaultTimezone="America/New_York" onClose={fn()} onSaved={fn()} />,
  play: async ({ canvas, userEvent }) => {
    await expect(canvas.queryByText(/seam/)).not.toBeInTheDocument();
    await userEvent.selectOptions(await canvas.findByLabelText(/^Time zone/), 'Asia/Tokyo');
    await expect(await canvas.findByRole('status')).toHaveTextContent(/reports will show a seam/);
  },
};

export const EditTeamError: Story = {
  render: () => <TeamDialog team={platform} onClose={fn()} onSaved={fn()} />,
  beforeEach: () => {
    mocked(api.updateTeam).mockImplementation(() => failure('name: must be unique'));
  },
  play: async ({ canvas, userEvent }) => {
    const name = await canvas.findByLabelText(/^Name/);
    await userEvent.type(name, ' 2');
    await userEvent.click(canvas.getByRole('button', { name: 'Save team' }));
    await expect(await canvas.findByRole('alert')).toHaveTextContent('name: must be unique');
  },
};

export const NewBoard: Story = {
  render: () => <BoardDialog team={platform} nextSortOrder={4} onClose={fn()} onSaved={fn()} />,
  play: async ({ canvas, userEvent }) => {
    await expect(await canvas.findByRole('dialog', { name: 'New board in Platform' })).toBeInTheDocument();
    await userEvent.type(canvas.getByLabelText(/^Name/), 'Runbook');
    await userEvent.click(canvas.getByRole('button', { name: 'Create board' }));
    await waitFor(() => expect(api.createBoard).toHaveBeenCalledWith('t1', 'Runbook', '', 4));
  },
};

export const EditBoardMove: Story = {
  render: () => (
    <BoardDialog board={triage} teams={[platform, support]} onClose={fn()} onSaved={fn()} onMoveRequested={fn()} />
  ),
  play: async ({ canvas, userEvent }) => {
    await userEvent.selectOptions(await canvas.findByLabelText(/^Team/), 't2');
    await expect(canvas.getByRole('button', { name: 'Continue' })).toBeEnabled();
    await expect(canvas.getByText(/asked to confirm/)).toBeInTheDocument();
  },
};

export const ConfirmArchive: Story = {
  render: () => (
    <ConfirmDialog title="Archive Platform triage?" confirmLabel="Archive board" danger onConfirm={fn()} onCancel={fn()}>
      <p>The board disappears from the sidebar and becomes read-only.</p>
    </ConfirmDialog>
  ),
  play: async ({ canvas }) => {
    await expect(await canvas.findByRole('alertdialog', { name: 'Archive Platform triage?' })).toBeInTheDocument();
    await expect(canvas.getByRole('button', { name: 'Cancel' })).toHaveFocus();
  },
};
