// In-app help on the page itself (design in_app_help.md §4.2-4.4): ? marks beside
// the documented controls, the note a mark opens (with its leader line and the
// control outlined), the bar's stepper, and the keyboard. The Dashboard and the
// header are the first pages with notes.
import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import {
  apiFixtures, moreTargets, openPage, orangeIn, staleScheduler, startApp, stopApp, turnHelpOn, waitForStyle,
} from './help-fixtures.mjs'

let app
before(async () => { app = await startApp() })
after(() => stopApp(app))

const mark = (page, id) => page.locator(`.help-mark[data-help-mark="${id}"]`)
const markIds = page => page.locator('.help-mark').evaluateAll(els => els.map(e => e.dataset.helpMark))
const note = page => page.locator('.help-note')
const noteTitle = page => note(page).locator('.help-note-title')
const stepper = page => page.locator('.help-bar .help-stepper-pos')
const focused = page => page.evaluate(() => document.activeElement?.dataset.helpMark ?? document.activeElement?.tagName)
// The note is measured hidden, then placed and shown: wait for it on screen
const waitForTitle = (page, title) => page.waitForFunction(title => {
  const note = document.querySelector('.help-note')
  return note?.querySelector('.help-note-title').textContent.trim() === title
    && getComputedStyle(note).visibility === 'visible'
}, title, { timeout: 2000 })
const waitForFocusInNote = page => page.waitForFunction(() => !!document.activeElement?.closest('.help-note'),
  null, { timeout: 2000 })
const overlap = (a, b) => a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height
const levelWith = (m, row) => m.y + m.height / 2 > row.y && m.y + m.height / 2 < row.y + row.height

// The fixture Dashboard, rows folded: Web shop comes first (it is down), then
// Core switch. Every documented control on screen, in page order, then the header.
const MARKS = ['dashboard.updated', 'dashboard.cats', 'dashboard.row', 'dashboard.type', 'dashboard.pct',
  'dashboard.sla', 'dashboard.updown', 'dashboard.bars90', 'dashboard.bars4h',
  'header.version', 'header.dots', 'header.user']

// The Dashboard as an operator, rows on screen, help on and its marks drawn.
async function dashboardWithHelp(t, options = {}) {
  const { page } = await openPage(t, app, '/', { role: 'operator', ...options })
  await page.locator('.target-card').first().waitFor()
  await turnHelpOn(page)
  await mark(page, 'header.user').waitFor()
  return page
}

const allMarks = (page, n) => page.waitForFunction(n => document.querySelectorAll('.help-mark').length === n, n,
  { timeout: 2000 })

test('one ? mark per documented control on screen, in page order and then the header; the bar counts them', async t => {
  const page = await dashboardWithHelp(t)
  await allMarks(page, MARKS.length)
  assert.deepEqual(await markIds(page), MARKS)
  const updated = mark(page, 'dashboard.updated')
  assert.equal(await updated.getAttribute('aria-label'), 'Help: Updated')
  assert.equal(await updated.getAttribute('title'), 'Updated', 'hover shows the name')
  // A control on every row has its mark on the first row only
  const firstRow = await page.locator('.target-header').first().boundingBox()
  assert.ok(levelWith(await mark(page, 'dashboard.row').boundingBox(), firstRow), 'row mark beside the first row')
  // The bar: the hint, and the stepper counting each key once
  assert.equal((await page.locator('.help-bar-hint').innerText()).trim(), 'Click any ? to read it')
  assert.equal((await stepper(page).innerText()).trim(), `${MARKS.length} notes`)
  await page.locator('.help-bar').getByRole('button', { name: 'Next note', exact: true }).click()
  await waitForTitle(page, 'Updated')
  assert.equal((await stepper(page).innerText()).trim(), `1 of ${MARKS.length}`)
  await page.locator('.help-bar').getByRole('button', { name: 'Previous note', exact: true }).click()
  await waitForTitle(page, 'Your account')
  assert.equal((await stepper(page).innerText()).trim(), `${MARKS.length} of ${MARKS.length}`)
})

test('unfolding a target adds marks for its PRIMARY badge and its check rows', async t => {
  const page = await dashboardWithHelp(t)
  await allMarks(page, MARKS.length)
  await page.locator('.target-header').first().click()
  await mark(page, 'dashboard.prim').waitFor()
  await mark(page, 'dashboard.check').waitFor()
  await allMarks(page, MARKS.length + 2)
  assert.equal((await stepper(page).innerText()).trim(), `${MARKS.length + 2} notes`)
  const firstCheck = await page.locator('.check-row').first().boundingBox()
  assert.ok(levelWith(await mark(page, 'dashboard.check').boundingBox(), firstCheck), 'check mark beside the first check')
})

test('a mark opens its note with a leader line to the control, which is outlined; one note at a time', async t => {
  const page = await dashboardWithHelp(t)
  await allMarks(page, MARKS.length)
  await mark(page, 'dashboard.sla').click()
  await waitForTitle(page, 'HEALTHY / UNHEALTHY')
  await waitForFocusInNote(page)
  assert.match(await note(page).innerText(), /a target can be UP and UNHEALTHY/)
  assert.match(await note(page).locator('.help-note-foot').innerText(), /6 of 12/)
  assert.equal((await stepper(page).innerText()).trim(), '6 of 12')
  assert.equal(await mark(page, 'dashboard.sla').getAttribute('aria-expanded'), 'true')

  // The SLA badge on the first row is outlined in turquoise
  const badge = page.locator('.target-card').first().locator('[data-help="dashboard.sla"]')
  await waitForStyle(badge, 'outlineStyle', 'solid')
  assert.equal(await badge.evaluate(el => getComputedStyle(el).outlineWidth), '2px')
  assert.equal(await badge.evaluate(el => getComputedStyle(el).outlineColor), 'rgb(13, 148, 136)')
  assert.equal(await page.locator('[data-help-on]').count(), 1, 'only that control is outlined')

  // A leader line from the note's edge to the badge, ending in a dot on it; the note doesn't cover the badge
  const box = await badge.boundingBox()
  const noteBox = await note(page).boundingBox()
  assert.ok(!overlap(noteBox, box), 'the note leaves the badge visible')
  const line = await page.locator('.help-lines line').evaluate(l =>
    Object.fromEntries(['x1', 'y1', 'x2', 'y2'].map(a => [a, Number(l.getAttribute(a))])))
  const dot = await page.locator('.help-lines circle').evaluate(c => ({ x: Number(c.getAttribute('cx')), y: Number(c.getAttribute('cy')) }))
  assert.ok(Math.hypot(line.x2 - line.x1, line.y2 - line.y1) > 10, 'a visible line')
  const near = (x, y, b, d) => x >= b.x - d && x <= b.x + b.width + d && y >= b.y - d && y <= b.y + b.height + d
  // The line leaves the note from the edge that faces the badge (the note sits below it here)
  assert.ok(noteBox.y >= box.y + box.height, 'the note is below the badge')
  assert.ok(Math.abs(line.y1 - noteBox.y) <= 1, `line starts on the note's top edge (y ${line.y1}, note top ${noteBox.y})`)
  assert.ok(line.x1 >= noteBox.x && line.x1 <= noteBox.x + noteBox.width, `line starts within the note's width (x ${line.x1})`)
  assert.deepEqual([dot.x, dot.y], [line.x2, line.y2], 'the dot ends the line')
  assert.ok(near(dot.x, dot.y, box, 6), `dot on the badge (${dot.x}, ${dot.y})`)
  // Turquoise, the help colour
  assert.equal(await page.locator('.help-lines line').evaluate(l => getComputedStyle(l).stroke), 'rgb(13, 148, 136)')
  assert.equal(await page.locator('.help-lines circle').evaluate(c => getComputedStyle(c).fill), 'rgb(13, 148, 136)')

  // Help is turquoise: nothing in the marks, the note, its line or the bar is Bekci orange
  assert.deepEqual(await orangeIn(page, '.help-mark, .help-note, .help-lines, .help-bar'), [])

  // Another mark moves the note; the same mark again closes it; so does a click on the page
  await mark(page, 'dashboard.updown').click()
  await waitForTitle(page, 'UP / DOWN')
  assert.equal(await note(page).count(), 1, 'one note at a time')
  assert.equal(await mark(page, 'dashboard.sla').getAttribute('aria-expanded'), 'false')
  await mark(page, 'dashboard.updown').click()
  await note(page).waitFor({ state: 'detached' })
  assert.equal(await page.locator('[data-help-on]').count(), 0, 'outline gone with the note')
  await mark(page, 'dashboard.updown').click()
  await note(page).waitFor()
  await page.locator('.page-header h2').click()
  await note(page).waitFor({ state: 'detached' })
  assert.equal(await page.locator('.help-bar').count(), 1, 'help stays on')
})

test('keyboard: Tab goes mark to mark, Enter opens a note, ← and → step, Esc closes the note and then help', async t => {
  const page = await dashboardWithHelp(t)
  await allMarks(page, MARKS.length)
  await mark(page, 'dashboard.sla').focus()
  await page.keyboard.press('Tab')
  assert.equal(await focused(page), 'dashboard.updown', 'Tab reaches the next mark')
  await page.keyboard.press('Shift+Tab')
  assert.equal(await focused(page), 'dashboard.sla')
  await page.keyboard.press('Enter')
  await waitForTitle(page, 'HEALTHY / UNHEALTHY')
  await waitForFocusInNote(page)
  await page.keyboard.press('ArrowRight')
  await waitForTitle(page, 'UP / DOWN')
  assert.equal((await stepper(page).innerText()).trim(), '7 of 12')
  await page.keyboard.press('ArrowLeft')
  await page.keyboard.press('ArrowLeft')
  await waitForTitle(page, '90-day uptime')
  assert.equal((await stepper(page).innerText()).trim(), '5 of 12')
  await page.keyboard.press('Escape')
  await note(page).waitFor({ state: 'detached' })
  assert.equal(await focused(page), 'dashboard.pct', 'focus back on the mark of the note that closed')
  assert.equal(await page.locator('.help-bar').count(), 1, 'the first Esc closes only the note')
  await page.keyboard.press('Escape')
  await page.locator('.help-bar').waitFor({ state: 'detached' })
  assert.equal(await page.locator('.help-mark').count(), 0, 'no marks once help is off')
  assert.equal(await page.locator('[data-help-on]').count(), 0, 'no outline once help is off')
})

test('a text box keeps its own Esc and arrows; a checkbox does not keep Esc from help', async t => {
  const page = await dashboardWithHelp(t)
  // The Dashboard has no field: add a text box and a checkbox outside the app
  await page.evaluate(() => {
    for (const [id, type] of [['probe-text', 'text'], ['probe-box', 'checkbox']]) {
      document.body.append(Object.assign(document.createElement('input'), { id, type }))
    }
  })
  await mark(page, 'dashboard.sla').click()
  await waitForTitle(page, 'HEALTHY / UNHEALTHY')
  await page.locator('#probe-text').focus()
  await page.keyboard.press('ArrowRight')
  await page.keyboard.press('Escape')
  assert.equal((await noteTitle(page).innerText()).trim(), 'HEALTHY / UNHEALTHY', 'the note stays, on the same item')
  await page.locator('#probe-box').focus()
  await page.keyboard.press('Escape')
  await note(page).waitFor({ state: 'detached' })
  assert.equal(await page.locator('.help-bar').count(), 1, 'Esc on a checkbox closed the note, help still on')
  assert.equal(await page.evaluate(() => document.activeElement.id), 'probe-box', 'focus stays where the user put it')
  await page.keyboard.press('Escape')
  await page.locator('.help-bar').waitFor({ state: 'detached' })
})

test('marks follow their control when the page scrolls; the pagination has a mark', async t => {
  const { page } = await openPage(t, app, '/', {
    role: 'operator', viewport: { width: 1280, height: 800 }, fixtures: moreTargets(10),
  })
  await page.locator('.target-card').nth(9).waitFor()
  await turnHelpOn(page)
  const rowMark = mark(page, 'dashboard.row')
  await rowMark.waitFor()
  await mark(page, 'dashboard.pages').waitFor()
  const before = await rowMark.boundingBox()
  await page.evaluate(() => window.scrollBy(0, 200))
  assert.equal(await page.evaluate(() => window.scrollY), 200, 'the page scrolled')
  await page.waitForFunction(([id, y]) =>
    document.querySelector(`.help-mark[data-help-mark="${id}"]`).getBoundingClientRect().y !== y,
  ['dashboard.row', before.y], { timeout: 2000 })
  const moved = await rowMark.boundingBox()
  assert.ok(Math.abs(moved.y - (before.y - 200)) <= 1, `mark top ${before.y} → ${moved.y}`)
  assert.ok(levelWith(moved, await page.locator('.target-header').first().boundingBox()), 'still beside its row')
})

test('a note stays inside the window and off its control: Your account at 1280×800', async t => {
  const { page } = await openPage(t, app, '/', { role: 'operator', viewport: { width: 1280, height: 800 } })
  await page.locator('.target-card').first().waitFor()
  await turnHelpOn(page)
  await mark(page, 'header.user').click()
  await waitForTitle(page, 'Your account')
  assert.match(await note(page).innerText(), /Profile .*Logout/s)
  const box = await note(page).boundingBox()
  assert.ok(box.x >= 0 && box.y >= 0 && box.x + box.width <= 1280 && box.y + box.height <= 800,
    `note inside the window: ${JSON.stringify(box)}`)
  assert.ok(!overlap(box, await page.locator('.user-menu-trigger').boundingBox()), 'the control stays visible')
})

// The header's notes are the last stops of the stepper on every page: a note
// opened there must leave the bar's ‹, › and Done where they can be clicked
test('a header note leaves the bar\'s ‹, › and Done uncovered, and stays inside the window', async t => {
  for (const viewport of [{ width: 1280, height: 800 }, { width: 1440, height: 900 }]) {
    const { page } = await openPage(t, app, '/', { role: 'operator', viewport })
    await page.locator('.target-card').first().waitFor()
    await turnHelpOn(page)
    await allMarks(page, MARKS.length)
    const bar = page.locator('.help-bar')
    const buttons = {
      '‹': bar.getByRole('button', { name: 'Previous note', exact: true }),
      '›': bar.getByRole('button', { name: 'Next note', exact: true }),
      Done: bar.getByRole('button', { name: 'Done', exact: true }),
    }
    for (const [id, title] of [['header.version', 'Version'], ['header.dots', 'Server health'],
      ['header.user', 'Your account']]) {
      await mark(page, id).click()
      await waitForTitle(page, title)
      const box = await note(page).boundingBox()
      const where = `${viewport.width}×${viewport.height}, ${title}`
      assert.ok(box.x >= 0 && box.y >= 0 && box.x + box.width <= viewport.width && box.y + box.height <= viewport.height,
        `${where}: note inside the window ${JSON.stringify(box)}`)
      for (const [name, button] of Object.entries(buttons)) {
        const hit = await button.evaluate(el => {
          const r = el.getBoundingClientRect()
          return document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2) === el
        })
        assert.ok(hit, `${where}: the bar's ${name} is uncovered`)
      }
    }
    // And they work with the note open: › from the last header note goes round to the first note
    await buttons['›'].click()
    await waitForTitle(page, 'Updated')
  }
})

// Review Focus 3: help is on while the page's data is still loading. The rows
// are held back until help is on and its marks are drawn, then let through.
test('help turned on while the Dashboard is loading marks the rows once they arrive', async t => {
  let release
  const held = new Promise(resolve => { release = resolve })
  t.after(() => release())
  const rows = apiFixtures({ role: 'operator' })['/api/dashboard/status']
  const { page } = await openPage(t, app, '/', {
    role: 'operator', fixtures: { '/api/dashboard/status': async () => { await held; return rows } },
  })
  await page.getByText('Loading dashboard...').waitFor()
  await turnHelpOn(page)
  await mark(page, 'header.user').waitFor()
  assert.equal(await page.locator('.target-card').count(), 0, 'the rows are still held back')
  assert.equal(await mark(page, 'dashboard.row').count(), 0)
  release()
  await mark(page, 'dashboard.row').waitFor()
  assert.ok(levelWith(await mark(page, 'dashboard.row').boundingBox(), await page.locator('.target-header').first().boundingBox()),
    'beside the first row')
  await allMarks(page, MARKS.length)
  assert.equal((await stepper(page).innerText()).trim(), `${MARKS.length} notes`)
})

test('the "Monitoring engine not responding" banner has a mark only while it shows', async t => {
  const normal = await dashboardWithHelp(t)
  await allMarks(normal, MARKS.length)
  assert.equal(await normal.locator('.stale-banner').count(), 0)
  assert.equal(await mark(normal, 'header.banner').count(), 0)

  const stale = await dashboardWithHelp(t, { fixtures: staleScheduler() })
  await stale.locator('.stale-banner').waitFor()
  await mark(stale, 'header.banner').waitFor()
  assert.deepEqual(await markIds(stale), [...MARKS, 'header.banner'])
  await mark(stale, 'header.banner').click()
  await waitForTitle(stale, 'Monitoring engine not responding')
  assert.match(await note(stale).innerText(), /Tell an admin\./)
})
