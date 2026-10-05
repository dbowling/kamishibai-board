import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, fn, waitFor } from 'storybook/test';
import { api } from '../lib/api';
import { USERS } from '../stories/fixtures';
import { TimeZoneDialog } from './TimeZoneDialog';

const meta = {
  title: 'Auth/TimeZoneDialog',
  parameters: {
    layout: 'fullscreen',
    docs: { story: { inline: false, iframeHeight: 400 } },
  },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const closed = fn();

export const ChooseAZone: Story = {
  render: () => <TimeZoneDialog userId={USERS.dana.id} current="" onClose={closed} />,
  play: async ({ canvas, userEvent }) => {
    await expect(await canvas.findByRole('dialog', { name: 'Display time zone' })).toBeInTheDocument();
    // The copy has to say this is display only, since a team's own zone is what
    // decides its periods.
    await expect(canvas.getByText(/only changes how times are displayed/)).toBeInTheDocument();

    await userEvent.selectOptions(canvas.getByLabelText(/^Show times in/), 'Pacific/Auckland');
    await userEvent.click(canvas.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(api.updateDisplayTimeZone).toHaveBeenCalledWith('udana', 'Pacific/Auckland'));
    await waitFor(() => expect(closed).toHaveBeenCalled());
  },
};
