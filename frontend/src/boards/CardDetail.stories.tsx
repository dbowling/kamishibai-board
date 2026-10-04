import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, fn, waitFor } from 'storybook/test';
import {
  ARCHIVED_CARD,
  DONE_CARD,
  IN_PROGRESS_CARD,
  NOT_STARTED_CARD,
  UNSAFE_INSTRUCTIONS_HTML,
  makeCard,
} from '../stories/fixtures';
import { CardDetail } from './CardDetail';

const meta = {
  title: 'Board/CardDetail',
  component: CardDetail,
  parameters: {
    layout: 'fullscreen',
    docs: {
      story: { inline: false, iframeHeight: 700 },
      description: {
        component:
          'The full card in a side drawer: instructions, quick links, the checklist and a notes field. Instructions are sanitised before they are rendered.',
      },
    },
  },
  args: {
    card: NOT_STARTED_CARD,
    canRestore: false,
    onClose: fn(),
    onStart: fn(),
    onComplete: fn(),
    onReopen: fn(),
    onArchive: fn(),
    onRestore: fn(),
  },
  argTypes: {
    card: { control: 'object' },
    busy: { control: 'boolean' },
    canRestore: { control: 'boolean' },
  },
} satisfies Meta<typeof CardDetail>;

export default meta;
type Story = StoryObj<typeof meta>;

export const NotStarted: Story = {
  play: async ({ canvas, userEvent, args }) => {
    const dialog = await canvas.findByRole('dialog', { name: 'Verify backups' });
    await expect(dialog).toBeInTheDocument();

    await userEvent.type(canvas.getByPlaceholderText('Anything the next person should know.'), 'typed note');
    await userEvent.click(canvas.getByRole('button', { name: 'Mark done' }));
    await expect(args.onComplete).toHaveBeenCalledWith(args.card, 'typed note');

    await userEvent.keyboard('{Escape}');
    await expect(args.onClose).toHaveBeenCalled();
  },
};

export const InProgress: Story = {
  args: { card: IN_PROGRESS_CARD },
};

export const Done: Story = {
  args: { card: DONE_CARD },
};

export const ArchivedAdmin: Story = {
  args: { card: ARCHIVED_CARD, canRestore: true },
  play: async ({ canvas, userEvent, args }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'Restore' }));
    await expect(args.onRestore).toHaveBeenCalledWith(args.card);
  },
};

export const ArchivedUser: Story = {
  args: { card: ARCHIVED_CARD, canRestore: false },
  play: async ({ canvas }) => {
    await expect(await canvas.findByText('An administrator can restore this card.')).toBeInTheDocument();
    await expect(canvas.queryByRole('button', { name: 'Restore' })).not.toBeInTheDocument();
  },
};

/** Instructions are written by teammates, so scripts and handlers must not survive. */
export const UnsafeContent: Story = {
  args: {
    card: makeCard({ instructions: UNSAFE_INSTRUCTIONS_HTML, links: [] }),
  },
  play: async ({ canvas, canvasElement }) => {
    const dialog = await canvas.findByRole('dialog');
    await expect(dialog).toHaveTextContent('Safe paragraph.');
    await expect(dialog.querySelector('script')).toBeNull();
    await expect(dialog.querySelector('[onerror]')).toBeNull();
    await expect(canvasElement.querySelector('a[href^="javascript:"]')).toBeNull();
    await waitFor(() => expect((window as unknown as { __pwned?: boolean }).__pwned).toBeUndefined());
  },
};
