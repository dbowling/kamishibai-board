import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, mocked, waitFor, within } from 'storybook/test';
import { api } from '../lib/api';
import { BOARD, CARDS } from '../stories/fixtures';
import { defaultSeed, installFakeBackend, pending } from '../stories/fakeBackend';
import { BoardPage } from './BoardPage';

const meta = {
  title: 'Pages/BoardPage',
  component: BoardPage,
  parameters: {
    docs: {
      story: { inline: false, iframeHeight: 700 },
      description: {
        component:
          'One board: its cards grouped by cadence, the progress tally, and the card and new-card dialogs. Data comes from the in-memory fake backend.',
      },
    },
  },
  args: { boardId: 'b1' },
  argTypes: {
    boardId: { control: 'select', options: ['b1', 'b2', 'missing'] },
  },
} satisfies Meta<typeof BoardPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvas, userEvent }) => {
    await expect(await canvas.findByRole('heading', { name: 'Platform triage' })).toBeInTheDocument();

    // Completing a card calls the API, announces it, and the refetch shows it done.
    const tile = (await canvas.findByRole('article', { name: 'Verify backups' })) as HTMLElement;
    await userEvent.click(within(tile).getByRole('button', { name: 'Mark done' }));
    await waitFor(() => expect(api.complete).toHaveBeenCalledWith('c1', undefined));
    await expect(await canvas.findByRole('status')).toHaveTextContent('Verify backups marked done.');
    await waitFor(() =>
      expect(within(canvas.getByRole('article', { name: 'Verify backups' })).getByText('Done', { selector: '.card__status' })).toBeInTheDocument(),
    );

    // The cadence filter narrows the board to one group.
    await userEvent.selectOptions(canvas.getByLabelText('Cadence'), 'weekly');
    await expect(canvas.getByRole('article', { name: 'Rotate on-call' })).toBeInTheDocument();
    await expect(canvas.queryByRole('article', { name: 'Verify backups' })).not.toBeInTheDocument();
    await userEvent.selectOptions(canvas.getByLabelText('Cadence'), 'all');

    // A card opens in a dialog and Escape closes it.
    await userEvent.click(canvas.getByRole('button', { name: 'Rotate on-call' }));
    await expect(await canvas.findByRole('dialog', { name: 'Rotate on-call' })).toBeInTheDocument();
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(canvas.queryByRole('dialog')).not.toBeInTheDocument());
  },
};

export const CreateCard: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'New card' }));
    const dialog = await canvas.findByRole('dialog', { name: 'New card' });

    await userEvent.type(within(dialog).getByLabelText('Title'), 'Check certificates');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Create card' }));

    await waitFor(() =>
      expect(api.createCard).toHaveBeenCalledWith(
        expect.objectContaining({ boardId: 'b1', title: 'Check certificates', sortOrder: CARDS.length + 1 }),
      ),
    );
    await expect(await canvas.findByRole('button', { name: 'Check certificates' })).toBeInTheDocument();
    await expect(canvas.queryByRole('dialog')).not.toBeInTheDocument();
  },
};

export const Admin: Story = {
  parameters: { auth: 'admin' },
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole('checkbox', { name: 'Show archived' }));
    await userEvent.click(await canvas.findByRole('button', { name: 'Retired VPN check' }));

    // Only an administrator is offered the restore button on an archived card.
    const dialog = await canvas.findByRole('dialog', { name: 'Retired VPN check' });
    await userEvent.click(within(dialog).getByRole('button', { name: 'Restore' }));
    await waitFor(() => expect(api.restoreCard).toHaveBeenCalledWith('c5'));
  },
};

export const Empty: Story = {
  beforeEach: () => {
    const seed = defaultSeed();
    seed.boards.b1 = { board: BOARD, cards: [], activity: [] };
    installFakeBackend(seed);
  },
  play: async ({ canvas }) => {
    await expect(await canvas.findByText('No cards yet.')).toBeInTheDocument();
    await expect(canvas.getByRole('button', { name: 'Add the first card' })).toBeInTheDocument();
  },
};

export const ArchivedBoard: Story = {
  beforeEach: () => {
    const seed = defaultSeed();
    seed.boards.b1 = { board: { ...BOARD, archived: true }, cards: CARDS, activity: [] };
    installFakeBackend(seed);
  },
  play: async ({ canvas }) => {
    await canvas.findByRole('heading', { name: 'Platform triage' });
    // An archived board is read-only, so there is nowhere to add a card.
    await expect(canvas.queryByRole('button', { name: 'New card' })).not.toBeInTheDocument();
  },
};

export const Loading: Story = {
  beforeEach: () => {
    mocked(api.boardState).mockImplementation(() => pending());
  },
  play: async ({ canvas }) => {
    await expect(await canvas.findByText('Loading the board…')).toBeInTheDocument();
  },
};

export const Unavailable: Story = {
  args: { boardId: 'missing' },
  play: async ({ canvas }) => {
    const alert = await canvas.findByRole('alert');
    await expect(alert).toHaveTextContent('That board is not available');
  },
};
