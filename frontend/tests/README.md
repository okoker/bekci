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
typed (`şişli` finds `ŞİŞLİ`, `it` finds `IT`), and the Settings save using the server's SLA key
for a category named with `İ`.

`full-backup-download.test.mjs` covers the Full Database Backup card: after a failed or
completed download, the Saved Backups list is reloaded so the server's copy appears.

`alert-method.test.mjs` covers Settings > Alerting > Alert Method "Disabled" (stored as
`""`): it loads as Disabled, saves as `""`, and still shows Disabled after the save.

`primary-badge.test.mjs` covers the PRIMARY badge for a target with two checks of its
preferred type: Targets, Dashboard and Search badge only the check the server names in
`primary_check_id`, and the Dashboard row and SOC wall show that check's uptime. After a
save, the open Targets row badges the new primary check.

`wrong-password.test.mjs` covers Profile › Change Password with a wrong current password: the
server's 401 is shown on the page, which stays on Profile; any other 401 still goes to the login
page. It also checks that the Audit Log gives `login_failed` the failure badge colour.

`tag-delete.test.mjs` covers Settings › Tags: deleting a free tag, project, location or category
asks "Really delete this?", and only an accepted confirm sends the delete.
