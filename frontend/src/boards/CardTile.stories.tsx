import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, fn } from 'storybook/test';
import { ARCHIVED_CARD, DONE_CARD, IN_PROGRESS_CARD, NOT_STARTED_CARD } from '../stories/fixtures';
import { CardTile } from './CardTile';

const meta = {
  title: 'Board/CardTile',
  component: CardTile,
  parameters: {
    docs: {
      description: {
        component:
          'A single card on the board. Status is carried by the face colour and by text, so it does not rely on colour alone.',
      },
    },
  },
  args: {
    card: NOT_STARTED_CARD,
    onOpen: fn(),
    onStart: fn(),
    onComplete: fn(),
    onReopen: fn(),
  },
  argTypes: {
    card: { control: 'object' },
    busy: { control: 'boolean' },
  },
  // CardTile renders an <li>, so it needs a list around it to be valid markup.
  decorators: [
    (Story) => (
      <ul className="card-grid">
        <Story />
      </ul>
    ),
  ],
} satisfies Meta<typeof CardTile>;

export default meta;
type Story = StoryObj<typeof meta>;

export const NotStarted: Story = {
  play: async ({ canvas, userEvent, args }) => {
    await userEvent.click(canvas.getByRole('button', { name: 'Start' }));
    await expect(args.onStart).toHaveBeenCalledWith(args.card);

    await userEvent.click(canvas.getByRole('button', { name: 'Mark done' }));
    await expect(args.onComplete).toHaveBeenCalledWith(args.card);
  },
};

export const InProgress: Story = {
  args: { card: IN_PROGRESS_CARD },
  play: async ({ canvas }) => {
    await expect(canvas.getByText(/Started by Dana Ops/)).toBeInTheDocument();
    await expect(canvas.queryByRole('button', { name: 'Start' })).not.toBeInTheDocument();
  },
};

export const Done: Story = {
  args: { card: DONE_CARD },
  play: async ({ canvas, userEvent, args }) => {
    await userEvent.click(canvas.getByRole('button', { name: 'Reopen' }));
    await expect(args.onReopen).toHaveBeenCalledWith(args.card);
  },
};

export const Archived: Story = {
  args: { card: ARCHIVED_CARD },
  play: async ({ canvas }) => {
    await expect(canvas.getByText(/read-only/)).toBeInTheDocument();
    await expect(canvas.queryByRole('button', { name: /Start|Mark done|Reopen/ })).not.toBeInTheDocument();
  },
};

export const Busy: Story = {
  args: { busy: true },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole('button', { name: 'Start' })).toBeDisabled();
    await expect(canvas.getByRole('button', { name: 'Mark done' })).toBeDisabled();
  },
};
