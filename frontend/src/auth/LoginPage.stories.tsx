import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect } from 'storybook/test';
import { LoginPage } from './LoginPage';

const meta = {
  title: 'Auth/LoginPage',
  component: LoginPage,
  parameters: {
    auth: 'signedOut',
    layout: 'fullscreen',
    docs: {
      story: { inline: false, iframeHeight: 600 },
      description: {
        component:
          'The sign-in form. Accounts are created by an administrator; the form does not say whether an email exists.',
      },
    },
  },
} satisfies Meta<typeof LoginPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const InvalidCredentials: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.type(canvas.getByLabelText('Email'), 'dana@example.test');
    await userEvent.type(canvas.getByLabelText('Password'), 'wrong-password');
    await userEvent.click(canvas.getByRole('button', { name: 'Sign in' }));
    await expect(await canvas.findByRole('alert')).toHaveTextContent('Failed to authenticate.');
  },
};
