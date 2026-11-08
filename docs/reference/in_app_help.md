# In-app help — reference

How the in-app help works in Bekci 3.5.0. Citations are `file:line` in the repository as of
07/11/2026. Frontend paths are under `frontend/src/` unless stated.

## What it is

Help is a turquoise layer over the live page, turned on with **Help** in the header. It has:

- **the help bar**: a slate strip under the header with the page's summary, a stepper through the
  notes, the version the help was written for, and Done;
- **? marks** beside each documented control, each opening a **note** with a leader line to the
  control;
- **target form help**: the same marks and notes inside the target form, plus the **plain-words
  card**, which reads the alert rule back as sentences and flags settings that cause false alarms;
- **rule recipes**: a window of 12 ready-made alert rules that "Start from a recipe" puts into the
  target form;
- **the rule lesson**: a longer explanation of alert rules.

Every page that uses the main layout has help (Dashboard, Targets, Search, SLA, Alerts, Profile,
Audit Log, Users with its API Access tab, and each Settings tab). The SOC wall and Login have none.

It is frontend only. It adds no API endpoint, setting, table or stored state: everything lives in a
reactive object in the page's memory (no `localStorage`, nothing sent to the server). The help reads
only `GET /api/system/health`, which the header already polls (`components/DefaultLayout.vue:36-46`).

## Parts and files

| File | What it is | Bundle |
|---|---|---|
| `help/useHelp.js` | The shared state, the keyboard handler, and the lazy loaders with their error components | main |
| `help/anchors.js` | Plain data only: `HEADER_PAGE` and `settingsGeneralAnchors` (`help/anchors.js:18-29`) | main |
| `help/applyConditions.js` | Puts a recipe into a form: `applyConditions`, `needOf`, `needError`; imports nothing | main |
| `conditionRules.js` | The target form's pure rules (default configs, `checkInterval`, `minFailWindow`, `failWindowError`, `valueError`), shared by the form, the card and the recipes | main |
| `help/HelpBar.vue` | The help bar | lazy |
| `help/HelpLayer.vue` + `help/locate.js` | Marks, notes, leader lines; the host of the recipes window and the lesson | lazy |
| `help/content/*.js` | The notes, one module per page; `content/index.js` merges them into `pages` | lazy (one chunk, named after `content/index.js`) |
| `help/content/version.js` | `HELP_VERSION = '3.5.0'` | lazy |
| `help/RuleCard.vue` + `help/explainRule.js` | The plain-words card and the pure function behind it | lazy |
| `help/recipes.js`, `help/RecipesWindow.vue`, `help/RuleDiagram.vue` | The 12 recipes, their window, the rule diagram | lazy |
| `help/LessonPanel.vue` | The rule lesson | lazy |

The tokens are defined once on `:root` in `assets/main.css:5-13` (`--help #0d9488`, `--help-dk
#0f766e`, `--help-tint #f0fdfa`, `--help-line #99f6e4`, the bar `#1e293b` / `#cbd5e1`, the glow
`0 0 0 5px rgba(20,184,166,.16)`, 6px on windows). Help never uses Bekci's orange `#ea580c`, and every
help box has a full border with no top or side stripe. Bekci's own controls drawn inside a note keep
their real colours. The bar's styles live in `main.css` (shared with the "could not load" strip); the
rest of help's CSS is scoped inside the lazy components.

### The lazy-load rule and its guard

Of `help/`, the main bundle holds only `useHelp.js`, `anchors.js` and `applyConditions.js`
(plus `conditionRules.js`, which sits in `src/`). Everything else reaches the page through
`defineAsyncComponent` (`help/useHelp.js:161-213`), so its code downloads the first time it is needed
and the SOC wall never loads it. Each loader has an error component: when a chunk can't be fetched
(typically an open tab after a deploy, whose chunk names no longer exist), the bar's place shows "Help
could not load: reload the page." with Done (`help/useHelp.js:151-158`), and the recipes window or
lesson shows the same text in a strip above the target form, with Close (`help/useHelp.js:176-191`).
The rest of the page keeps working.

`tests/help-anchors.test.mjs` holds the rule down (paths from `frontend/`):

- `:94-115`: outside `src/help/`, only `IMPORTABLE` (`useHelp.js`, `anchors.js`,
  `applyConditions.js`) is imported statically; `useHelp.js` imports only Vue; `anchors.js` imports
  nothing;
- `:117-154`: it walks every static import from `src/main.js` (through the router and the `.vue`
  files; `import()` is not followed) and fails if any other `src/help/` file is reached, naming who
  imported it. So a help module can't reach the main bundle through an allowed one either.

A built bundle is checked by looking for help-only strings: "Use this recipe for your monitoring
target" appears only in the `RecipesWindow-*.js` chunk, note texts only in the content chunk. Sizes
are in `reference/tech_stack.md` (Frontend Bundle).

## State (`help/useHelp.js`)

`help` (`help/useHelp.js:8-20`) holds: `on`; `page` (the help page on screen); `items` (the notes
with a mark on screen, in page order, each key once, kept by the layer); `current` (`{ id, n }`, the
open note and which of its key's marks); `runningVersion`; `recipesOpen`; `lessonOpen`; and
`formApply` (set by an open target form; while null the recipes window has no "Use this recipe").

- **On and off.** `toggleHelp()` / `closeHelp()` (`:75-96`). Help turns off on Done, on Esc (after
  everything above it has closed), on Help again, and on any route change; the route change also
  closes the recipes window and the lesson (`components/DefaultLayout.vue:120-126`). Leaving for a
  page without the layout (SOC, Login) unmounts it, which does the same (`:136-145`).
- **The page on screen.** `findPage()` (`help/useHelp.js:25-28`) takes the **last** element in document order
  carrying `data-help-page`. While help is on, a MutationObserver keeps it current (`help/useHelp.js:82-83`), so a
  Settings tab switch or the target form opening changes the page without a route change.
- **Keys** (`help/useHelp.js:53-73`), on `window`, listened for while help is on or a window is open (`:100-103`).
  A key already used by the page (`defaultPrevented`), or typed into a text field (inputs other than
  checkbox, radio and buttons; `textarea`, `select`, `contenteditable`; `help/useHelp.js:36-40`), is left alone.
  ← and → step through the notes while one is open and focus is on help's own parts.

### Esc, one layer per press

Help is not modal, so other windows can be open under or over it. Each press closes one thing:

1. a page's own window: the SLA zoom, an audit entry, Search's save box. Their handlers sit on
   `document`, run before help's (on `window`), and mark the key used (`preventDefault`);
2. the target form's "Replace the N conditions…" question: Esc is its Cancel
   (`components/TargetEditModal.vue:256-262`, also on `document`);
3. the recipes window, then the lesson under it, then the open note, then help itself
   (`help/useHelp.js:55-67`).

## Where help attaches: pages, scopes and anchors

- **Anchor.** A documented control carries `data-help="<page>.<key>"`, a static string. The note is
  item `key` of page `page` in `content/` (shape documented in `help/content/index.js:1-26`). A view
  that renders controls from a list takes keys from a `<page>Anchors` map in `anchors.js` (Settings ›
  General: `settingsGeneralAnchors`).
- **Page.** Each view's root names its help page with `data-help-page`: `dashboard`
  (`views/DashboardView.vue:332`), `targets` (`views/TargetsView.vue:305`), `search`
  (`views/SearchView.vue:403`), `sla` (`views/SlaView.vue:290`), `alerts` (`views/AlertsView.vue:52`),
  `profile` (`views/ProfileView.vue:54`). Settings computes its id from the tab on show
  (`views/SettingsView.vue:406-413`, root `:1180`): `settingsGeneral`, `settingsTags`, `settingsSla`,
  `settingsAlerting`, `settingsBackup`, `settingsFail2ban`, `settingsHealth`, `auditLog`, `users`,
  `apiAccess`; so the same key on two tabs (`refresh` on Fail2Ban and System Health) doesn't clash.
  The target form names `targetForm` (`components/TargetEditModal.vue:414-415`); it sits inside its
  view, so it wins while open. The header's controls use page `header` (`HEADER_PAGE`) and are added
  after the page's own on every page.
- **Scope.** Marks are looked for only inside the **last** element carrying `data-help-scope`
  (`help/locate.js:16-19`): `layout` (`components/DefaultLayout.vue:149`), `form` (the target form
  overlay), `dialog` (each page's own confirm or detail dialog, and the form's replace question),
  `recipes` and `lesson` (the two windows). So while a dialog or window is open, the marks of the page
  under it go, and they come back when it closes.
- **Which controls get a mark** (`help/locate.js:57-74`): those with a note for the page on screen or
  the header, and shown (`isShown`, `:26-51`: a box, `visibility: visible`, not clipped away by an
  ancestor with hidden or clipped overflow, and not inside a collapsed `.collapsible-body`). A
  repeated control gets a mark on its first shown instance only, unless its note says `each` (the
  target form's per-condition and per-group fields: a mark on every instance). Controls hidden by role
  (`v-if="auth.isOperator"` and the like) are simply absent, so there is no role list to keep in sync.
  Rows that arrive after help was turned on get their marks when they render.
- **API Access** (`views/SettingsView.vue`): Create Token in the page header has its own note
  (`apiAccess.create`). The Token name note (`apiAccess.name`) sits on the Create form's field while the
  form is open and on the table's Name header while it is closed: the form comes first in the page, so it
  takes the one mark. Both places read right with the same text.

### How a page adds help

1. Put `data-help-page="<page>"` on the view's root element.
2. Put `data-help="<page>.<key>"` on each documented control (a static string, or a key from a map in
   `anchors.js`).
3. Write the notes in a content module (`help/content/<page>.js`, exporting the page under its id)
   and spread it into `pages` in `help/content/index.js`.
4. Give the page's own dialogs `data-help-scope="dialog"`, and route their Esc through `document` with
   `preventDefault()` so help doesn't also close.
5. `tests/help-anchors.test.mjs:65-92` fails until every note has an anchor and every anchor a note;
   add the page's case to `tests/help-pages.test.mjs`. Every claim in a note must be true of the code.

## Marks and notes (`help/HelpLayer.vue`, `help/locate.js`)

- The layer is teleported to `<body>` and draws in window coordinates, so turning help on never
  moves the page (`help/HelpLayer.vue:335-397`). It sits above every app layer (marks and lines
  `z-index` 1100, the note 1101, the windows 1102), including the SLA zoom (1000); the scope rule
  decides which marks show.
- The layout mounts it while help is on **or** the recipes window or lesson is open
  (`components/DefaultLayout.vue:201`); marks and notes draw only while help is on
  (`help/HelpLayer.vue:91-97`).
- It re-measures on any scroll (captured), resize, size change (ResizeObserver), DOM change
  (MutationObserver) and transition end, at most once per frame (`:82-118`, `:307-319`). Changes inside
  the layer itself are ignored, except a help scope arriving or leaving there (the windows), so the
  page's marks go when a window's lazy chunk appears late (`:212-218`).
- **A mark**: an 18px `#0f766e` disc with "?", `aria-label="Help: <title>"`; right of the control by
  default, left of it, or on its top-right corner (`place: 'corner'`, for tight spots such as the
  PRIMARY badge) (`help/locate.js:130-142`). Tab reaches the marks in page order, the header's last
  (`help/HelpLayer.vue:137-141`).
- **A note**: one open at a time; title, one to three sentences, optional rows of drawn controls (the
  app's own classes), an optional timeline, an optional grey tip and an optional link (the lesson or
  the recipes); a footer with "‹ previous", "n of N" and "next ›" (`:357-393`). Everything is
  rendered with `{{ }}`; nothing uses `v-html`.
- **Placement** (`help/locate.js:162-190`): below the control, else right, above, left, the first that
  fits in the window without covering the control or the help bar's ‹, › and Done (`keepClear`,
  `:149-153`); if none fits, the roomier of below and above with the note scrolling inside. A scroll
  keeps an open note where it was relative to its control. In the target form at 1280px and wider the
  note takes the right margin instead (`marginSlot`, `placeInMargin`, `:195-218`), and its leader line
  runs under the field and turns up into it at a right angle, so it never crosses an input
  (`leaderLine`, `:226-243`).
- **The open control** gets a 2px turquoise outline with a faint glow (`data-help-on`). A note whose
  text depends on its control's figures reads them from `data-help-args` and follows them while open
  (the form's Fail Count and Time Window; `help/HelpLayer.vue:145-152`;
  `components/TargetEditModal.vue:741`, `:747`).
- **Closing.** The same mark again, Esc, or a click elsewhere on the page closes the note; help stays
  on (`help/HelpLayer.vue:242-254`). Not closing it: a click on help's own parts, a press on a
  scrollbar (the window's or a scrolling element's, such as the form overlay), and a click into a
  typing field (input, select, textarea or its label) inside the outlined control, so typing into the
  form's Fail Count keeps its note open. Any other click on the control closes the note, so it gets
  out of the way of what the click opens (the header's health popover, a Dashboard row's checks).
  When a note's control goes (a row folds, a tab switches, a condition is removed) the note closes;
  for a key on every instance it stays with its own instance while others are removed (`:125-132`).
- **Focus** moves into a note when it opens and back to its mark when it closes, unless it was closed
  by a click elsewhere (`:273-301`). A control out of view is scrolled to before its note opens.

## The help bar (`help/HelpBar.vue`)

- In normal flow under the header, full width, so it pushes the page down
  (`components/DefaultLayout.vue:199`). Inside the target form, which covers the header, the form
  draws its own copy across the top of its overlay and the layout's is hidden meanwhile
  (`components/TargetEditModal.vue:416`, `:1197`).
- Left: the page's summary (`help/HelpBar.vue:12`, `:27-29`). Right: "Click any ? to read it" and the
  stepper ‹ › while there are marks; the stepper reads "N notes" until a note opens, then "n of N",
  counting each key once (`:17-21`); "Written for Bekci 3.5.0", plus "(running X)" when the server
  reports another version (`:13-14`, `:39`); Done.

## Target form help

- The form overlay covers the header, so the form has its own turquoise "Help" button in its title
  row (`components/TargetEditModal.vue:422-425`). If help is already on when the form opens, it opens
  with help on.
- At 1280px and wider the plain-words card sits left of the form and notes open in the right margin;
  narrower, the card sits above Condition Groups and notes open beside their field, below it
  when they fit (`components/TargetEditModal.vue:187-201`, cards at `:417-418` and `:501-502`).
- Per-condition and per-group fields have a mark on every instance; the stepper counts each key once.
  The Fail Count note draws an eight-result timeline with that condition's own interval and count
  (`failCountTimeline`, `help/explainRule.js:70`), at the interval Bekci runs (`checkInterval`: blank =
  60, kept within 10 to 86400; `conditionRules.js:49-51`).
- New conditions still start at Fail Count 1 (`conditionRules.js:28-43`).

## The plain-words card (`help/RuleCard.vue`, `help/explainRule.js`)

Shown in the target form only while help is on; it follows the form as it changes. `explainRule(conditions,
targetName, recipientCount)` (`help/explainRule.js:230-312`) is pure and returns plain strings.

- **Reading.** "<name> counts as down when:" ("This target" while Name is empty), one sentence per
  group, groups joined by slate OR pills, and "It counts as up again on the first check where this is
  no longer true" (one group) / "…where none of these is true" (several). Groups follow the engine
  (`ruleGroups`, `:216`): OR between groups; inside a group the operator is the **last** condition's,
  empty = AND (`internal/engine/engine.go:76-88`); conditions come in group, then position order
  (`internal/store/rules.go:46`). A group's sentence names each check's subject and what happens
  (TCP refused/accepted swapped by Port must stay closed), and the count: "on the latest check" at
  Fail Count 1, "N checks in a row" above, kept apart from a phrase that ends in a number or an "or …"
  clause ("…not answered with 200 on 3 checks in a row"; `withCount`, `help/explainRule.js:162-168`). Fail Count > 1 adds a
  delay line, "(N−1)×interval to N×interval after it starts, at one check every <interval>", at the
  interval Bekci runs (`help/explainRule.js:23-25`).
- **False-alarm checks**, in this order (`help/explainRule.js:245-304`); ✓ green when met, ! amber when not:

| Shown when | Text |
|---|---|
| An AND group has two or more conditions of different check types | ✓ Group N needs two different tests to agree before it alerts. |
| Every condition that can flicker has Fail Count ≥ 2 | ✓ No test that can flicker alerts on a single failure. |
| Otherwise, for each condition that can flicker with Fail Count 1 | ! <check name> alerts on a single failed check. One blip will page people; 3 in a row is safer. |
| A group uses OR with two or more conditions | ! Group N alerts when any one of its tests fails. If they should agree first, switch it to AND. |
| A Page Hash "down" condition is not in an AND group with an HTTP "Status equals up" condition | ! Page Hash also fails when the site is simply down. Pair it with "web page answers" in an AND group (recipe: Defacement). |
| A TCP condition without Port must stay closed uses Status equals up | ! This alerts when the port opens, but the target's uptime and SLA will read close to 0%, … Tick Port must stay closed and use Status equals down instead (recipe: A port that must stay closed). |
| A TCP condition with Port must stay closed uses Status equals up | ! This alerts while the port is closed. To alert when it opens, use Status equals down. |
| A DNS condition has no Expected Value | ! The DNS check only fails if the lookup fails, not if the answer changes. |
| No alert recipients are ticked | (no sign) No one is ticked, so this target sends no email or Signal. The webhook, if it is switched on, still sends. |

  **Which conditions can flicker** (`help/explainRule.js:252-276`): every condition except Status equals up (a passing
  test, used to rule out outages, such as the S2 canary), so Response (ms) and Packet loss
  conditions count too. Left out is a condition in an AND group with a condition of **another check
  type that is not Status equals up and has Fail Count 2 or more**: that failing test must agree
  first, so one blip can't page on its own (a ping on the latest check AND'd with the web page on 2
  in a row pages only once the web page has failed twice). A partner that must be passing confirms
  nothing: Page Hash on the latest check AND'd with "web page answers" is still warned about. Two
  Fail Count 1 down conditions of different types AND'd confirm nothing, because a short network
  drop fails both at once: both are warned about, and the ✓ is not shown.
- An empty rule shows "No conditions yet." A name like `<b>x</b>` shows as typed.
- Buttons: "Rule recipes: ready-made rules" and "Learn alert rules: the full lesson".

## Rule recipes

### The data (`help/recipes.js`)

Twelve recipes on three shelves (Basic: one test; Combined: several tests; Security: catch tampering).
Each condition is in exactly the form's shape (`cond()`, `help/recipes.js:15-31`; config = the type's
defaults from `getDefaultConfig` plus the recipe's own). Each recipe also has display data (formula,
catches, read-as, situations, before-you-use-it), `preferred`, and `needs`: the fields the person must
fill in, which the data leaves empty.

| Id | Recipe | Conditions | Preferred | PRIMARY check | Needs |
|---|---|---|---|---|---|
| B1 | Reachable on the network | Ping, 60 s, down 3 in a row | ping | Ping | — |
| B2 | Website answers | HTTP, 60 s, down 3 in a row | http | Web page | — |
| B3 | Service port open | TCP, 60 s, down 3 in a row | tcp | Service port | port |
| B4 | Certificate renewal | TLS 443, warn 14 days, 600 s, down 3 in a row (window 1800) | tls_cert | Certificate | — |
| C1 | Server down, confirmed | Ping AND HTTP, each down 3 | http | Web page | — |
| C2 | Fast when sure, patient when not | (Ping AND HTTP, 3) OR HTTP "Web page, patient" down 5 | http | Web page | — |
| C3 | Web server and its certificate | (Ping AND HTTP, 3) OR TLS as B4 | http | Web page | — |
| C4 | Down or too slow | HTTP down 3 OR HTTP "Web page speed" Response (ms) > 5000, 5 | http | Web page | — |
| S1 | Defacement, without outage noise | Page Hash down 2 AND HTTP up 2, 300 s | http | Web page answers | — |
| S2 | DNS hijack | DNS at 8.8.8.8 down 2 AND DNS at 1.1.1.1 down 2 AND canary `one.one.one.one` at 1.1.1.1 expecting 1.1.1.1 up 1 | dns | DNS at Cloudflare | expected value, on both resolvers |
| S3 | A port that must stay closed | TCP 3389, Port must stay closed, 300 s, down 2 | tcp | Remote desktop port | — |
| S4 | Admin page that must stay hidden | HTTP endpoint (expect 404) down 2 AND HTTP up 2, 300 s | http | Admin page hidden | admin path, must start with `/` |

PRIMARY is the first check of the preferred type by name (`internal/store/checks.go:72-82`, checks in
name order, `:43`). Facts the recipe texts rely on: HTTP follows up to 9 redirects (CheckRedirect
refuses the 10th, `internal/checker/http.go:29-34`); an HTTP or Page Hash URL is the host plus one "/" and
the endpoint (`http.go:20`, `url.go`): a missing "/" is added, but an empty endpoint is the home page, hence S4's pattern; Page Hash is down for a changed page and
also for an unreachable or non-2xx one (`internal/checker/page_hash.go:38-86`), hence S1's pairing; a DNS
check with an empty query asks for the target's host, so S2's target must be a name, and a lookup that
fails is down too, so S2 also alerts when both resolvers fail to answer while the canary passes
(`internal/checker/dns.go:82-106`); with Port must stay closed any failed connection is up and a
connection down (`internal/checker/tcp.go:10-50`).

### `applyConditions` (`help/applyConditions.js`)

`applyConditions(form, conditions, preferred, needs)` (`:16-31`) always **replaces** the form's
conditions (never merges) with deep copies, blanks `check_id` (they save as new checks), puts each need
on its copied condition (never inside `config`), sets `preferred_check_type` for every recipe, and
returns the form's previous conditions and Preferred Check for Undo. JSON copies, not
`structuredClone`, because the form's values can be Vue proxies; so editing an applied condition can
never reach the recipe data. `needOf` / `needError` (`:35-51`) give a field's need and, while it is
empty or fails the need's pattern, the recipe's message.

### In the target form (`components/TargetEditModal.vue`)

- "Start from a recipe" sits next to "+ Add Group or Condition" on New, Clone and Edit, with help on
  or off (`:509`). It is disabled until Edit or Clone has loaded the target, and the form registers
  `help.formApply` only then (`:224`, `:275-281`). A load sequence number keeps a form closed while
  loading from ending the wait of the next one (`:381-409`).
- **Use this recipe** in the window calls the form (`help/RecipesWindow.vue:118-123`). A form that holds
  conditions first asks, in an in-app dialog (`components/TargetEditModal.vue:790-797`): "Replace the N conditions in this form with
  this recipe? On a saved target, Save then deletes the replaced checks and their history, SLA
  included." ("the 1 condition" for one), with Replace and Cancel; Esc is Cancel. The warning is true:
  saving a target deletes every check no longer in the form (`internal/store/targets.go:372-383`) and
  its results go with it (`ON DELETE CASCADE`). Nothing is sent until Save.
- After it goes in (`components/TargetEditModal.vue:240-246`), the form scrolls to Condition Groups and a turquoise banner says
  "Filled in from the recipe <name>. Fill in the fields marked in turquoise, then Save." (or "Check the
  settings, then Save.") with **Undo**, which puts back the previous conditions and Preferred Check,
  check ids included (`components/TargetEditModal.vue:266-271`, banner `:514-517`).
- A needed field is empty, dashed turquoise, with an italic placeholder; the marking clears once it
  has a value. Save is refused while one is empty (or S4's path doesn't start with "/"), through the
  form's custom-validity popup with the recipe's message and a `saveTarget` guard (`components/TargetEditModal.vue:127-128`, `:327`).
  Changing a recipe condition's check type drops its needs (`components/TargetEditModal.vue:96-101`).
- Save builds its payload field by field (`components/TargetEditModal.vue:329-356`), so the `needs` key never reaches the server.
  After that the form is an ordinary form.

### The window (`help/RecipesWindow.vue`)

Drawn by the help layer over the target form, with help on or off; 40px margins, a 2px turquoise
frame. The left column holds the three shelves; the right the chosen recipe: shelf, name, what it
catches, the rule diagram (`help/RuleDiagram.vue`: blue group boxes, AND/OR between chips, slate OR
pills, green chips for tests that must be passing, with a legend), "Read it as", "What alerts, and what
doesn't", "Before you use it", and "What it puts in the form" (a row per condition; it lists the timeout for HTTP, TCP and ping, the checks whose form shows a Timeout field, and not for Page Hash and TLS certificate, whose form has none). The footer's centred button "Use this
recipe for your monitoring target!" (at least 460px wide) shows only while a target form is open.
Opened elsewhere (from the lesson) the window is read-only: the footer says "To use a recipe, press +
Target, then Start from a recipe." to operators and admins, and nothing to viewers
(`help/RecipesWindow.vue:220-224`). Focus moves into the window and back to its opener on close
(`:125-137`). The window, like the lesson, has `aria-modal` but no Tab trap, as the app's own modals.

## The rule lesson (`help/LessonPanel.vue`)

Opened by "How alert rules work" in the Targets and Search State notes, "Learn alert rules: the full
lesson" in the form's Condition Groups note, and the card's button. Everyone can open it. Same frame
as the recipes window, its own help scope (`:99`). Six parts (`:41-48`): how a rule is built, with the
Mail Gateway rule drawn and read back by `explainRule` (the rule has recipe C3's settings, so the card
finds nothing to warn about; `:11-15`); Fail Count, with ten results of a flaky link drawn as three
lanes, Fail Count 1 "down twice, up twice" and Fail Count 3 "down once, up once" (changes of state, not
messages; part 2 says part 5 tells which changes send a message; `:60-71`); AND or OR inside a group;
groups never confirm each other; when people are told (a message on down and on recovery, one skipped
inside the cooldown, 30 minutes by default; a reminder every hour by default;
`internal/alerter/alerter.go:43-53`, defaults `internal/store/store.go:270-271`); and the mistakes that
cause false alarms. Its "Rule recipes" button opens the window on top, read-only (`help/LessonPanel.vue:202`).

## Versioning

`HELP_VERSION` (`help/content/version.js:3`) names the Bekci version the help text was last checked
against. When a release changes a page, its help content changes in the same commit; `HELP_VERSION` is
raised once every page's help has been re-checked for that release. The bar and both windows show
"Written for Bekci <HELP_VERSION>", so a reader can tell when the help may be behind the server.

## Limits

- The browser's own `confirm()` (Settings › Tags delete, API token revoke, saved-backup delete) draws
  above every page layer; help can't sit over it.
- A note's link to a window the page can't open simply isn't shown.
- An open tab from before a deploy can't load help's chunks: it says to reload.

## Tests

All from `frontend/`, one file at a time (`node --test --test-concurrency=1 tests/<file>.test.mjs`);
browser tests answer every API call from fixtures (`tests/help-fixtures.mjs`), so no backend or live data
is used. Details per file: `frontend/tests/README.md`.

| File | Holds down |
|---|---|
| `help-anchors.test.mjs` (Node) | Every note has an anchor and every anchor a note; `:data-help` only from `anchors.js` maps; the main-bundle import rule and the static import walk; every Settings › General field has an anchor key |
| `help-switch.test.mjs` | The Help item per role, absent on SOC and Login; the bar, its stamp and "(running X)"; off on Done, Esc, Help, navigation; Esc layering with page windows; the "could not load" strip |
| `help-marks.test.mjs` | Marks and notes on the Dashboard and header: order, stepper, leader line, outline, placement, keyboard, scroll, late rows, the stale banner |
| `help-pages.test.mjs` | Every other page and Settings tab: marks in order, titles, summaries, role limits, drawn controls' colours, controls that appear later, dialogs taking the marks away, pinned wordings |
| `explain-rule.test.mjs` (Node) | The card's reading, count phrase, delay, every false-alarm check's text, level and order, and the Fail Count timeline |
| `help-target-form.test.mjs` | Help in New, Edit and Clone; per-condition marks; live Fail Count and Time Window notes; the card at 1440 and 1100; margin notes and their bent line; click and scrollbar behaviour |
| `recipes.test.mjs` (Node) | The 12 recipes' shape, validity, PRIMARY check, needs, and no card warning; `applyConditions` replaces, copies, returns the previous values |
| `recipes-window.test.mjs` | The window, using a recipe on New, Clone and Edit, the replace question and its Esc, Undo, needed fields blocking Save, the load gate, no `needs` in any request |
| `help-lesson.test.mjs` | The lesson's openers, frame, parts, example, lanes, read-only recipes from the lesson, Esc and focus |
| `condition-rules.test.mjs` (Node) | The form's pure rules the card and recipes share |

Every browser file also checks that nothing in help is orange (`orangeIn`, with a positive control).
