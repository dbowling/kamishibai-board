import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, mocked } from 'storybook/test';
import { api } from '../lib/api';
import { failure, pending } from '../stories/fakeBackend';
import { ActivityPanel } from './ActivityPanel';

const meta = {
  title: 'Reports/ActivityPanel',
  component: ActivityPanel,
  parameters: {
    docs: {
      story: { inline: false, iframeHeight: 600 },
      description: {
        component:
          'The completions heatmap with its own fetch, loading and error states. It is independent of the report page’s cadence and period selectors.',
      },
    },
  },
  args: { boardId: 'b1' },
} satisfies Meta<typeof ActivityPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Loaded: Story = {
  play: async ({ canvas }) => {
    await expect(await canvas.findByText(/completions across \d+ days\./)).toBeInTheDocument();
    await expect(canvas.getByText(/America\/New_York/)).toBeInTheDocument();
  },
};

export const Loading: Story = {
  beforeEach: () => {
    mocked(api.activity).mockImplementation(() => pending());
  },
  play: async ({ canvas }) => {
    await expect(await canvas.findByText('Loading activity…')).toBeInTheDocument();
  },
};

export const Error: Story = {
  beforeEach: () => {
    mocked(api.activity).mockImplementation(() => failure('Activity is unavailable.'));
  },
  play: async ({ canvas }) => {
    await expect(await canvas.findByRole('alert')).toHaveTextContent('Activity is unavailable.');
  },
};
