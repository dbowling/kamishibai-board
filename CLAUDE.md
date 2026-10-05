# CLAUDE.md

Notes for Claude Code working in this repository. See `docs/testing.md` and
`docs/ci.md` for the full testing and CI setup.

## Running Storybook tests in a Claude Code cloud session

The Claude Code cloud container ships a preinstalled Chromium under
`/opt/pw-browsers`, but its build is usually older than the one the pinned
Playwright expects, so `npm run test:storybook` fails with
`browserType.launch: Executable doesn't exist at /opt/pw-browsers/...`.

Do not run `playwright install`. Instead point `CHROME_PATH` (read by
`frontend/vitest.config.ts`) at the installed Chromium:

```sh
cd frontend
CHROME_PATH=/opt/pw-browsers/chromium-1194/chrome-linux/chrome npm run test:storybook
```

If that path is missing, `ls /opt/pw-browsers` to find the current
`chromium-<build>` directory. Unit tests (`npm test`) and `npm run lint` need no
browser.
