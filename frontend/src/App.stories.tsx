import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, mocked, within } from 'storybook/test';
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
