import type { Preview } from '@storybook/react-vite';
import { sb } from 'storybook/test';
import '../src/index.css';
import { AuthProvider } from '../src/auth/AuthProvider';
import { pb } from '../src/lib/pocketbase';
import { installFakeBackend } from '../src/stories/fakeBackend';
import { USERS } from '../src/stories/fixtures';

// PocketBase is swapped for src/lib/__mocks__/pocketbase.ts, and every method on
// `api` becomes a spy. Storybook restores those spies before each story, so
// installFakeBackend() spies on them again and sets their behaviour.
sb.mock(import('../src/lib/pocketbase.ts'));
sb.mock(import('../src/lib/api.ts'), { spy: true });

type AuthParam = 'user' | 'admin' | 'signedOut';

const preview: Preview = {
  tags: ['autodocs'],
  parameters: {
    layout: 'padded',
    controls: { matchers: { color: /(background|color)$/i, date: /Date$/i } },
  },
  decorators: [
    (Story) => (
      <AuthProvider>
        <Story />
      </AuthProvider>
    ),
  ],
  // Runs before each story, project level first, so a story's own beforeEach can
  // override anything set here.
  beforeEach: ({ parameters }) => {
    const auth = (parameters.auth as AuthParam | undefined) ?? 'user';
    if (auth === 'signedOut') pb.authStore.clear();
    else pb.authStore.save('storybook-token', auth === 'admin' ? USERS.admin : USERS.dana);
    installFakeBackend();

    // The app's router reads window.location, which inside Storybook's iframe is
    // /iframe.html. Stories pick the route they want with `parameters.route`.
    const route = parameters.route as string | undefined;
    if (!route) return;
    const original = location.pathname + location.search + location.hash;
    history.replaceState(history.state, '', route + location.search);
    return () => history.replaceState(history.state, '', original);
  },
};

export default preview;
