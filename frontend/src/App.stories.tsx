import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, mocked, waitFor, within } from 'storybook/test';
import { api } from './lib/api';
import { App } from './App';
import { failure } from './stories/fakeBackend';

const meta = {
  title: 'Pages/App',
  component: App,
  parameters: {
    layout: 'fullscreen',
    controls: { disable: true },
    docs: {
      story: { inline: false, iframeHeight: 800 },
      description: {
        component:
          'The whole application: sign-in, the sidebar of teams and boards, and the board and report views. Stories choose the signed-in user with `parameters.auth` and the page with `parameters.route`.',
      },
    },
  },
} satisfies Meta<typeof App>;

export default meta;
type Story = StoryObj<typeof meta>;

export const SignedOut: Story = {
  parameters: { auth: 'signedOut', route: '/' },
  play: async ({ canvas, userEvent }) => {
    await userEvent.type(await canvas.findByLabelText('Email'), 'dana@example.test');
    await userEvent.type(canvas.getByLabelText('Password'), 'correct-horse');
    await userEvent.click(canvas.getByRole('button', { name: 'Sign in' }));

    // Signing in swaps the form for the shell, and the home route lands on a board.
    await expect(await canvas.findByText('Triage board', { selector: 'small' })).toBeInTheDocument();
    const sidebar = await canvas.findByRole('navigation', { name: 'Boards' });
    await expect(await within(sidebar).findByRole('link', { name: 'Platform triage' })).toBeInTheDocument();
  },
};

export const BoardView: Story = {
  parameters: { route: '/boards/b1' },
  play: async ({ canvas, userEvent }) => {
    await expect(await canvas.findByRole('heading', { name: 'Platform triage', level: 2 })).toBeInTheDocument();
    const sidebar = canvas.getByRole('navigation', { name: 'Boards' });
    await expect(await within(sidebar).findByRole('link', { name: 'Security checks' })).toBeInTheDocument();

    await userEvent.click(canvas.getByRole('tab', { name: 'Reporting' }));
    await expect(await canvas.findByRole('heading', { name: /^Reporting/ })).toBeInTheDocument();
  },
};

export const ReportView: Story = {
  parameters: { route: '/boards/b1/report' },
  play: async ({ canvas }) => {
    await expect(await canvas.findByRole('heading', { name: 'Reporting · Platform triage' })).toBeInTheDocument();
  },
};

export const AdminBoardView: Story = {
  parameters: { auth: 'admin', route: '/boards/b1' },
  play: async ({ canvas }) => {
    await expect(await canvas.findByText('admin', { selector: '.badge--admin' })).toBeInTheDocument();
  },
};

export const NoTeams: Story = {
  parameters: { route: '/' },
  beforeEach: () => {
    mocked(api.teams).mockImplementation(async () => []);
  },
  play: async ({ canvas }) => {
    await expect(await canvas.findByText(/You are not on any team yet/)).toBeInTheDocument();
  },
};

export const TeamsError: Story = {
  parameters: { route: '/' },
  beforeEach: () => {
    mocked(api.teams).mockImplementation(() => failure('Teams are unavailable.'));
  },
  play: async ({ canvas }) => {
    const sidebar = canvas.getByRole('navigation', { name: 'Boards' });
    await expect(await within(sidebar).findByRole('alert')).toHaveTextContent('Teams are unavailable.');
  },
};

export const NotFound: Story = {
  parameters: { route: '/nope' },
  play: async ({ canvas }) => {
    await expect(await canvas.findByRole('heading', { name: 'Page not found' })).toBeInTheDocument();
  },
};

export const NonAdminHasNoEditButton: Story = {
  parameters: { route: '/boards/b1' },
  play: async ({ canvas }) => {
    const sidebar = await canvas.findByRole('navigation', { name: 'Boards' });
    await expect(await within(sidebar).findByRole('link', { name: 'Security checks' })).toBeInTheDocument();
    await expect(within(sidebar).queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument();
  },
};

export const AdminCreatesBoard: Story = {
  parameters: { auth: 'admin', route: '/boards/b1' },
  play: async ({ canvas, userEvent }) => {
    const sidebar = await canvas.findByRole('navigation', { name: 'Boards' });
    await userEvent.click(await within(sidebar).findByRole('button', { name: 'Edit' }));
    await userEvent.click(await within(sidebar).findByRole('button', { name: 'Add board to Support' }));

    const dialog = await canvas.findByRole('dialog', { name: 'New board in Support' });
    await userEvent.type(within(dialog).getByLabelText(/^Name/), 'Escalations');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Create board' }));

    await expect(await within(sidebar).findByText('Escalations')).toBeInTheDocument();
    await expect(canvas.queryByRole('dialog')).not.toBeInTheDocument();
  },
};

export const AdminRestoresBoard: Story = {
  parameters: { auth: 'admin', route: '/boards/b1' },
  play: async ({ canvas, userEvent }) => {
    const sidebar = await canvas.findByRole('navigation', { name: 'Boards' });
    await userEvent.click(await within(sidebar).findByRole('button', { name: 'Edit' }));
    await userEvent.click(await within(sidebar).findByText('Archived'));
    await userEvent.click(await within(sidebar).findByRole('button', { name: 'Restore board Retired checks' }));

    // Back in the active list for team Platform, and gone from Archived.
    await waitFor(() =>
      expect(within(sidebar).queryByRole('button', { name: 'Restore board Retired checks' })).not.toBeInTheDocument(),
    );
    await expect(
      await within(sidebar).findByRole('button', { name: 'Edit board Retired checks' }),
    ).toBeInTheDocument();
  },
};

export const AdminArchivesOpenBoard: Story = {
  parameters: { auth: 'admin', route: '/boards/b2' },
  play: async ({ canvas, userEvent }) => {
    const sidebar = await canvas.findByRole('navigation', { name: 'Boards' });
    await userEvent.click(await within(sidebar).findByRole('button', { name: 'Edit' }));
    await userEvent.click(await within(sidebar).findByRole('button', { name: 'Archive board Security checks' }));
    await userEvent.click(await canvas.findByRole('button', { name: 'Archive board' }));

    // The open board vanished, so the app goes home and lands on the first board.
    await waitFor(() => expect(location.pathname).toBe('/boards/b1'));
  },
};

export const AdminMovesBoardWithDialog: Story = {
  parameters: { auth: 'admin', route: '/boards/b1' },
  play: async ({ canvas, userEvent }) => {
    const sidebar = await canvas.findByRole('navigation', { name: 'Boards' });
    await userEvent.click(await within(sidebar).findByRole('button', { name: 'Edit' }));
    await userEvent.click(await within(sidebar).findByRole('button', { name: 'Edit board Security checks' }));

    const dialog = await canvas.findByRole('dialog', { name: 'Edit board' });
    await userEvent.selectOptions(within(dialog).getByLabelText(/^Team/), 't2');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Continue' }));

    const confirm = await canvas.findByRole('alertdialog', { name: 'Move Security checks to Support?' });
    await expect(confirm).toHaveTextContent('cards and history move with it');
    await userEvent.click(within(confirm).getByRole('button', { name: 'Move board' }));

    await waitFor(() => expect(api.moveBoard).toHaveBeenCalledWith('b2', 't2'));
    await waitFor(() => expect(api.saveOrder).toHaveBeenCalledWith({ boards: { t2: ['b3', 'b2'] } }));
  },
};
