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
json)`, or a function of the URL returning one of those or a promise of one, which the answer waits for),
`delays` (ms to hold back a path's answer) and `viewport`. It
returns `{ page, writes }`; writes are recorded and answered `{}`. A test fails if its page throws a
JavaScript error or asks for an API path no fixture answers. Fixture sets to spread into `fixtures`:
`moreTargets(count)` (a Dashboard, SOC wall and Targets list with `count` more healthy targets,
"Server 03" on, each with one ping check: enough rows to scroll and page; their detail and checks
aren't answered, so don't unfold them) and `staleScheduler()` (a stale scheduler, so the
header shows the red "Monitoring engine not responding" banner). Also: `helpButton(page)`,
`turnHelpOn(page)`, `waitForStyle(locator, prop, value)` and `orangeIn(page, selector)` (every use of
Bekci's orange in those elements, SVG fill and stroke included; help never uses it). So that an empty
answer means something, `orangeIn` fails when `selector` matches nothing, and when the same scan
doesn't find the header's orange (its positive control).

`help-anchors.test.mjs` (Node only, no browser) holds the help's anchors and content together: every
note in `src/help/content` has a `data-help="<page>.<key>"` in some view (or a key in an
`<page>Anchors` map in `src/help/anchors.js`), and every anchor has a note; a computed `:data-help`
must come from such a map. It also holds the help out of the main bundle: outside `src/help/`, only
`useHelp.js` and `anchors.js` may be imported from it statically (the rest loads with `import()`),
`useHelp.js` imports only Vue, and `anchors.js` imports nothing. A failure names the file and import
or the key.

`help-marks.test.mjs` covers the ? marks and notes on the Dashboard and the header: one mark per
documented control on screen (the first row only for a control on every row), in page order and then
the header's, each named "Help: <name>"; the bar's "Click any ? to read it" and its stepper ("12
notes", then "n of N" counting each key once). Unfolding a target adds its PRIMARY and check-row
marks. A mark opens its note (one at a time) with a turquoise leader line from the note's facing edge
to a dot on the control, which gets a turquoise outline; the note doesn't cover it, and nothing is
orange. The same mark again, or a click on the page, closes it. Keyboard: Tab goes mark to mark, Enter opens, ← and → step, Esc closes
the note (focus back on its mark) and then help; a text box keeps its own Esc and arrows, a checkbox
doesn't keep Esc. Marks follow a scroll (−200 px for `scrollBy(0, 200)`), a note stays inside a
1280×800 window, and a header note (the stepper's last stops) leaves the bar's ‹, › and Done clickable
at 1280×800 and 1440×900. Marks appear for rows that arrive after help was turned on (the Dashboard's
answer is held back until the header marks are drawn), and the stale-scheduler banner has a mark only
while it shows.

`help-pages.test.mjs` covers the notes of the pages after the Dashboard, a section per page:
Targets, Search, SLA, Alerts, Profile, the Audit Log, Users (Humans and API Access) and every
Settings tab (General, Tags, SLA, Alerting, Backup & Restore, Fail2Ban, System Health). Per page or
tab, the ? marks on screen equal the expected list in page order (then the header's), each named
"Help: <title>", each opens a note with that title (one at a time, its control outlined), the bar
says the page's summary, and none of help's own parts is orange. Switching a Settings tab or a Users
sub-tab while help is on swaps the summary and the marks (Fail2Ban and System Health each have their
own Refresh). Role limits: a viewer on Targets has no + Target and no Actions mark; a viewer sent
from Search or the Audit Log, and an operator sent from Users or Settings, to the Dashboard has none
of their marks. Drawn controls: the State, Type, Action, Status and Role notes' badges and the
Actions note's buttons have the app's own colours (checked against the table's own; the Audit Log
case lays its own five entries over the default answer). Controls that appear later get marks then:
PRIMARY after unfolding a Targets row, the pagination with 51 targets, Save and the result columns
once a Search has results, Last 4 hours after unfolding a result, Saved searches with a saved search
in localStorage, a new API token's card, and a closed Settings section's fields once it is opened
(Alerting's Email, Signal and Webhook; Backup's Full Database Backup). The Alerting test note is
the agreed wording and doesn't repeat the hint printed beside the buttons. A page's own dialog
(Pause, Unpause, Delete on Targets and Search; the SLA zoom; an audit entry; Edit and Suspend on
Users; the Config Restore confirmation) takes the page's marks away while it is open
(`data-help-scope="dialog"`) and they come back when it closes. The browser's own `confirm()`
(token revoke, tag and saved-backup delete) is never triggered here.

`help-switch.test.mjs` covers Help in the header: it comes after the last link each role sees, is
missing on the SOC wall and Login, and turns on a slate bar under the header (pushing the page down)
with the page's summary and "Written for Bekci <version>", plus "(running <version>)" when the server
runs another. Done, Esc, Help again and going to another page turn it off. Esc closes one thing per
press: a page window open while help is on (the SLA zoom, an audit entry) closes first and help stays
on, and Esc typed in a text box stays with the box. Each page's root names its
help page (`data-help-page`; Settings per tab), and the bar follows a tab change. Nothing in the bar or
the Help item is orange. When the bar's code can't be fetched (an old tab after a deploy; tried as a
404 and as the index.html a built Bekci serves for a missing file), it says "Help could not load:
reload the page." and the page keeps working.

`explain-rule.test.mjs` covers `explainRule`, `outageDelay` and `failCountTimeline` in `src/help/explainRule.js`
(pure functions, Node only, no browser). It pins the plain-words reading of a rule: the opening (with
"This target" for an empty name), one sentence per group (AND "have both", OR "any one of", the last
condition's operator winning), the subject and verb per check type for Status equals down and up (TCP
refused or accepted, swapped by Port must stay closed), the count phrase and the delay line in minutes or
seconds, and the fallback wording for a field or comparator the form doesn't offer. It also pins each
false-alarm check's text, level and order (no-recipient is a neutral note; a Fail Count 1 condition AND'd
with a failing test of another type is confirmed by it and not warned about, while one alone, in an OR group,
AND'd with its own type or only with a test that must pass (Status equals up) is), the Fail Count lane labels,
summary and the figures it used (an interval under
10 seconds runs every 10), and that a name like `<b>x</b>` comes back unchanged as a plain string.

`help-target-form.test.mjs` covers help inside the target form (New, Edit, Clone). With help on, on
Targets, the form opens with help on: its own help bar across the top of the overlay ("The target form
sets what Bekci watches…"; the page's bar under the header is hidden meanwhile), marks on its fields,
and the form's turquoise "? Help" turns help off and on. Every condition's Fail Count (and each other
per-condition or per-group control) has a mark, while the stepper counts each key once; a Fail Count
note uses its own condition's numbers in its text and eight-result timeline ("…on the second failed
check, 5 to 10 minutes in." for 300 seconds, Fail Count 2), Time Window shows Fail Count × Interval, and
AND / OR draws the form's own buttons; an interval of 5 seconds is shown as the 10 Bekci runs. The plain-words
card follows the form (for a ping alone in its group, Fail Count 1→3 reads "3 checks in a row" and drops the
single-failure warning; a ping on the latest check AND'd with the web page isn't warned about; groups are
joined by slate OR pills), says "No
conditions yet." for an empty rule, notes a Clone's empty recipient list without a warning sign, and
shows a name like `<b>x</b>` as typed. At 1440 the card sits left of the form and a note opens in the
right margin, its leader line running under the field and turning up into it at a right angle, crossing
no input, with the bar's ‹, › and Done left clickable; at 1100 the card sits above Condition Groups and
notes stay inside the window, off their field (below it when they fit). Typing into the field a note
describes keeps the note open and its text follows; pressing the overlay's own scrollbar (drawn by a
second Chrome started without `--hide-scrollbars`) doesn't close it. Removing a condition while a note is
open keeps the note on its own condition (focus goes back to its mark) or closes it when that condition
goes. Nothing in the card or the form's help is orange. Outside a field, a click on the control a note describes
closes the note: the header's health dots (the popover opens) and a Dashboard row (its checks unfold).
