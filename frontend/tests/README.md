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
page, including Profile Save with an expired session, while the SOC wall (not signed in) stays on
`/soc`. A failed password change or Profile Save after a successful one shows only the new error.
It also checks that the Audit Log gives `login_failed` the failure badge colour.

`tag-delete.test.mjs` covers Settings › Tags: deleting a free tag, project, location or category
asks "Really delete this?", and only an accepted confirm sends the delete.

`auto-backup-time.test.mjs` covers Settings › Backup & Restore › Automated Backups: the time is a
dropdown of the 24 whole hours, shows the saved hour, and Save sends the one chosen. A time saved
before whole hours shows as the hour the server runs it in (`04:30` → `04:00`, `25:00` or empty → `03:00`).

`sla-uptime.test.mjs` covers the SLA page's Not Meeting SLA, category and All cards: they use each
host's `uptime_90d_pct` from the server (the Dashboard's pooled figure), one value per host, and
leave out hosts without data (-1), instead of averaging the daily percentages.

`tcp-expect-closed.test.mjs` covers the target editor's TCP check: "Port must stay closed" is
off for a new check, ticking it shows its hint and Save sends `expect_closed: true` with the
default rule (Status equals down); a saved check with the option opens ticked, and unticking it
saves `expect_closed: false`.

`packet-loss-rule.test.mjs` covers the target editor's "Alert when..." Field for ping: Packet loss (%)
is offered for ping only, and choosing it sets greater than with an empty Value; Save is refused until
Value is a number from 0 to 100, and the hint gives the steps for the Ping Count (3 pings: 33.3%;
greater than 30 = 1 or more lost). Leaving ping resets the rule to Status equals down. Response (ms)
gets the same greater than / number rule, also for a saved rule whose value is not a number.

`condition-rules.test.mjs` (Node only, no browser) pins the target form's pure validation rules in
`src/conditionRules.js`: the TCP default config, the failure-window minimum and its error text, the
number check for Response (ms) and Packet loss (%), and the packet-loss hint.

`targets-tag-filter.test.mjs` covers the Targets filter bar: a Tags chip has only its Remove button
(no pin), and tags, Location and Project combine so each one narrows the list; removing a chip
widens it again.

`search-confirm-dialog.test.mjs` covers the Search page's Delete, Pause and Unpause dialogs: each is
the same white card as the Targets page's dialogs (compared with one opened there), opens level with
the button that was clicked and stays inside the window, Cancel sends nothing, and confirming
Unpause unpauses the row it was opened for.

`help-fixtures.mjs` is not a test file: it is the shared set-up the in-app help tests
(`help-*.test.mjs`) import. `startApp()` / `stopApp(app)` start and stop a Vite server and Chrome.
`openPage(t, app, path, options)` opens a page signed in as `role` (`'admin'`, `'operator'`,
`'viewer'`, or `null` for signed out) with every API request answered from `apiFixtures()`: three
users, two targets (Core switch, healthy; Web shop, down, with a PRIMARY HTTP check) and the data each
page loads. Options: `version` (what `/api/system/health` reports; defaults to `HELP_VERSION`),
`fixtures` (answers laid over the defaults, by GET path or `"METHOD /path"`; a value, `reply(status,
json)`, or a function of the URL), `delays` (ms to hold back a path's answer) and `viewport`. It
returns `{ page, writes }`; writes are recorded and answered `{}`. A test fails if its page throws a
JavaScript error or asks for an API path no fixture answers. Also: `helpButton(page)`,
`turnHelpOn(page)`, `waitForStyle(locator, prop, value)` and `orangeIn(page, selector)` (every use of
Bekci's orange in those elements; help never uses it).

`help-switch.test.mjs` covers Help in the header: it comes after the last link each role sees, is
missing on the SOC wall and Login, and turns on a slate bar under the header (pushing the page down)
with the page's summary and "Written for Bekci <version>", plus "(running <version>)" when the server
runs another. Done, Esc, Help again and going to another page turn it off. Each page's root names its
help page (`data-help-page`; Settings per tab), and the bar follows a tab change. Nothing in the bar or
the Help item is orange. When the bar's code can't be fetched (an old tab after a deploy; tried as a
404 and as the index.html a built Bekci serves for a missing file), it says "Help could not load:
reload the page." and the page keeps working.
