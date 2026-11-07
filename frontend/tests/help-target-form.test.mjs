// In-app help inside the target form (design in_app_help.md §4.3-4.6, §7.2): the
// form's own "? Help" and help bar, a ? mark on every instance of a per-condition
// field (the stepper counts each key once), notes in the right margin with a
// leader line that bends under the field, and the plain-words card that reads the
// rule back and flags settings that cause false alarms.
import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { chromium } from 'playwright-core'
import { apiFixtures, openPage, orangeIn, startApp, stopApp, turnHelpOn } from './help-fixtures.mjs'

let app
before(async () => { app = await startApp() })
after(() => stopApp(app))

const mark = (page, id) => page.locator(`.help-mark[data-help-mark="targetForm.${id}"]`)
const note = page => page.locator('.help-note')
const card = page => page.locator('.rule-card')
const waitForTitle = (page, title) => page.waitForFunction(title => {
  const note = document.querySelector('.help-note')
  return note?.querySelector('.help-note-title').textContent.trim() === title
    && getComputedStyle(note).visibility === 'visible'
}, title, { timeout: 2000 })
// Marks are drawn in the window, beside their field: a field below the form's
// fold is scrolled to first, as a person would
async function clickMark(page, key, n = 0) {
  await page.locator(`[data-help="targetForm.${key}"]`).nth(n).evaluate(el => el.scrollIntoView({ block: 'center' }))
  await mark(page, key).nth(n).click()
}
// The value a stylesheet rule sets (Chrome snaps a 1.5px border to whole pixels at scale 1)
const authored = (page, selector, prop) => page.evaluate(([selector, prop]) => {
  for (const sheet of document.styleSheets) {
    for (const rule of sheet.cssRules) {
      if (rule.selectorText?.split(/\s*,\s*/).some(s => s.replace(/\[data-v-[^\]]+\]/g, '') === selector)) {
        const value = rule.style.getPropertyValue(prop)
        if (value) return value
      }
    }
  }
  return null
}, [selector, prop])
const overlap = (a, b) => a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height
const inside = (box, view) => box.x >= 0 && box.y >= 0 && box.x + box.width <= view.width && box.y + box.height <= view.height

// Waits until the locator's text matches, then checks it (so a failure shows the text it ended on)
async function textMatches(locator, pattern, message) {
  const handle = await locator.elementHandle()
  await locator.page().waitForFunction(([el, source]) => new RegExp(source).test(el.innerText),
    [handle, pattern.source], { timeout: 2000 }).catch(() => {})
  assert.match(await locator.innerText(), pattern, message)
}

// The Web shop target as the form loads it: a ping that alerts on the latest
// check, and a web page checked every 5 minutes that alerts on 2 in a row; one
// group (AND); admin ticked as recipient.
const ping = { check_id: 'c3', check_type: 'ping', check_name: 'Ping', config: JSON.stringify({ count: 3, timeout_s: 5 }),
  interval_s: 60, field: 'status', comparator: 'eq', value: 'down', fail_count: 1, fail_window: 0,
  condition_group: 0, group_operator: 'AND' }
const web = { check_id: 'c2', check_type: 'http', check_name: 'Web page',
  config: JSON.stringify({ scheme: 'https', port: 0, endpoint: '/', expect_status: 200, skip_tls_verify: false, timeout_s: 10 }),
  interval_s: 300, field: 'status', comparator: 'eq', value: 'down', fail_count: 2, fail_window: 600,
  condition_group: 0, group_operator: 'AND' }
const FORM_TARGET = { ...apiFixtures()['/api/targets/t2'], conditions: [ping, web] }
// The same two checks, each in a group of its own: nothing confirms the ping,
// so its Fail Count 1 can page on a single blip
const SPLIT_TARGET = { ...FORM_TARGET, conditions: [ping, { ...web, condition_group: 1 }] }

// The notes with a mark on this form, in page order, each key once
const KEYS = ['name', 'host', 'category', 'preferred', 'notes', 'filters', 'enabled', 'groups', 'addgroup', 'group',
  'andor', 'remove', 'type', 'int', 'pingcount', 'timeout', 'when', 'fc', 'tw', 'expect', 'skiptls', 'addcond',
  'rcp', 'save']

// Targets as an operator; help on (unless `help` is false); then the form,
// opened with `button` (Edit, Clone or + Target), with its conditions on screen.
async function openForm(t, { viewport = { width: 1440, height: 900 }, help = true, button = 'Edit', target = FORM_TARGET,
  browser = app.browser } = {}) {
  const { page, writes } = await openPage(t, { ...app, browser }, '/targets', {
    role: 'operator', viewport, fixtures: { '/api/targets/t2': target },
  })
  const row = page.locator('tbody tr', { hasText: 'Web shop' })
  await row.waitFor()
  if (help) await turnHelpOn(page)
  if (button === '+ Target') await page.getByRole('button', { name: '+ Target', exact: true }).click()
  else await row.getByRole('button', { name: button, exact: true }).click()
  const overlay = page.locator('.modal-overlay').filter({ has: page.locator('.modal-wide') })
  const modal = overlay.locator('.modal-card')
  await modal.waitFor()
  if (button !== '+ Target') await modal.locator('.condition-card').nth(target.conditions.length - 1).waitFor()
  return { page, writes, overlay, modal }
}

// The form field (its .form-group) labelled `label` in the nth condition
const field = (modal, label, n = 0) => modal.locator('.condition-card').nth(n).locator('.form-group')
  .filter({ has: modal.page().locator('label').filter({ hasText: new RegExp(`^${label}$`) }) })

test('Help on, on Targets: + Target, Edit and Clone open the form with help on; its own "? Help" turns help off and on', async t => {
  const { page, overlay, modal } = await openForm(t)
  const bar = overlay.locator('.help-bar')
  await bar.waitFor()
  assert.match(await bar.locator('.help-bar-text').innerText(), /^The target form sets what Bekci watches, how it tests it, and exactly when it counts as down\./)
  assert.equal(await bar.getAttribute('data-for-page'), 'targetForm')
  const barBox = await bar.boundingBox()
  const formBox = await modal.boundingBox()
  assert.ok(barBox.y <= 1 && barBox.width >= 1440 - 20, `the bar runs across the top of the overlay (${JSON.stringify(barBox)})`)
  assert.ok(barBox.y + barBox.height <= formBox.y, 'the form starts under the bar')
  // The page's own bar, under the header, would show through the overlay's backdrop as a second bar
  const pageBar = page.locator('.layout-v > .help-bar')
  assert.equal(await pageBar.evaluate(el => getComputedStyle(el).visibility), 'hidden', 'only the form\'s bar shows')
  await mark(page, 'name').waitFor()

  // The form's "? Help": turquoise outline, pressed while help is on, and it toggles help
  const formHelp = modal.getByRole('button', { name: 'Help', exact: true })
  assert.equal(await formHelp.getAttribute('aria-pressed'), 'true')
  assert.equal(await formHelp.evaluate(el => getComputedStyle(el).borderTopColor), 'rgb(13, 148, 136)')
  assert.deepEqual(await orangeIn(page, '.modal-card .btn-form-help'), [])
  await formHelp.click()
  await bar.waitFor({ state: 'detached' })
  assert.equal(await page.locator('.help-bar').count(), 0, 'help is off everywhere, the layout bar too')
  assert.equal(await page.locator('.help-mark').count(), 0, 'no marks')
  assert.equal(await card(page).count(), 0, 'the plain-words card shows only in help mode')
  assert.equal(await formHelp.getAttribute('aria-pressed'), 'false')
  await formHelp.click()
  await bar.waitFor()
  await mark(page, 'name').waitFor()
  await card(page).waitFor()

  // Cancel; the page's bar is back. + Target opens with help on, and its card has no conditions to read yet
  await modal.getByRole('button', { name: 'Cancel', exact: true }).click()
  await modal.waitFor({ state: 'detached' })
  assert.equal(await pageBar.evaluate(el => getComputedStyle(el).visibility), 'visible', 'the page\'s bar is back')
  await page.getByRole('button', { name: '+ Target', exact: true }).click()
  await bar.waitFor()
  await card(page).waitFor()
  await textMatches(card(page), /No conditions yet\./, 'an empty rule says so')
  assert.doesNotMatch(await card(page).innerText(), /counts as down when/, 'no dangling opening')

  // Clone opens with help on; it starts with nobody ticked, which the card notes without a warning sign
  await modal.getByRole('button', { name: 'Cancel', exact: true }).click()
  await modal.waitFor({ state: 'detached' })
  await page.locator('tbody tr', { hasText: 'Web shop' }).getByRole('button', { name: 'Clone', exact: true }).click()
  await bar.waitFor()
  await textMatches(card(page), /No one is ticked, so this target sends no email or Signal\./)
  const neutral = card(page).locator('.rule-card-check.is-note')
  assert.equal(await neutral.count(), 1)
  assert.equal((await neutral.locator('.rule-card-sign').innerText()).trim(), '', 'no ! mark on the neutral note')
})

test('a mark on every condition\'s Fail Count, the stepper counting it once; each note uses its own condition\'s numbers', async t => {
  const { page, overlay } = await openForm(t)
  await page.waitForFunction(() => document.querySelectorAll('.help-mark[data-help-mark="targetForm.fc"]').length === 2,
    null, { timeout: 2000 })
  const ids = await page.locator('.help-mark').evaluateAll(els => els.map(e => e.dataset.helpMark))
  assert.deepEqual([...new Set(ids)], KEYS.map(k => `targetForm.${k}`), 'the form\'s notes in page order, header marks gone')
  for (const [key, count] of [['fc', 2], ['tw', 2], ['type', 2], ['when', 2], ['remove', 3], ['group', 1]]) {
    assert.equal(await mark(page, key).count(), count, `${key}: a mark on each instance`)
  }
  assert.equal(await mark(page, 'fc').first().getAttribute('aria-label'), 'Help: Fail Count')
  const stepper = overlay.locator('.help-bar .help-stepper-pos')
  assert.equal((await stepper.innerText()).trim(), `${KEYS.length} notes`)

  // The second condition: checked every 300 seconds, Fail Count 2
  await clickMark(page, 'fc', 1)
  await waitForTitle(page, 'Fail Count')
  const text = await note(page).innerText()
  assert.match(text, /How many results in a row must fail before this condition counts/)
  assert.match(text, /a check every 300 seconds, Fail Count 2/)
  assert.match(text, /A single failure never alerts\. A real outage alerts on the second failed check, 5 to 10 minutes in\./)
  assert.match(text, /Rule of thumb: 3, for almost every check\./)
  // The timeline: 8 results, the running count, "down" where it triggers (failCountTimeline's labels)
  assert.deepEqual(await note(page).locator('.help-timeline-label').evaluateAll(els => els.map(e => e.textContent.trim())),
    ['', '1', 'down', '', '1', 'down', 'down', 'down'])
  assert.equal(await note(page).locator('.help-timeline-result').count(), 8)
  assert.equal((await stepper.innerText()).trim(), `${KEYS.indexOf('fc') + 1} of ${KEYS.length}`)
  const outlined = page.locator('[data-help-on]')
  assert.equal(await outlined.count(), 1)
  assert.equal(await outlined.evaluate(el => el === document.querySelectorAll('[data-help="targetForm.fc"]')[1]), true,
    'the second condition\'s Fail Count is outlined')
  assert.equal(await mark(page, 'fc').nth(1).getAttribute('aria-expanded'), 'true')

  // The first condition: Fail Count 1, once a minute
  await clickMark(page, 'fc', 0)
  await textMatches(note(page), /a check every 60 seconds, Fail Count 1/)
  assert.match(await note(page).innerText(), /Every failed check alerts at once, however brief the blip\./)
  assert.equal(await mark(page, 'fc').nth(0).getAttribute('aria-expanded'), 'true')

  // Time Window, with the form's numbers
  await clickMark(page, 'tw', 1)
  await waitForTitle(page, 'Time Window')
  assert.match(await note(page).innerText(), /at least Fail Count × Interval \(here 2 × 300 = 600\), at most 1800/)

  // AND / OR draws the form's own buttons
  await clickMark(page, 'andor')
  await waitForTitle(page, 'AND / OR')
  const drawn = note(page).locator('.help-note-control .op-btn')
  assert.deepEqual(await drawn.evaluateAll(els => els.map(e => e.textContent.trim())), ['AND', 'OR'])
  const formAnd = await overlay.locator('.op-btn.active').evaluate(el => getComputedStyle(el).backgroundColor)
  assert.equal(await drawn.first().evaluate(el => getComputedStyle(el).backgroundColor), formAnd)
})

test('the plain-words card follows the form: Fail Count 1→3 reads "3 checks in a row" and drops the single-failure warning', async t => {
  // The ping alone in its group: nothing confirms it, so Fail Count 1 is warned about
  const { page, modal } = await openForm(t, { target: SPLIT_TARGET })
  await card(page).waitFor()
  assert.equal((await card(page).locator('h3').innerText()).trim(), 'Your rule in plain words')
  assert.match(await card(page).innerText(), /Rewritten from the form as you change it/)
  await textMatches(card(page), /Web shop counts as down when:/)
  let text = await card(page).innerText()
  const group = card(page).locator('.rule-card-group')
  assert.equal(await group.count(), 2)
  assert.equal((await group.nth(0).innerText()).trim(), 'ping has failed on the latest check')
  assert.match(await group.nth(1).innerText(), /the web page has not answered with 200 2 checks in a row\s+5 to 10 minutes into an outage, at one check every 5 minutes/)
  assert.match(text, /It counts as up again on the first check where none of these is true\./)
  assert.match(text, /Checked for false alarms/)
  assert.match(text, /Ping alerts on a single failed check\. One blip will page people; 3 in a row is safer\./)
  const warn = card(page).locator('.rule-card-check.is-warn .rule-card-sign')
  assert.equal(await warn.count(), 1)
  assert.equal((await warn.innerText()).trim(), '!')
  assert.equal(await warn.evaluate(el => getComputedStyle(el).color), 'rgb(180, 83, 9)', 'amber !')
  assert.equal(await warn.evaluate(el => getComputedStyle(el).backgroundColor), 'rgb(254, 243, 199)')
  assert.equal(await authored(page, '.rule-card-group', 'border-top-width'), '1.5px')
  assert.equal(await group.first().evaluate(el => getComputedStyle(el).borderTopColor), 'rgb(147, 197, 253)')
  // The two groups have a slate OR pill between them
  const pill = card(page).locator('.rule-card-or span')
  assert.equal((await pill.innerText()).trim(), 'OR')
  assert.equal(await pill.evaluate(el => getComputedStyle(el).backgroundColor), 'rgb(71, 85, 105)')
  assert.equal(await pill.evaluate(el => getComputedStyle(el).color), 'rgb(255, 255, 255)')

  const failCount = field(modal, 'Fail Count', 0).locator('input')
  await failCount.fill('3')
  await failCount.press('Tab')
  await textMatches(group.nth(0), /^ping has failed 3 checks in a row\s+2 to 3 minutes into an outage, at one check a minute$/)
  text = await card(page).innerText()
  assert.doesNotMatch(text, /on the latest check/)
  assert.doesNotMatch(text, /single failed check/)
  assert.match(text, /No test that can flicker alerts on a single failure\./)
  assert.equal(await card(page).locator('.rule-card-check.is-warn').count(), 0)
  const ok = card(page).locator('.rule-card-check.is-ok .rule-card-sign').first()
  assert.equal((await ok.innerText()).trim(), '✓')
  assert.equal(await ok.evaluate(el => getComputedStyle(el).color), 'rgb(21, 128, 61)', 'green ✓')

  // A third group: another box and another OR pill
  await modal.getByRole('button', { name: '+ Add Group or Condition', exact: true }).click()
  await page.waitForFunction(() => document.querySelectorAll('.rule-card-group').length === 3, null, { timeout: 2000 })
  assert.equal(await pill.count(), 2)
})

test('a ping on the latest check AND\'d with a web page that fails 2 in a row is not warned about: the web page must fail too', async t => {
  const { page } = await openForm(t)
  await card(page).waitFor()
  await textMatches(card(page), /ping has failed on the latest check and the web page has not answered with 200 2 checks in a row/)
  const text = await card(page).innerText()
  assert.match(text, /Group 1 needs two different tests to agree before it alerts\./)
  assert.match(text, /No test that can flicker alerts on a single failure\./)
  assert.doesNotMatch(text, /single failed check/)
  assert.equal(await card(page).locator('.rule-card-check.is-warn').count(), 0)
})

test('the card sits left of the form at 1440, above Condition Groups at 1100', async t => {
  const wide = await openForm(t)
  await card(wide.page).waitFor()
  const cardBox = await card(wide.page).boundingBox()
  const formBox = await wide.modal.boundingBox()
  assert.ok(cardBox.x + cardBox.width < formBox.x, `card ${JSON.stringify(cardBox)} left of the form ${JSON.stringify(formBox)}`)
  assert.ok(cardBox.width <= 340, 'at most 340px wide')
  assert.equal(await wide.modal.locator('.rule-card').count(), 0, 'not inside the form')

  const narrow = await openForm(t, { viewport: { width: 1100, height: 800 } })
  await card(narrow.page).waitFor()
  assert.equal(await card(narrow.page).count(), 1)
  assert.equal(await narrow.modal.locator('.rule-card').count(), 1, 'inside the form')
  const box = await card(narrow.page).boundingBox()
  const section = await narrow.modal.locator('.conditions-section').boundingBox()
  assert.ok(box.y + box.height <= section.y, `card ${JSON.stringify(box)} above Condition Groups ${JSON.stringify(section)}`)
})

test('at 1440 a note sits in the right margin and its line bends at a right angle under the field, crossing no input', async t => {
  const { page, modal, overlay } = await openForm(t)
  await clickMark(page, 'fc', 1)
  await waitForTitle(page, 'Fail Count')
  const control = await page.locator('[data-help-on]').boundingBox()
  const noteBox = await note(page).boundingBox()
  const formBox = await modal.boundingBox()
  assert.ok(noteBox.x >= formBox.x + formBox.width + 12, `note ${JSON.stringify(noteBox)} right of the form ${JSON.stringify(formBox)}`)
  assert.ok(inside(noteBox, { width: 1440, height: 900 }), 'inside the window')

  const points = (await page.locator('.help-lines polyline').getAttribute('points'))
    .trim().split(/\s+/).map(p => p.split(',').map(Number))
  assert.ok(points.length >= 3, 'a bent line')
  const [x0, y0] = points[0]
  assert.ok(Math.abs(x0 - noteBox.x) <= 1 && y0 >= noteBox.y && y0 <= noteBox.y + noteBox.height, 'it leaves the note\'s left edge')
  for (let i = 1; i < points.length; i++) {
    const [a, b] = [points[i - 1], points[i]]
    assert.ok(Math.abs(a[0] - b[0]) < 0.5 || Math.abs(a[1] - b[1]) < 0.5, `segment ${i} is horizontal or vertical`)
  }
  const [bendX, bendY] = points[points.length - 2]
  const [endX, endY] = points[points.length - 1]
  assert.ok(bendY > control.y + control.height, 'the bend is under the field')
  assert.ok(bendX >= control.x && bendX <= control.x + control.width, 'level with the field')
  assert.ok(Math.abs(endX - bendX) < 0.5 && endY < bendY, 'then straight up to the field')
  assert.ok(Math.abs(endY - (control.y + control.height + 4)) <= 1, 'ending on its outline')
  const dot = await page.locator('.help-lines circle').evaluate(c => [Number(c.getAttribute('cx')), Number(c.getAttribute('cy'))])
  assert.deepEqual(dot, [endX, endY], 'the dot ends the line')
  assert.equal(await page.locator('.help-lines polyline').evaluate(l => getComputedStyle(l).stroke), 'rgb(13, 148, 136)')
  // No segment crosses an input, select or text box
  const fields = await modal.locator('input, select, textarea').evaluateAll(els => els.map(e => e.getBoundingClientRect())
    .filter(r => r.width > 0 && r.height > 0).map(r => ({ x: r.x, y: r.y, width: r.width, height: r.height })))
  for (let i = 1; i < points.length; i++) {
    const [[ax, ay], [bx, by]] = [points[i - 1], points[i]]
    const seg = { x: Math.min(ax, bx), y: Math.min(ay, by), width: Math.abs(ax - bx) || 0.01, height: Math.abs(ay - by) || 0.01 }
    const crossed = fields.filter(f => overlap(seg, f))
    assert.deepEqual(crossed, [], `segment ${i} crosses a field`)
  }

  // A note never covers the form's help bar: its ‹, › and Done stay clickable
  await clickMark(page, 'name')
  await waitForTitle(page, 'Name')
  const bar = overlay.locator('.help-bar')
  for (const name of ['Previous note', 'Next note', 'Done']) {
    const button = bar.getByRole('button', { name, exact: true })
    const b = await button.boundingBox()
    assert.ok(await page.evaluate(([x, y, el]) => el.contains(document.elementFromPoint(x, y)),
      [b.x + b.width / 2, b.y + b.height / 2, await button.elementHandle()]), `${name} uncovered`)
  }
  assert.ok((await note(page).boundingBox()).x >= formBox.x + formBox.width, 'the Name note is in the margin too')
})

// Below 1280 there is no margin: a note opens below its field when it fits
// there, otherwise on the roomier side, scrolling inside (the Fail Count note,
// with its timeline, is taller than the room under a field in the middle of an
// 800px window)
test('at 1100 notes open below their field and stay inside the window (Review Focus 5)', async t => {
  const view = { width: 1100, height: 800 }
  const { page, overlay } = await openForm(t, { viewport: view })
  for (const [key, n, title] of [['name', 0, 'Name'], ['fc', 1, 'Fail Count'], ['andor', 0, 'AND / OR'], ['rcp', 0, 'Alert Recipients'],
    ['save', 0, 'Save and Cancel']]) {
    await clickMark(page, key, n)
    await waitForTitle(page, title)
    const control = await page.locator('[data-help-on]').boundingBox()
    const box = await note(page).boundingBox()
    assert.ok(inside(box, view), `${key}: note ${JSON.stringify(box)} inside the 1100×800 window`)
    assert.ok(!overlap(box, control), `${key}: the note leaves its field visible`)
    assert.equal(await page.locator('.help-lines polyline').count(), 0, `${key}: a straight line, no margin`)
    if (key === 'name' || key === 'andor') assert.ok(box.y >= control.y + control.height, `${key}: the note opens below the field`)
    const bar = await overlay.locator('.help-bar').boundingBox()
    if (bar.y + bar.height > 0) assert.ok(!overlap(box, bar), `${key}: the note keeps off the form's help bar`)
    await page.keyboard.press('Escape')
    await note(page).waitFor({ state: 'detached' })
  }
})

test('a name like <b>x</b> shows as typed in the card (Review Focus 4), and nothing in the form\'s help is orange', async t => {
  const { page, modal } = await openForm(t)
  await card(page).waitFor()
  await modal.locator('.form-group').filter({ has: page.locator('label', { hasText: /^Name$/ }) }).locator('input').fill('<b>x</b>')
  await textMatches(card(page), /<b>x<\/b> counts as down when:/)
  assert.equal(await card(page).locator('b').count(), 0, 'no b element: the name is text')
  assert.deepEqual(await orangeIn(page, '.rule-card'), [])
  await clickMark(page, 'fc')
  await waitForTitle(page, 'Fail Count')
  assert.deepEqual(await orangeIn(page, '.modal-overlay .help-bar, .btn-form-help, .help-mark, .help-note, .help-lines'), [])
})

test('typing into the field a note describes keeps the note open and its text follows; so does pressing the overlay\'s scrollbar', async t => {
  // A Chrome that draws scrollbars (the tests' Chrome hides them), so there is one to press
  const browser = await chromium.launch({
    ...(process.env.CHROME_PATH ? { executablePath: process.env.CHROME_PATH } : { channel: 'chrome' }),
    ignoreDefaultArgs: ['--hide-scrollbars'],
  })
  const { page, modal, overlay } = await openForm(t, { browser })
  t.after(() => browser.close()) // after the page has closed
  await clickMark(page, 'fc', 0)
  await waitForTitle(page, 'Fail Count')
  assert.match(await note(page).innerText(), /Every failed check alerts at once/)
  const input = field(modal, 'Fail Count', 0).locator('input')
  await input.click()
  await input.fill('3')
  await textMatches(note(page), /One or two failures in a row never alert\. A real outage alerts on the third failed check, 2 to 3 minutes in\./,
    'the note follows the field')
  assert.match(await note(page).innerText(), /a check every 60 seconds, Fail Count 3/)
  assert.equal(await note(page).count(), 1, 'still open')

  // The overlay's own scrollbar (drawn wide here, so there is one to press)
  await page.addStyleTag({ content: '.modal-overlay::-webkit-scrollbar { width: 14px } .modal-overlay::-webkit-scrollbar-thumb { background: #94a3b8 }' })
  assert.equal(await overlay.evaluate(el => el.offsetWidth - el.clientWidth), 14, 'the overlay has a scrollbar')
  const o = await overlay.boundingBox()
  await page.mouse.click(o.x + o.width - 7, o.y + o.height - 30)
  assert.equal(await note(page).count(), 1, 'pressing the scrollbar left the note open')

  // A click elsewhere in the form closes it; help stays on
  await modal.locator('h3').first().click()
  await note(page).waitFor({ state: 'detached' })
  assert.equal(await overlay.locator('.help-bar').count(), 1)
})

test('removing a condition while a note is open: the note stays with its own condition, or closes when that one goes', async t => {
  const { page, modal } = await openForm(t)
  const removeButton = n => modal.locator('.condition-card').nth(n).getByRole('button', { name: 'Remove', exact: true })

  // The web page's Fail Count note is open; the ping condition above it is removed (keyboard: no click elsewhere)
  await clickMark(page, 'fc', 1)
  await waitForTitle(page, 'Fail Count')
  await removeButton(0).focus()
  await page.keyboard.press('Enter')
  await page.waitForFunction(() => document.querySelectorAll('.condition-card').length === 1, null, { timeout: 2000 })
  await page.waitForFunction(() => document.querySelectorAll('.help-mark[data-help-mark="targetForm.fc"]').length === 1,
    null, { timeout: 2000 })
  assert.equal(await note(page).count(), 1, 'the note stays open')
  assert.match(await note(page).innerText(), /a check every 300 seconds, Fail Count 2/, 'on its own condition')
  assert.equal(await mark(page, 'fc').getAttribute('data-help-n'), '0')
  assert.equal(await mark(page, 'fc').getAttribute('aria-expanded'), 'true')
  await note(page).focus()
  await page.keyboard.press('Escape')
  await note(page).waitFor({ state: 'detached' })
  assert.deepEqual(await page.evaluate(() => [document.activeElement?.dataset.helpMark, document.activeElement?.dataset.helpN]),
    ['targetForm.fc', '0'], 'focus back on the note\'s mark')

  // Two more conditions; the third one's note is open when it is removed: the note closes
  await modal.getByRole('button', { name: '+ Add condition to group', exact: true }).click()
  await modal.getByRole('button', { name: '+ Add condition to group', exact: true }).click()
  await page.waitForFunction(() => document.querySelectorAll('.help-mark[data-help-mark="targetForm.fc"]').length === 3,
    null, { timeout: 2000 })
  await clickMark(page, 'fc', 2)
  await waitForTitle(page, 'Fail Count')
  await removeButton(2).focus()
  await page.keyboard.press('Enter')
  await page.waitForFunction(() => document.querySelectorAll('.condition-card').length === 2, null, { timeout: 2000 })
  await note(page).waitFor({ state: 'detached' })
  assert.equal(await page.locator('[data-help-on]').count(), 0, 'no outline left')
  assert.equal(await mark(page, 'fc').count(), 2)
  assert.equal(await page.locator('.modal-overlay .help-bar').count(), 1, 'help stays on')
})

test('the Fail Count and Time Window notes use the interval Bekci uses: 5 seconds counts as 10', async t => {
  const { page, modal } = await openForm(t)
  const interval = field(modal, 'Interval \\(seconds\\)', 0).locator('input')
  await interval.fill('5')
  await interval.press('Tab')
  await clickMark(page, 'fc', 0)
  await waitForTitle(page, 'Fail Count')
  await textMatches(note(page), /a check every 10 seconds, Fail Count 1/, 'the caption gives the interval Bekci runs')
  const failCount = field(modal, 'Fail Count', 0).locator('input')
  await failCount.click()
  await failCount.fill('3')
  await textMatches(note(page), /One or two failures in a row never alert\. A real outage alerts on the third failed check, 20 to 30 seconds in\./)
  assert.match(await note(page).innerText(), /a check every 10 seconds, Fail Count 3/)
  await failCount.press('Tab')
  await clickMark(page, 'tw', 0)
  await waitForTitle(page, 'Time Window')
  assert.match(await note(page).innerText(), /at least Fail Count × Interval \(here 3 × 10 = 30\), at most 1800/)
})

// The exception for typing into a field is the target form's: elsewhere a click
// on the control a note describes is a click on the page, and closes the note
test('outside a form field, a click on the control a note describes closes the note: the header dots, a Dashboard row', async t => {
  const { page } = await openPage(t, app, '/', { role: 'operator', viewport: { width: 1440, height: 900 } })
  await page.locator('[data-help="dashboard.row"]').first().waitFor()
  await turnHelpOn(page)
  const markOf = id => page.locator(`.help-mark[data-help-mark="${id}"]`)
  // "Click for details": the dots open the health popover, and the note gets out of its way
  await markOf('header.dots').click()
  await waitForTitle(page, 'Server health')
  await page.locator('.health-dots').click()
  await note(page).waitFor({ state: 'detached' })
  assert.equal(await page.locator('[data-help-on]').count(), 0)
  await page.locator('.health-popover').waitFor()
  await page.locator('.health-dots').click() // closes the popover again
  await page.locator('.health-popover').waitFor({ state: 'detached' })
  // A row note, then a click on that row to unfold it
  await markOf('dashboard.row').click()
  await waitForTitle(page, 'Target rows')
  await page.locator('[data-help="dashboard.row"]').first().click()
  await note(page).waitFor({ state: 'detached' })
  await page.locator('.check-row').first().waitFor()
  assert.equal(await page.locator('.help-bar').count(), 1, 'help stays on')
})
