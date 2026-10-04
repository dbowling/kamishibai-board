import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, fn, mocked, waitFor } from 'storybook/test';
import { api } from '../lib/api';
import { failure } from '../stories/fakeBackend';
import { NewCardDialog } from './NewCardDialog';

const meta = {
  title: 'Board/NewCardDialog',
  component: NewCardDialog,
  parameters: {
    layout: 'fullscreen',
    docs: {
      story: { inline: false, iframeHeight: 700 },
      description: {
        component:
          'The create-card form. Links and steps are entered one per line; instructions are escaped into paragraphs before they are sent.',
      },
    },
  },
  args: {
    boardId: 'b1',
    nextSortOrder: 5,
    onClose: fn(),
    onCreated: fn(),
  },
} satisfies Meta<typeof NewCardDialog>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvas, userEvent, args }) => {
    const create = await canvas.findByRole('button', { name: 'Create card' });
    await expect(create).toBeDisabled();

    await userEvent.type(canvas.getByLabelText('Title'), 'Check certificates');
    await expect(create).toBeEnabled();
    await userEvent.type(canvas.getByLabelText(/^Summary/), 'Renew anything expiring soon.');
    await userEvent.selectOptions(canvas.getByLabelText(/^Cadence/), 'weekly');
    await userEvent.click(create);

    await waitFor(() =>
      expect(api.createCard).toHaveBeenCalledWith(
        expect.objectContaining({ boardId: 'b1', title: 'Check certificates', cadence: 'weekly', sortOrder: 5 }),
      ),
    );
    await waitFor(() => expect(args.onCreated).toHaveBeenCalled());
  },
};

export const ServerError: Story = {
  beforeEach: () => {
    mocked(api.createCard).mockImplementation(() => failure('title: cannot be blank'));
  },
  play: async ({ canvas, userEvent }) => {
    await userEvent.type(await canvas.findByLabelText('Title'), 'Anything');
    await userEvent.click(canvas.getByRole('button', { name: 'Create card' }));
    await expect(await canvas.findByRole('alert')).toHaveTextContent('title: cannot be blank');
  },
};
