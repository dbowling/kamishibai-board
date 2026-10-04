import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, waitFor } from 'storybook/test';
import { ACTIVITY_DAYS, BOARD, makeActivity } from '../stories/fixtures';
import { ActivityHeatmap } from './ActivityHeatmap';

// Heat.js keys each chart on the element id, which comes from the board id, so
// every story uses its own board id. The docs page mounts them all at once.
const forBoard = (id: string, days = ACTIVITY_DAYS) => makeActivity({ ...BOARD, id }, days);

const meta = {
  title: 'Reports/ActivityHeatmap',
  component: ActivityHeatmap,
  parameters: {
    docs: {
      description: {
        component:
          'Completions per calendar day, drawn by Heat.js. The chart is hidden from assistive technology, so the summary sentence beneath it carries the same information.',
      },
    },
  },
  args: { report: forBoard('heatdefault') },
  argTypes: { report: { control: 'object' } },
} satisfies Meta<typeof ActivityHeatmap>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvas, canvasElement }) => {
    // Heat.js draws after mount, so wait for it to have claimed the element.
    const chart = canvasElement.querySelector('#activity-heatmap-heatdefault');
    await waitFor(() => expect(chart).toHaveClass('heat-js'));

    // 3 daily + 1 weekly on 3 September is 4 completions: the second colour step.
    const day = canvasElement.querySelector('[data-heat-js-map-date="03-09-2026"]');
    await expect(day).toHaveClass('day-color-2');
    await expect(canvas.getByText('32 completions across 10 days.')).toBeInTheDocument();
  },
};

export const Empty: Story = {
  args: { report: forBoard('heatempty', []) },
  play: async ({ canvas }) => {
    await expect(canvas.getByText('No completions recorded yet.')).toBeInTheDocument();
  },
};

export const SingleCadence: Story = {
  args: {
    report: forBoard('heatdaily', [
      { date: '2026-09-07', cadence: 'daily', completed: 2 },
      { date: '2026-09-08', cadence: 'daily', completed: 6 },
      { date: '2026-09-09', cadence: 'daily', completed: 1 },
    ]),
  },
  play: async ({ canvas }) => {
    await expect(canvas.getByText('9 completions across 3 days.')).toBeInTheDocument();
  },
};
