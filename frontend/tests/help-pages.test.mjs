// The help notes of the pages after the Dashboard (design in_app_help.md §7.3 and
// §7.5-7.11): per page, which controls get a ? mark, that every mark opens a note
// with its title, that role-limited controls get no mark for people who can't see
// them, that a page's own dialog hides the page's marks, and that help's own
// chrome stays turquoise. The Dashboard and the header are in help-marks.test.mjs.
// Sections: Targets, Search, SLA, Alerts, Profile, Audit Log, Users, Settings.
import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { apiFixtures, moreTargets, openPage, orangeIn, startApp, stopApp, turnHelpOn } from './help-fixtures.mjs'

let app
before(async () => { app = await startApp() })
after(() => stopApp(app))

const mark = (page, id) => page.locator(`.help-mark[data-help-mark="${id}"]`)
const markIds = page => page.locator('.help-mark').evaluateAll(els => els.map(e => e.dataset.helpMark))
const note = page => page.locator('.help-note')
const stepper = page => page.locator('.help-bar .help-stepper-pos')
const waitForTitle = (page, title) => page.waitForFunction(title => {
  const note = document.querySelector('.help-note')
  return note?.querySelector('.help-note-title').textContent.trim() === title
    && getComputedStyle(note).visibility === 'visible'
}, title, { timeout: 2000 })
const waitForMarks = (page, n) => page.waitForFunction(n => document.querySelectorAll('.help-mark').length === n, n,
  { timeout: 2000 })
const HEADER = ['header.version', 'header.dots', 'header.user']

// Opens `path` with help on and waits until the marks stop changing.
async function withHelp(t, path, options = {}, ready) {
  const { page, writes } = await openPage(t, app, path, { role: 'operator', ...options })
  await ready(page)
  await turnHelpOn(page)
  await mark(page, 'header.user').waitFor()
  return { page, writes }
}

// Every mark of `expected` ([id, title] pairs, in page order) is on screen, and
// each opens its note with that title; the bar counts them; nothing is orange.
async function checkMarks(page, expected) {
  const ids = [...expected.map(([id]) => id), ...HEADER]
  await waitForMarks(page, ids.length)
  assert.deepEqual(await markIds(page), ids)
  assert.equal((await stepper(page).innerText()).trim(), `${ids.length} notes`)
  for (const [id, title] of expected) {
    assert.equal(await mark(page, id).getAttribute('aria-label'), `Help: ${title}`)
    await mark(page, id).click()
    await waitForTitle(page, title)
    assert.equal(await note(page).count(), 1, 'one note at a time')
    assert.ok((await note(page).locator('.help-note-text').first().innerText()).trim().length > 0, `${id} has text`)
    assert.equal(await page.locator('[data-help-on]').count(), 1, `${id}: its control is outlined`)
    // An open note can sit over a neighbour's mark: close it before the next
    await page.keyboard.press('Escape')
    await note(page).waitFor({ state: 'detached' })
  }
  // Help is turquoise; the drawn app controls inside notes keep their own colours,
  // and none of the notes drawn here is orange by nature (no + Target / Save rows)
  await mark(page, expected[0][0]).click()
  await note(page).waitFor()
  assert.deepEqual(await orangeIn(page, '.help-mark, .help-note, .help-lines, .help-bar'), [])
  await page.keyboard.press('Escape')
}

const bar = page => page.locator('.help-bar-text').innerText()

// ── Targets ──────────────────────────────────────────────────────────────────

const TARGETS = [
  ['targets.add', '+ Target'], ['targets.cats', 'Categories'], ['targets.filt', 'Project, Location and Tags'],
  ['targets.state', 'State'], ['targets.cond', 'Conditions'], ['targets.en', 'Enabled'], ['targets.acts', 'Actions'],
  ['targets.row', 'Target rows'], ['targets.guide', 'Target creation guide'],
]
const targetsReady = page => page.locator('tbody tr').first().waitFor()

test('Targets: a mark for each documented control, each opens its note, nothing orange; the bar says what the page is', async t => {
  const { page } = await withHelp(t, '/targets', {}, targetsReady)
  assert.match(await bar(page), /^Targets lists every system Bekci watches/)
  await checkMarks(page, TARGETS)
})

test('Targets: unfolding a row adds the PRIMARY mark; 51 targets add the pagination mark', async t => {
  const { page } = await withHelp(t, '/targets', { fixtures: moreTargets(49) }, targetsReady)
  await mark(page, 'targets.pages').waitFor()
  await page.locator('tbody tr').first().click()
  await mark(page, 'targets.prim').waitFor()
  const ids = TARGETS.map(([id]) => id)
  ids.splice(ids.indexOf('targets.guide'), 0, 'targets.prim', 'targets.pages') // after the row, before the guide
  await waitForMarks(page, ids.length + HEADER.length)
  assert.deepEqual(await markIds(page), [...ids, ...HEADER])
  await mark(page, 'targets.prim').click()
  await waitForTitle(page, 'PRIMARY')
  await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight)) // 50 rows: the pagination is below the fold
  await mark(page, 'targets.pages').click()
  await waitForTitle(page, 'Pages')
  assert.match(await note(page).innerText(), /50 targets per page/)
})

test('Targets: the State note draws the badges of the table, the Actions note its buttons', async t => {
  const { page } = await withHelp(t, '/targets', {}, targetsReady)
  await mark(page, 'targets.state').click()
  await waitForTitle(page, 'State')
  const row = label => note(page).locator('.help-note-row').filter({ has: page.locator('.help-note-control > span', { hasText: new RegExp(`^${label}$`) }) })
  const background = (label) => row(label).locator('.badge').evaluate(el => getComputedStyle(el).backgroundColor)
  assert.equal(await background('healthy'), 'rgb(220, 252, 231)')
  assert.equal(await background('unhealthy'), 'rgb(254, 226, 226)')
  assert.equal(await background('paused'), 'rgb(224, 231, 255)', 'paused: the app\'s indigo, drawn in the layer')
  // "—" is plain muted text on the page (no rule: no checks), so it is drawn as text, not a pill
  const dash = row('—')
  assert.equal(await dash.locator('.badge').count(), 0, '— is not a badge')
  assert.equal(await dash.locator('.help-note-control > span').evaluate(el => getComputedStyle(el).backgroundColor),
    'rgba(0, 0, 0, 0)')
  assert.match(await note(page).innerText(), /No checks, so there is no rule to judge it by/)
  assert.match(await note(page).innerText(), /A new target starts healthy until its checks report/)
  assert.doesNotMatch(await note(page).innerText(), /Not evaluated yet/)
  // The same pictures as the table's own badges
  const table = await page.locator('tbody .badge-active').first().evaluate(el => getComputedStyle(el).backgroundColor)
  assert.equal(table, await background('healthy'))
  assert.equal(await note(page).locator('.help-note-link').count(), 1, 'the lesson link shows')
  assert.equal((await note(page).locator('.help-note-link').innerText()).trim(), 'How alert rules work')
  await mark(page, 'targets.cond').click()
  await waitForTitle(page, 'Conditions')
  assert.match(await note(page).innerText(), /How many checks this target has\./)
  await mark(page, 'targets.acts').click()
  await waitForTitle(page, 'Actions')
  const pauseRow = await note(page).locator('.help-note-row').filter({ has: page.locator('.btn', { hasText: /^Pause$/ }) }).innerText()
  assert.match(pauseRow, /a Run Now still can/)
  const unpauseRow = await note(page).locator('.help-note-row').filter({ has: page.locator('.btn', { hasText: /^Unpause$/ }) }).innerText()
  assert.match(unpauseRow, /Bekci asks first/)
  const button = label => note(page).locator('.help-note-row .btn', { hasText: new RegExp(`^${label}$`) })
  for (const label of ['Pause', 'Unpause', 'Run Now', 'Edit', 'Clone', 'Delete']) assert.equal(await button(label).count(), 1, label)
  const tableDelete = await page.locator('tbody .btn-danger').first().evaluate(el => getComputedStyle(el).color)
  assert.equal(await button('Delete').evaluate(el => getComputedStyle(el).color), tableDelete)
  assert.equal(await button('Pause').evaluate(el => getComputedStyle(el).color), 'rgb(67, 56, 202)')
  assert.equal(await button('Unpause').evaluate(el => getComputedStyle(el).color), 'rgb(22, 101, 52)')
})

test('Targets as a viewer: no + Target mark and no Actions mark', async t => {
  const { page } = await withHelp(t, '/targets', { role: 'viewer' }, targetsReady)
  await waitForMarks(page, TARGETS.length - 2 + HEADER.length)
  const ids = await markIds(page)
  assert.ok(!ids.includes('targets.add'))
  assert.ok(!ids.includes('targets.acts'))
  assert.deepEqual(ids, [...TARGETS.map(([id]) => id).filter(id => id !== 'targets.add' && id !== 'targets.acts'), ...HEADER])
})

// A dialog the page opens: its marks are gone while it is open and back after Cancel.
async function dialogHidesMarks(page, buttonIn, button, heading, marks) {
  await waitForMarks(page, marks)
  await page.locator(`${buttonIn} .btn`, { hasText: new RegExp(`^${button}$`) }).first().click()
  await page.locator('.modal-card h3', { hasText: heading }).waitFor()
  await waitForMarks(page, 0)
  assert.equal(await page.locator('[aria-label^="Help: "]').count(), 0, `${heading}: no mark over the dialog`)
  await page.locator('.modal-card .btn', { hasText: /^Cancel$/ }).click()
  await waitForMarks(page, marks)
}

// The first fixture target, paused: its row offers Unpause
const pausedFirst = () => apiFixtures()['/api/targets'].map((t, i) =>
  i === 0 ? { ...t, paused_at: new Date().toISOString() } : t)

test('Targets: while a Pause, Delete or Unpause dialog is open the page\'s marks are gone; they come back when it closes', async t => {
  const { page } = await withHelp(t, '/targets', {}, targetsReady)
  const marks = TARGETS.length + HEADER.length
  await dialogHidesMarks(page, 'tbody .actions', 'Pause', 'Pause Target', marks)
  await dialogHidesMarks(page, 'tbody .actions', 'Delete', 'Delete Target', marks)
})

test('Targets: the Unpause dialog hides the marks too', async t => {
  const { page } = await withHelp(t, '/targets', { fixtures: { '/api/targets': pausedFirst() } }, targetsReady)
  await dialogHidesMarks(page, 'tbody .actions', 'Unpause', 'Unpause Target', TARGETS.length + HEADER.length)
})

// ── Search ───────────────────────────────────────────────────────────────────

const SEARCH_BOX = [['search.q', 'Search box'], ['search.filters', 'Filters']]
const SEARCH_RESULTS = [['search.save', '+ Save'], ['search.state', 'State'], ['search.acts', 'Actions'],
  ['search.row', 'Results']]
const searchReady = page => page.locator('.search-input').waitFor()
const searchFor = async (page, text) => {
  await page.locator('.search-input').fill(text)
  await page.locator('tbody tr').first().waitFor()
}

test('Search: the search box and filters have marks; results add Save, State, Actions and the first result', async t => {
  const { page } = await withHelp(t, '/search', {}, searchReady)
  assert.match(await bar(page), /^Search finds targets by name or host/)
  await waitForMarks(page, SEARCH_BOX.length + HEADER.length)
  assert.deepEqual(await markIds(page), [...SEARCH_BOX.map(([id]) => id), ...HEADER])
  await searchFor(page, 'core')
  await checkMarks(page, [...SEARCH_BOX, ...SEARCH_RESULTS])
})

test('Search: unfolding a result adds the Last 4 hours mark; saved searches have theirs', async t => {
  const { page } = await withHelp(t, '/search', {}, async page => {
    await searchReady(page)
    await page.evaluate(() => localStorage.setItem('bekci_saved_searches',
      JSON.stringify([{ id: '1', name: 'Switches', text: 'switch', project: '', location: '', tags: [] }])))
    await page.reload()
    await searchReady(page)
  })
  await mark(page, 'search.saved').waitFor()
  assert.deepEqual(await markIds(page), ['search.q', 'search.filters', 'search.saved', ...HEADER])
  await page.locator('.saved-chip').click() // the saved search fills the box
  await page.locator('tbody tr').first().waitFor()
  await page.locator('tbody tr').first().click()
  await mark(page, 'search.strip').waitFor()
  assert.deepEqual(await markIds(page), ['search.q', 'search.filters', 'search.save', 'search.saved', 'search.state',
    'search.acts', 'search.row', 'search.strip', ...HEADER])
  await mark(page, 'search.strip').click()
  await waitForTitle(page, 'Last 4 hours')
  assert.equal(await mark(page, 'search.saved').getAttribute('aria-label'), 'Help: Saved searches')
  await mark(page, 'search.saved').click()
  await waitForTitle(page, 'Saved searches')
  await page.keyboard.press('Escape')
  await note(page).waitFor({ state: 'detached' })
  await mark(page, 'search.save').click()
  await waitForTitle(page, '+ Save')
  assert.match(await note(page).innerText(), /this browser only/)
})

test('Search: the State note draws the table\'s badges; Pause, Delete and Unpause dialogs hide the marks', async t => {
  const { page } = await withHelp(t, '/search', { fixtures: { '/api/targets': pausedFirst() } }, searchReady)
  await searchFor(page, 'e')
  const marks = SEARCH_BOX.length + SEARCH_RESULTS.length + HEADER.length
  await waitForMarks(page, marks)
  await mark(page, 'search.state').click()
  await waitForTitle(page, 'State')
  assert.equal(await note(page).locator('.badge', { hasText: /^paused$/ }).evaluate(el => getComputedStyle(el).backgroundColor),
    'rgb(224, 231, 255)')
  assert.equal(await note(page).locator('.badge').filter({ hasText: /^—$/ }).count(), 0, '— is not drawn as a badge')
  await page.keyboard.press('Escape')
  await note(page).waitFor({ state: 'detached' })
  // Delete is grey on the Search page (no .btn-danger rule there): the note draws it the same
  await mark(page, 'search.acts').click()
  await waitForTitle(page, 'Actions')
  const live = await page.locator('tbody .actions .btn', { hasText: /^Delete$/ }).first().evaluate(el => getComputedStyle(el).color + getComputedStyle(el).borderTopColor)
  assert.equal(await note(page).locator('.help-note-row .btn', { hasText: /^Delete$/ }).evaluate(el => getComputedStyle(el).color + getComputedStyle(el).borderTopColor), live)
  await page.keyboard.press('Escape')
  await note(page).waitFor({ state: 'detached' })
  await dialogHidesMarks(page, 'tbody .actions', 'Unpause', 'Unpause Target', marks)
  await dialogHidesMarks(page, 'tbody tr:nth-child(2) .actions', 'Pause', 'Pause Target', marks)
  await dialogHidesMarks(page, 'tbody tr:nth-child(2) .actions', 'Delete', 'Delete Target', marks)
})

test('Search is for operators and admins: a viewer is sent to the Dashboard and has no Search marks', async t => {
  const { page } = await openPage(t, app, '/search', { role: 'viewer' })
  await page.locator('.target-card').first().waitFor()
  assert.equal(new URL(page.url()).pathname, '/')
  await turnHelpOn(page)
  await mark(page, 'header.user').waitFor()
  assert.deepEqual((await markIds(page)).filter(id => id.startsWith('search.')), [])
})

// ── SLA ──────────────────────────────────────────────────────────────────────

const SLA = [['sla.hosts', 'Hosts Monitored'], ['sla.notmeet', 'Not Meeting SLA'], ['sla.avg', 'Averages'],
  ['sla.planned', 'Planned Work'], ['sla.chart', 'Category charts']]
const slaReady = page => page.locator('.sla-card canvas').first().waitFor()

test('SLA: a mark for each card and the charts; each opens its note; nothing orange', async t => {
  const { page } = await withHelp(t, '/sla', {}, slaReady)
  assert.match(await bar(page), /^SLA shows each category's daily uptime over the last 90 days/)
  await checkMarks(page, SLA)
})

test('SLA: the enlarged chart hides the page\'s marks and names its own; Esc closes it and the marks return', async t => {
  const { page } = await withHelp(t, '/sla', {}, slaReady)
  await waitForMarks(page, SLA.length + HEADER.length)
  await page.locator('.sla-card.clickable').first().click()
  await page.locator('.modal-content').waitFor()
  await page.waitForFunction(() => document.querySelectorAll('.help-mark').length === 1, null, { timeout: 2000 })
  assert.deepEqual(await markIds(page), ['sla.zoom'])
  // The help layer draws above the zoom window (z-index 1000): the mark is clickable, its note on top
  const topmost = (sel) => page.evaluate(sel => {
    const el = document.querySelector(sel)
    const b = el.getBoundingClientRect()
    const at = document.elementFromPoint(b.x + b.width / 2, b.y + b.height / 2)
    return { hit: el.contains(at), inside: b.left >= 0 && b.top >= 0 && b.right <= innerWidth && b.bottom <= innerHeight }
  }, sel)
  assert.deepEqual(await topmost('.help-mark[data-help-mark="sla.zoom"]'), { hit: true, inside: true })
  await mark(page, 'sla.zoom').click()
  await waitForTitle(page, 'Enlarged chart')
  assert.deepEqual(await topmost('.help-note'), { hit: true, inside: true })
  const z = sel => page.locator(sel).evaluate(el => Number(getComputedStyle(el).zIndex))
  assert.ok(await z('.help-lines') > 1000 && await z('.help-note') > await z('.help-lines'), 'line above the zoom, note above the line')
  await page.keyboard.press('Escape') // the window closes first, and the note with its control
  await page.locator('.modal-content').waitFor({ state: 'detached' })
  assert.equal(await page.locator('.help-bar').count(), 1, 'the window closed first; help stays on')
  await waitForMarks(page, SLA.length + HEADER.length)
})

// ── Alerts ───────────────────────────────────────────────────────────────────

const ALERTS = [['alerts.audit', 'Total'], ['alerts.type', 'Type'], ['alerts.rcp', 'Recipient'], ['alerts.msg', 'Message']]

test('Alerts: a mark for the total and the Type, Recipient and Message columns; each opens its note', async t => {
  const { page } = await withHelp(t, '/alerts', {}, page => page.locator('tbody tr').first().waitFor())
  assert.match(await bar(page), /^Alerts lists every alert message Bekci tried to send/)
  await checkMarks(page, ALERTS)
})

test('Alerts: the Type note draws the three badges of the table', async t => {
  const { page } = await withHelp(t, '/alerts', {}, page => page.locator('tbody tr').first().waitFor())
  await mark(page, 'alerts.type').click()
  await waitForTitle(page, 'Type')
  const background = label => note(page).locator('.badge', { hasText: new RegExp(`^${label}$`) })
    .evaluate(el => getComputedStyle(el).backgroundColor)
  assert.equal(await background('firing'), 'rgb(254, 226, 226)')
  assert.equal(await background('recovery'), 'rgb(220, 252, 231)')
  assert.equal(await background('re-alert'), 'rgb(254, 243, 199)')
  const closeNote = async () => { await page.keyboard.press('Escape'); await note(page).waitFor({ state: 'detached' }) }
  await closeNote()
  await mark(page, 'alerts.audit').click()
  await waitForTitle(page, 'Total')
  assert.match(await note(page).innerText(), /webhook failures are also in the Audit Log/)
  assert.match(await note(page).innerText(), /Email and Signal delivery failures are only in the server log/)
  await closeNote()
  await mark(page, 'alerts.msg').click()
  await waitForTitle(page, 'Message')
  assert.match(await note(page).innerText(), /RE-ALERT/)
  await closeNote()
  await mark(page, 'alerts.type').click()
  await waitForTitle(page, 'Type')
  const table = await page.locator('tbody .badge-firing').evaluate(el => getComputedStyle(el).backgroundColor)
  assert.equal(table, await background('firing'))
})

test('Alerts as a viewer has the same marks', async t => {
  const { page } = await withHelp(t, '/alerts', { role: 'viewer' }, page => page.locator('tbody tr').first().waitFor())
  await waitForMarks(page, ALERTS.length + HEADER.length)
  assert.deepEqual(await markIds(page), [...ALERTS.map(([id]) => id), ...HEADER])
})

// ── Profile ──────────────────────────────────────────────────────────────────

const PROFILE = [['profile.role', 'Role'], ['profile.email', 'Email'], ['profile.phone', 'Phone'],
  ['profile.save', 'Save'], ['profile.pw', 'New Password'], ['profile.change', 'Change Password']]

test('Profile: a mark for Role, Email, Phone, Save, New Password and Change Password; each opens its note', async t => {
  const { page } = await withHelp(t, '/profile', {}, page => page.locator('input[type=tel]').waitFor())
  assert.match(await bar(page), /^Profile shows your account/)
  await checkMarks(page, PROFILE)
})

test('Profile as a viewer has the same marks', async t => {
  const { page } = await withHelp(t, '/profile', { role: 'viewer' }, page => page.locator('input[type=tel]').waitFor())
  await waitForMarks(page, PROFILE.length + HEADER.length)
  assert.deepEqual(await markIds(page), [...PROFILE.map(([id]) => id), ...HEADER])
})

// Waits until the marks on screen are exactly `ids` (in order); fails showing the difference.
const waitForIds = (page, ids) => page.waitForFunction(ids => JSON.stringify(
  [...document.querySelectorAll('.help-mark')].map(e => e.dataset.helpMark)) === JSON.stringify(ids), ids, { timeout: 2000 })
  .catch(async () => assert.deepEqual(await markIds(page), ids))
const idsOf = list => list.map(([id]) => id)

// ── Audit Log ────────────────────────────────────────────────────────────────

const AUDIT = [['auditLog.audit', 'Events'], ['auditLog.q', 'Search'], ['auditLog.user', 'User'],
  ['auditLog.action', 'Action'], ['auditLog.res', 'Resource'], ['auditLog.status', 'Status'], ['auditLog.ip', 'IP'],
  ['auditLog.row', 'Audit entries']]
const auditReady = page => page.locator('.audit-row').first().waitFor()

// One entry per colour of the Action column, and a failure
const auditFixture = () => {
  const entry = (id, username, action, status) => ({
    id, user_id: `u-${username}`, username, action, resource_type: 'user', resource_id: 'a1b2c3d4-0000-4000-8000-000000000000',
    detail: '', ip_address: '192.0.2.10', status, created_at: new Date(Date.now() - id * 60_000).toISOString(),
  })
  const entries = [entry(1, 'admin', 'login', 'success'), entry(2, 'mallory', 'login_failed', 'failure'),
    entry(3, 'admin', 'create_user', 'success'), entry(4, 'admin', 'delete_tag', 'success'),
    entry(5, 'operator', 'pause_target', 'success')]
  return { entries, total: entries.length, page: 1, limit: 50 }
}

test('Audit Log as an operator: the bar says what it is; a mark for the count, the search box, each column and the first row', async t => {
  const { page } = await withHelp(t, '/audit-log', {}, auditReady)
  assert.match(await bar(page), /^Audit Log is a read-only record of who did what and when/)
  await checkMarks(page, AUDIT)
})

test('Audit Log: the Action note draws the table\'s badge colours, the Status note its words', async t => {
  const { page } = await withHelp(t, '/audit-log', { fixtures: { '/api/audit-log': auditFixture() } }, auditReady)
  const colours = el => `${getComputedStyle(el).backgroundColor} ${getComputedStyle(el).color}`
  await mark(page, 'auditLog.action').click()
  await waitForTitle(page, 'Action')
  for (const action of ['login_failed', 'login', 'create_user', 'delete_tag', 'pause_target']) {
    const drawn = note(page).locator('.help-note-row .badge', { hasText: new RegExp(`^${action}$`) })
    const table = page.locator('tbody .badge', { hasText: new RegExp(`^${action}$`) })
    assert.equal(await drawn.evaluate(colours), await table.evaluate(colours), action)
  }
  const background = action => note(page).locator('.badge', { hasText: new RegExp(`^${action}$`) })
    .evaluate(el => getComputedStyle(el).backgroundColor)
  assert.equal(await background('login_failed'), 'rgb(254, 226, 226)', 'login_failed is red')
  assert.equal(await background('login'), 'rgb(219, 234, 254)', 'login is blue')
  await page.keyboard.press('Escape')
  await note(page).waitFor({ state: 'detached' })
  await mark(page, 'auditLog.status').click()
  await waitForTitle(page, 'Status')
  for (const status of ['success', 'failure']) {
    const drawn = note(page).locator('.help-note-control > span', { hasText: new RegExp(`^${status}$`) })
    const table = page.locator('tbody td > span', { hasText: new RegExp(`^${status}$`) }).first()
    assert.equal(await drawn.evaluate(colours), await table.evaluate(colours), status)
  }
  assert.equal(await note(page).locator('.status-fail').evaluate(el => getComputedStyle(el).color), 'rgb(220, 38, 38)')
})

test('Audit Log: an entry\'s window hides the page\'s marks; closing it brings them back', async t => {
  const { page } = await withHelp(t, '/audit-log', {}, auditReady)
  const marks = AUDIT.length + HEADER.length
  await waitForMarks(page, marks)
  await page.locator('.audit-row .detail-cell').first().click()
  await page.locator('.modal-card h3', { hasText: 'Audit Event' }).waitFor()
  await waitForMarks(page, 0)
  assert.equal(await page.locator('[aria-label^="Help: "]').count(), 0, 'no mark over the entry')
  await page.locator('.modal-card .modal-close').click()
  await waitForMarks(page, marks)
})

test('Audit Log is for operators and admins: a viewer is sent to the Dashboard and has no Audit Log marks', async t => {
  const { page } = await openPage(t, app, '/audit-log', { role: 'viewer' })
  await page.locator('.target-card').first().waitFor()
  assert.equal(new URL(page.url()).pathname, '/')
  await turnHelpOn(page)
  await mark(page, 'header.user').waitFor()
  assert.deepEqual((await markIds(page)).filter(id => id.startsWith('auditLog.')), [])
})

// ── Users ────────────────────────────────────────────────────────────────────

const USERS = [['users.create', 'Create User'], ['users.role', 'Role'], ['users.status', 'Status'], ['users.edit', 'Edit'],
  ['users.suspend', 'Suspend and Activate'], ['users.priv', 'Role privileges']]
const API_ACCESS = [['apiAccess.name', 'Token name'], ['apiAccess.prefix', 'Prefix'], ['apiAccess.used', 'Last used'],
  ['apiAccess.revoke', 'Revoke']]
const usersReady = page => page.locator('tbody tr', { hasText: 'operator@example.com' }).waitFor()
const tokensReady = page => page.locator('tbody tr', { hasText: 'grafana' }).waitFor()

test('Users: the bar says what Humans is for; a mark for Create User, the role and status badges, Edit, Suspend and the privileges', async t => {
  const { page } = await withHelp(t, '/users', { role: 'admin' }, usersReady)
  assert.match(await bar(page), /^Users is where admins create accounts/)
  await checkMarks(page, USERS)
  await mark(page, 'users.role').click()
  await waitForTitle(page, 'Role')
  // The role badges are the table's own
  const drawn = await note(page).locator('.badge', { hasText: /^operator$/ }).evaluate(el => getComputedStyle(el).backgroundColor)
  assert.equal(drawn, await page.locator('tbody .badge', { hasText: /^operator$/ }).evaluate(el => getComputedStyle(el).backgroundColor))
})

test('Users: the API Access sub-tab swaps the summary and the marks, each opens its note; Humans swaps them back', async t => {
  const { page } = await withHelp(t, '/users', { role: 'admin' }, usersReady)
  await waitForIds(page, [...idsOf(USERS), ...HEADER])
  await page.getByRole('button', { name: 'API Access', exact: true }).click()
  await page.locator('.help-bar[data-for-page="apiAccess"]').waitFor()
  assert.match(await bar(page), /^API Access issues tokens that let other systems, such as Grafana or Zabbix, read Bekci's machine API/)
  await tokensReady(page)
  await checkMarks(page, API_ACCESS)
  await page.getByRole('button', { name: 'Humans', exact: true }).click()
  await page.locator('.help-bar[data-for-page="users"]').waitFor()
  assert.match(await bar(page), /^Users is where admins create accounts/)
  await waitForIds(page, [...idsOf(USERS), ...HEADER])
})

test('API Access: a new token\'s card gets the New token mark', async t => {
  const plaintext = `bk_${'9f8e7d6c'.repeat(8)}`
  const { page, writes } = await withHelp(t, '/users', {
    role: 'admin',
    fixtures: { 'POST /api/api-tokens': { token: { id: 'k2', name: 'zabbix', prefix: plaintext.slice(0, 11) }, plaintext } },
  }, async page => {
    await usersReady(page)
    await page.getByRole('button', { name: 'API Access', exact: true }).click()
    await tokensReady(page)
  })
  await waitForIds(page, [...idsOf(API_ACCESS), ...HEADER])
  await page.locator('.page-header .btn', { hasText: 'Create Token' }).click()
  await page.getByPlaceholder('e.g. grafana-prod, zabbix-dr').fill('zabbix')
  await page.locator('form .btn', { hasText: /^Create$/ }).click()
  await page.locator('.token-reveal').waitFor()
  await waitForIds(page, ['apiAccess.once', ...idsOf(API_ACCESS), ...HEADER])
  await mark(page, 'apiAccess.once').click()
  await waitForTitle(page, 'New token')
  assert.match(await note(page).innerText(), /shown this once/)
  assert.deepEqual(writes, [{ method: 'POST', path: '/api/api-tokens', body: { name: 'zabbix' } }])
})

test('Users: the Edit and Suspend windows hide the page\'s marks; they come back when it closes', async t => {
  const { page, writes } = await withHelp(t, '/users', { role: 'admin' }, usersReady)
  const marks = USERS.length + HEADER.length
  await dialogHidesMarks(page, 'tbody .actions', 'Edit', 'Edit admin', marks)
  await dialogHidesMarks(page, 'tbody .actions', 'Suspend', 'Suspend admin?', marks)
  assert.deepEqual(writes, [], 'Cancel changes nothing')
})

test('Users and Settings are for admins: an operator is sent to the Dashboard and has none of their marks', async t => {
  for (const path of ['/users', '/settings']) {
    const { page } = await openPage(t, app, path, { role: 'operator' })
    await page.locator('.target-card').first().waitFor()
    assert.equal(new URL(page.url()).pathname, '/', path)
    await turnHelpOn(page)
    await mark(page, 'header.user').waitFor()
    assert.deepEqual((await markIds(page)).filter(id => /^(users|apiAccess|settings)/.test(id)), [], path)
  }
})

// ── Settings ─────────────────────────────────────────────────────────────────

const GENERAL = [['settingsGeneral.session', 'Session Timeout'], ['settingsGeneral.history', 'History Retention'],
  ['settingsGeneral.auditret', 'Audit Log Retention'], ['settingsGeneral.copies', 'Backup Copies to Keep'],
  ['settingsGeneral.soc', 'SOC View Public Access'], ['settingsGeneral.rate', 'API Rate Limit'],
  ['settingsGeneral.snmp', 'SNMP Credentials']]
const TAGS = [['settingsTags.free', 'Free Tags'], ['settingsTags.proj', 'Project and Location Tags'],
  ['settingsTags.cat', 'Categories']]
const SLA_GOALS = [['settingsSla.goal', 'Category goal'], ['settingsSla.save', 'Save'], ['settingsSla.guide', 'Badge guide']]
const ALERTING = [['settingsAlerting.method', 'Alert Method'], ['settingsAlerting.cool', 'Cooldown'],
  ['settingsAlerting.realert', 'Re-alert Interval'], ['settingsAlerting.sections', 'Save (each section)'],
  ['settingsAlerting.system', 'System Alerts']]
const BACKUP = [['settingsBackup.auto', 'Automated Backups'], ['settingsBackup.config', 'Download Config Backup'],
  ['settingsBackup.full', 'Full Database Backup'], ['settingsBackup.restore', 'Config Restore']]
const FAIL2BAN = [['settingsFail2ban.refresh', 'Updated and Refresh'], ['settingsFail2ban.active', 'Active Bans'],
  ['settingsFail2ban.total', 'Bans (total)'], ['settingsFail2ban.failed', 'Failed (window) and (total)']]
const HEALTH = [['settingsHealth.refresh', 'Refresh'], ['settingsHealth.disk', 'Disk Space'],
  ['settingsHealth.load', 'CPU Load'], ['settingsHealth.updates', 'OS Updates'], ['settingsHealth.backup', 'Last Backup'],
  ['settingsHealth.sched', 'Scheduler'], ['settingsHealth.email', 'Email']]

// tab: [page id, the bar's summary, its marks, what shows once its data is in]
const SETTINGS_TABS = {
  'General': ['settingsGeneral', /^General settings are the system-wide limits: how long sign-ins last/, GENERAL,
    page => page.locator('input[type=number]').first().waitFor()],
  'Tags': ['settingsTags', /^Tags are the pick-lists used to label targets/, TAGS,
    page => page.locator('.tag-item').first().waitFor()],
  'SLA': ['settingsSla', /^SLA goals set the uptime each category should reach over 90 days/, SLA_GOALS,
    page => page.locator('.sla-input').first().waitFor()],
  'Alerting': ['settingsAlerting', /^Alerting sets how Bekci tells people a target went down or recovered/, ALERTING,
    page => page.locator('.collapsible-body.open select').first().waitFor()],
  'Backup & Restore': ['settingsBackup', /^Backup & Restore schedules automatic backups/, BACKUP,
    page => page.getByRole('button', { name: 'Download Config Backup', exact: true }).waitFor()],
  'Fail2Ban': ['settingsFail2ban', /^Fail2Ban is a read-only view of the server's fail2ban jails/, FAIL2BAN,
    page => page.locator('.f2b-summary-table').waitFor()],
  'System Health': ['settingsHealth', /^System Health shows the health of the server running Bekci/, HEALTH,
    page => page.locator('.sh-grid').first().waitFor()],
}
const tabButton = (page, tab) => page.locator('.tabs').getByRole('button', { name: tab, exact: true })
const settingsTab = tab => async page => {
  await tabButton(page, 'General').waitFor()
  if (tab !== 'General') await tabButton(page, tab).click()
  await SETTINGS_TABS[tab][3](page)
}

for (const [tab, [, summary, marks]] of Object.entries(SETTINGS_TABS)) {
  test(`Settings › ${tab}: the bar says what the tab is for; a mark for each documented control, each opens its note`, async t => {
    const { page } = await withHelp(t, '/settings', { role: 'admin' }, settingsTab(tab))
    assert.match(await bar(page), summary)
    await checkMarks(page, marks)
  })
}

test('Settings: switching tab while help is on swaps the summary and the marks; Fail2Ban and System Health have a Refresh each', async t => {
  const { page } = await withHelp(t, '/settings', { role: 'admin' }, settingsTab('General'))
  for (const tab of ['General', 'Tags', 'Fail2Ban', 'System Health', 'SLA', 'General']) {
    const [id, summary, marks, ready] = SETTINGS_TABS[tab]
    await tabButton(page, tab).click()
    await ready(page)
    await page.locator(`.help-bar[data-for-page="${id}"]`).waitFor()
    assert.match(await bar(page), summary, tab)
    await waitForIds(page, [...idsOf(marks), ...HEADER])
  }
})

const TEST_NOTE = 'Each test sends a real message. Test Email goes to the address in the box, or to your own Profile email '
  + 'if the box is empty. Test Signal goes to the number in the box. Test Webhook sends a test event named \'Bekci Test\' '
  + 'to the saved URL; a receiving system such as a SOAR may open a ticket for it, so tell its owners first. Every test '
  + 'is recorded in the Audit Log.'
const TEST_HINT = 'Uses the saved settings, not what is typed: save first.'

test('Settings › Alerting: a closed section\'s fields have no marks until it opens; the test note is the agreed text, not the hint', async t => {
  const { page } = await withHelp(t, '/settings', { role: 'admin' }, settingsTab('Alerting'))
  const first = idsOf(ALERTING).slice(0, 4) // method, cooldown, re-alert, the first section's Save
  const system = 'settingsAlerting.system'
  await waitForIds(page, [...idsOf(ALERTING), ...HEADER])
  const section = name => page.locator('.collapsible-header', { hasText: name })

  // Email closed: no From mark; opened: From and the test button
  await section('Email Alerting').click()
  await waitForIds(page, [...first, 'settingsAlerting.from', 'settingsAlerting.test', system, ...HEADER])
  await mark(page, 'settingsAlerting.test').click()
  await waitForTitle(page, 'Send Test')
  assert.equal((await note(page).locator('.help-note-text').allInnerTexts()).join(' '), TEST_NOTE)
  assert.equal(await page.locator('.collapsible-body.open .test-hint').first().innerText(), TEST_HINT, 'the hint is on the page')
  assert.ok(!(await note(page).innerText()).includes(TEST_HINT), 'the note does not repeat the hint')
  await page.keyboard.press('Escape')
  await note(page).waitFor({ state: 'detached' })
  await section('Email Alerting').click()
  await waitForIds(page, [...idsOf(ALERTING), ...HEADER])

  // Signal, then Webhook, on their own: their fields, and their own test button
  for (const [name, id, title] of [['Signal Alerting', 'settingsAlerting.signal', 'Signal fields'],
    ['Webhook Alerting', 'settingsAlerting.hook', 'Webhook']]) {
    await section(name).click()
    await waitForIds(page, [...first, id, 'settingsAlerting.test', system, ...HEADER])
    await mark(page, id).click()
    await waitForTitle(page, title)
    await page.keyboard.press('Escape')
    await note(page).waitFor({ state: 'detached' })
    await section(name).click()
    await waitForIds(page, [...idsOf(ALERTING), ...HEADER])
  }
})

test('Settings › Backup & Restore: opening Full Database Backup adds its marks; the restore dialog hides the marks', async t => {
  const { page, writes } = await withHelp(t, '/settings', { role: 'admin' }, settingsTab('Backup & Restore'))
  const marks = BACKUP.length + HEADER.length
  await waitForIds(page, [...idsOf(BACKUP), ...HEADER])
  const full = page.locator('.collapsible-header', { hasText: 'Full Database Backup' })
  await full.click()
  await waitForIds(page, ['settingsBackup.auto', 'settingsBackup.config', 'settingsBackup.full', 'settingsBackup.enc',
    'settingsBackup.saved', 'settingsBackup.restore', ...HEADER])
  for (const [id, title] of [['settingsBackup.enc', 'Encrypt backup'], ['settingsBackup.saved', 'Saved Backups']]) {
    await mark(page, id).click()
    await waitForTitle(page, title)
    await page.keyboard.press('Escape')
    await note(page).waitFor({ state: 'detached' })
  }
  await full.click()
  await waitForIds(page, [...idsOf(BACKUP), ...HEADER])
  // A config file chosen, Restore asks first in a window of the page's own
  await page.locator('input[type=file]').setInputFiles({ name: 'bekci-config.json', mimeType: 'application/json', buffer: Buffer.from('{}') })
  await dialogHidesMarks(page, '.restore-section', 'Restore from backup', 'Restore Backup', marks)
  assert.deepEqual(writes.filter(w => w.path === '/api/backup/restore'), [], 'Cancel restores nothing')
})
