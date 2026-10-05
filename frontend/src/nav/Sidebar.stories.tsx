import type { Meta, StoryObj } from '@storybook/react-vite';
import { useState } from 'react';
import { expect, mocked, waitFor, within } from 'storybook/test';
import { useAuth } from '../auth/AuthProvider';
import { useBoards } from '../boards/useBoards';
import { api } from '../lib/api';
import { useRouter } from '../lib/router';
import { pending } from '../stories/fakeBackend';
import { Sidebar } from './Sidebar';

/**
 * The sidebar wired to the real data hook, the way the app does it, so edits in a
 * story really change what the list shows.
 */
function Harness() {
  const { isAdmin } = useAuth();
  const { navigate } = useRouter();
  const [editing, setEditing] = useState(false);
  const { teams, archived, loading, error, refresh } = useBoards(isAdmin && editing);
  return (
    <div style={{ width: 280 }}>
      <Sidebar
        teams={teams}
        archived={archived}
        loading={loading}
        error={error}
        currentBoardId="b1"
        navigate={navigate}
        isAdmin={isAdmin}
        editing={editing}
        onEditingChange={setEditing}
        onChanged={refresh}
      />
    </div>
  );
}

const meta = {
  title: 'Navigation/Sidebar',
  component: Sidebar,
  // The Harness supplies the real props; these only satisfy the component's type.
  args: {
    teams: [],
    archived: { teams: [], boards: [] },
    loading: false,
    error: null,
    currentBoardId: null,
    navigate: () => {},
    isAdmin: false,
    editing: false,
    onEditingChange: () => {},
    onChanged: () => {},
  },
  render: () => <Harness />,
  parameters: {
    layout: 'centered',
    controls: { disable: true },
    docs: {
      story: { inline: false, iframeHeight: 600 },
      description: {
        component:
          'The list of teams and boards. Everyone sees links; admins get an Edit toggle that switches to drag-and-drop management, with archived teams and boards under a disclosure.',
      },
    },
  },
} satisfies Meta<typeof Sidebar>;

export default meta;
type Story = StoryObj<typeof meta>;

async function openEditor(canvas: ReturnType<typeof within>, userEvent: { click: (el: Element) => Promise<void> }) {
  await userEvent.click(await canvas.findByRole('button', { name: 'Edit' }));
  await canvas.findByRole('button', { name: 'Reorder team Platform' });
}

export const ReadOnlyUser: Story = {
  play: async ({ canvas }) => {
    await expect(await canvas.findByRole('link', { name: 'Platform triage' })).toHaveAttribute('aria-current', 'page');
    await expect(canvas.getByRole('link', { name: 'Support rota' })).toBeInTheDocument();
    await expect(canvas.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument();
  },
};

export const AdminReadOnly: Story = {
  parameters: { auth: 'admin' },
  play: async ({ canvas }) => {
    await expect(await canvas.findByRole('button', { name: 'Edit' })).toBeInTheDocument();
    await expect(canvas.getByRole('link', { name: 'Security checks' })).toBeInTheDocument();
  },
};

export const EditMode: Story = {
  parameters: { auth: 'admin' },
  play: async ({ canvas, userEvent }) => {
    await openEditor(canvas, userEvent);
    await expect(canvas.getByRole('button', { name: 'Reorder board Security checks' })).toBeInTheDocument();
    await expect(canvas.getByRole('button', { name: 'Edit board Platform triage' })).toBeInTheDocument();
    await expect(canvas.getByRole('button', { name: 'Archive team Support' })).toBeInTheDocument();
    await expect(canvas.getByRole('button', { name: 'Add board to Platform' })).toBeInTheDocument();
    await expect(canvas.getByRole('button', { name: '+ Add team' })).toBeInTheDocument();
    // Links are gone while editing; Done brings them back.
    await expect(canvas.queryByRole('link', { name: 'Security checks' })).not.toBeInTheDocument();
    await userEvent.click(canvas.getByRole('button', { name: 'Done' }));
    await expect(await canvas.findByRole('link', { name: 'Security checks' })).toBeInTheDocument();
  },
};

export const MoveAcrossZonesWarns: Story = {
  parameters: { auth: 'admin' },
  play: async ({ canvas, userEvent }) => {
    await openEditor(canvas, userEvent);
    await userEvent.click(canvas.getByRole('button', { name: 'Edit board Platform triage' }));
    await userEvent.selectOptions(await canvas.findByLabelText(/^Team/), 't2');
    await userEvent.click(canvas.getByRole('button', { name: 'Continue' }));

    // Platform inherits the instance default and Support runs on Tokyo time.
    await expect(await canvas.findByRole('alertdialog', { name: /Move Platform triage to Support/ })).toBeInTheDocument();
    await expect(await canvas.findByText(/different time zones/)).toHaveTextContent(/Asia\/Tokyo/);
    await expect(canvas.getByText(/reports will show a seam/)).toBeInTheDocument();
  },
};

export const ArchivedSection: Story = {
  parameters: { auth: 'admin' },
  play: async ({ canvas, userEvent }) => {
    await openEditor(canvas, userEvent);
    await userEvent.click(await canvas.findByText('Archived'));
    await expect(await canvas.findByRole('button', { name: 'Restore team Legacy' })).toBeVisible();
    await expect(canvas.getByRole('button', { name: 'Restore board Retired checks' })).toBeVisible();
    // A board of an archived team is not offered on its own.
    await expect(canvas.queryByText('Old rota')).not.toBeInTheDocument();
  },
};

export const ReorderWithKeyboard: Story = {
  parameters: { auth: 'admin' },
  play: async ({ canvas, userEvent }) => {
    await openEditor(canvas, userEvent);

    const handle = canvas.getByRole('button', { name: 'Reorder board Security checks' });
    handle.focus();
    // Space picks up, the arrow moves one place, Space drops.
    await userEvent.keyboard(' ');
    await userEvent.keyboard('{ArrowUp}');
    await userEvent.keyboard(' ');

    await waitFor(() =>
      expect(api.saveOrder).toHaveBeenCalledWith({ boards: { t1: ['b2', 'b1'] } }),
    );
    const items = within(canvas.getByRole('region', { name: 'Platform' })).getAllByRole('listitem');
    await expect(items[0]).toHaveTextContent('Security checks');
  },
};

export const ReorderFailureRollsBack: Story = {
  parameters: { auth: 'admin' },
  beforeEach: () => {
    mocked(api.saveOrder).mockImplementation(() => Promise.reject({ response: { message: 'Not allowed.' } }));
  },
  play: async ({ canvas, userEvent }) => {
    await openEditor(canvas, userEvent);

    canvas.getByRole('button', { name: 'Reorder board Security checks' }).focus();
    await userEvent.keyboard(' ');
    await userEvent.keyboard('{ArrowUp}');
    await userEvent.keyboard(' ');

    await expect(await canvas.findByRole('alert')).toHaveTextContent('Not allowed.');
    const items = within(canvas.getByRole('region', { name: 'Platform' })).getAllByRole('listitem');
    await expect(items[0]).toHaveTextContent('Platform triage');
  },
};

export const Empty: Story = {
  parameters: { auth: 'admin' },
  beforeEach: () => {
    mocked(api.teams).mockImplementation(async () => []);
  },
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'Edit' }));
    await expect(await canvas.findByText('No teams yet.')).toBeInTheDocument();
    await expect(canvas.getByRole('button', { name: '+ Add team' })).toBeInTheDocument();
  },
};

export const Loading: Story = {
  beforeEach: () => {
    mocked(api.teams).mockImplementation(() => pending());
  },
  play: async ({ canvas }) => {
    await expect(await canvas.findByText('Loading…')).toBeInTheDocument();
  },
};
