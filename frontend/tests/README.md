# Browser regression tests

From `frontend/`, install dependencies with `npm ci`, then run `npm run test:ui`.
The tests use Node's built-in test runner, Playwright Core and installed Google
Chrome. To use another Chrome/Chromium installation:

```sh
CHROME_PATH=/path/to/chromium npm run test:ui
```

Tests start and stop their own Vite server and browser. All API calls use fixture
responses; no backend service or credentials are needed and no live data changes.

`failure-window.test.mjs` covers SR-13's descriptive native browser popup on Save,
the submission guard, corrections, existing invalid edits/clones and single-failure
checks. Save stays clickable for invalid thresholds. Set `SR13_SCREENSHOT_PATH`
to an absolute PNG path to capture the validation popup for visual review.

`turkish-text.test.mjs` covers the tag input matching free tags whichever of i/ı/İ/I is
typed (`şişli` finds `ŞİŞLİ`, `it` finds `IT`).

`full-backup-download.test.mjs` covers the Full Database Backup card: after a failed or
completed download, the Saved Backups list is reloaded so the server's copy appears.
