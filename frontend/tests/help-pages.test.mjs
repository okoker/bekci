// The help notes of the pages after the Dashboard (design in_app_help.md §7.3 and
// §7.5-7.8): per page, which controls get a ? mark, that every mark opens a note
// with its title, that role-limited controls get no mark for people who can't see
// them, that a page's own dialog hides the page's marks, and that help's own
// chrome stays turquoise. The Dashboard and the header are in help-marks.test.mjs.
// Task 6 adds Settings, Users and Audit Log here, a section each.
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
  assert.equal(await note(page).locator('.help-note-link').count(), 0, 'the lesson link stays hidden until the lesson exists')
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
