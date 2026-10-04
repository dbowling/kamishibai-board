import type { StorybookConfig } from '@storybook/react-vite';

const config: StorybookConfig = {
  stories: ['../src/**/*.mdx', '../src/**/*.stories.@(ts|tsx)'],
  addons: ['@storybook/addon-docs', '@storybook/addon-vitest'],
  framework: { name: '@storybook/react-vite', options: { strictMode: true } },
  // TypeScript 7 has no JavaScript API, which react-docgen-typescript needs.
  typescript: { reactDocgen: 'react-docgen' },
  core: { disableTelemetry: true },
};

export default config;
