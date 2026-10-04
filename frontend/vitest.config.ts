import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { defineConfig, mergeConfig } from 'vitest/config';
import { storybookTest } from '@storybook/addon-vitest/vitest-plugin';
import { playwright } from '@vitest/browser-playwright';
import viteConfig from './vite.config.ts';

const dirname = path.dirname(fileURLToPath(import.meta.url));

// Two projects share one Vite config:
//
//   unit       jsdom component and logic tests (src/**/*.test.ts[x])
//   storybook  every story rendered in headless Chromium, play functions included
//
// `npm run test` runs only the first so it stays fast and needs no browser;
// `npm run test:storybook` runs the second.
export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      projects: [
        {
          extends: true,
          test: {
            name: 'unit',
            environment: 'jsdom',
            globals: true,
            setupFiles: ['./src/vitest.setup.ts'],
            css: false,
            restoreMocks: true,
            include: ['src/**/*.test.{ts,tsx}'],
          },
        },
        {
          extends: true,
          plugins: [
            storybookTest({
              configDir: path.join(dirname, '.storybook'),
              storybookScript: 'npm run storybook -- --ci',
            }),
          ],
          // Pre-bundled up front: discovering these mid-run makes Vite reload the
          // page under the tests, which fails whichever stories were running.
          optimizeDeps: {
            include: [
              'jheat.js',
              'dompurify',
              'pocketbase',
              'react',
              'react-dom/client',
              'react/jsx-dev-runtime',
              '@storybook/react-dom-shim',
            ],
          },
          test: {
            name: 'storybook',
            browser: {
              enabled: true,
              headless: true,
              provider: playwright(),
              instances: [{ browser: 'chromium' }],
            },
          },
        },
      ],
    },
  }),
);
