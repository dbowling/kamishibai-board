import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, mocked, waitFor } from 'storybook/test';
import { api } from '../lib/api';
import { REPORT_EMPTY } from '../stories/fixtures';
import { defaultSeed, failure, installFakeBackend } from '../stories/fakeBackend';
import { ReportPage } from './ReportPage';

const meta = {
  title: 'Pages/ReportPage',
  component: ReportPage,
  parameters: {
    docs: {
      story: { inline: false, iframeHeight: 800 },
      description: {
        component:
          'Completion history for a board: totals, a per-period table, a per-card table and the completions heatmap.',
      },
    },
  },
  args: { boardId: 'b1' },
  argTypes: { boardId: { control: 'select', options: ['b1', 'b2', 'missing'] } },
} satisfies Meta<typeof ReportPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvas, userEvent }) => {
    await expect(await canvas.findByRole('heading', { name: 'Reporting · Platform triage' })).toBeInTheDocument();
    // The open period is flagged because its numbers can still change.
    await expect(await canvas.findByText('live', { selector: '.badge--live' })).toBeInTheDocument();

    await userEvent.selectOptions(canvas.getByLabelText('Cadence'), 'weekly');
    await waitFor(() => expect(mocked(api.report).mock.lastCall).toEqual(['b1', 'weekly', 12]));

    await userEvent.selectOptions(canvas.getByLabelText('Periods'), '24');
    await waitFor(() => expect(mocked(api.report).mock.lastCall).toEqual(['b1', 'weekly', 24]));
  },
};

export const NoData: Story = {
  beforeEach: () => {
    const seed = defaultSeed();
    seed.reports.daily = { ...REPORT_EMPTY, cadence: 'daily' };
    installFakeBackend(seed);
  },
  play: async ({ canvas }) => {
    await expect(await canvas.findByText(/Nothing recorded yet for daily cards/)).toBeInTheDocument();
  },
};

export const Error: Story = {
  beforeEach: () => {
    mocked(api.report).mockImplementation(() => failure('The report could not be built.'));
  },
  play: async ({ canvas }) => {
    await expect(await canvas.findByText('The report could not be built.')).toBeInTheDocument();
  },
};
