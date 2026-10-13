# Browser regression tests

From `frontend/`, install dependencies with `npm ci`, then run `npm run test:ui`.
The tests use Node's built-in test runner, Playwright Core and installed Google
Chrome. To use another Chrome/Chromium installation:

```sh
CHROME_PATH=/path/to/chromium npm run test:ui
```

Tests start and stop their own Vite server and browser. All API calls use fixture
responses; no backend service or credentials are needed and no live data changes.

`failure-window.test.mjs` covers SR-13 feedback, submission blocking, corrections,
existing invalid edits/clones and single-failure checks. Set `SR13_SCREENSHOT_PATH`
to an absolute PNG path to capture the inline validation message for visual review.
