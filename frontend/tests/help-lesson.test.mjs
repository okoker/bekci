// The alert rule lesson (design in_app_help.md §4.8) and the recipes window outside a
// form (§4.7). The lesson opens from the State note ("How alert rules work"), the
// Condition Groups note and the plain-words card ("Learn alert rules: the full
// lesson"); six parts in order, the Mail Gateway rule drawn as a diagram, the Fail
// Count lanes drawn from failCountTimeline, and a button to the recipes window.
// Outside a target form that window is read-only.
import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { openPage, orangeIn, startApp, stopApp, turnHelpOn } from './help-fixtures.mjs'
import { HELP_VERSION } from '../src/help/content/version.js'

let app
before(async () => { app = await startApp() })
after(() => stopApp(app))

const lesson = page => page.locator('.lesson-window')
const recipesWin = page => page.locator('.recipes-window')
const mark = (page, id) => page.locator(`.help-mark[data-help-mark="${id}"]`)
const FOOTER = 'To use a recipe, press + Target, then Start from a recipe.'
const USE = 'Use this recipe for your monitoring target!'
const LESSON_BUTTON = 'Learn alert rules: the full lesson'
const RECIPES_BUTTON = 'Rule recipes: ready-made rules'
const HEADINGS = [
  'How a rule is built',
  'Fail count: riding out blips',
  'AND or OR inside a group?',
  'Groups never confirm each other',
  'When people are told',
  'Mistakes that cause false alarms',
]

// Targets with help on, the State note open
async function stateNote(t, role, path = '/targets') {
  const { page } = await openPage(t, app, path, { role })
  if (path === '/search') {
    await page.locator('.search-input').fill('core')
    await page.locator('tbody tr').first().waitFor()
  } else {
    await page.locator('tbody tr').first().waitFor()
  }
  await turnHelpOn(page)
  await mark(page, `${path.slice(1)}.state`).click()
  await page.locator('.help-note').waitFor()
  return page
}

async function openLessonFromNote(t, role = 'operator') {
  const page = await stateNote(t, role)
  await page.locator('.help-note').getByRole('button', { name: 'How alert rules work', exact: true }).click()
  await lesson(page).waitFor()
  return page
}

test('the State note on Targets opens the lesson; its six headings come in the designed order', async t => {
  const page = await openLessonFromNote(t)
  assert.equal(await lesson(page).getAttribute('role'), 'dialog')
  assert.equal(await lesson(page).getAttribute('data-help-scope'), 'lesson')
  assert.equal((await lesson(page).locator('h2').first().innerText()).trim(), 'How alert rules work')
  assert.ok((await lesson(page).innerText()).includes(`Written for Bekci ${HELP_VERSION}`))
  const headings = (await lesson(page).locator('h3.lesson-heading').allInnerTexts()).map(s => s.trim())
  assert.deepEqual(headings, HEADINGS)
  // The frame is the recipes window's: 2px turquoise all round, 40px margins
  const box = await lesson(page).evaluate(el => {
    const s = getComputedStyle(el)
    const r = el.getBoundingClientRect()
    return { w: s.borderTopWidth, c: s.borderTopColor, l: s.borderLeftColor, left: r.left, top: r.top }
  })
  assert.equal(box.w, '2px')
  assert.equal(box.c, 'rgb(13, 148, 136)')
  assert.equal(box.l, 'rgb(13, 148, 136)')
  assert.equal(box.left, 40)
  assert.equal(box.top, 40)
  // Focus moved into it
  assert.equal(await page.evaluate(() => !!document.activeElement.closest('.lesson-window')), true)
  // The marks of the page under it are not drawn
  assert.equal(await page.locator('.help-mark').count(), 0)
})

test('part 1: three boxes and the Mail Gateway rule drawn as a diagram with "Read it as"', async t => {
  const page = await openLessonFromNote(t)
  const part = lesson(page).locator('section.lesson-part').nth(0)
  const boxes = part.locator('.lesson-term')
  assert.deepEqual((await boxes.locator('b').allInnerTexts()).map(s => s.trim()), ['Condition', 'Group', 'Rule'])
  const text = await boxes.allInnerTexts()
  assert.ok(text[0].includes('ping, status equals down, 3 in a row'))
  assert.ok(text[1].includes('AND') && text[1].includes('OR'))
  assert.ok(text[2].includes('always joined by OR'))
  // Mail Gateway: Group 1 (ping AND web page, 3 in a row) OR Group 2 (certificate)
  const diagram = part.locator('.rule-diagram')
  assert.equal(await diagram.locator('.rd-group').count(), 2)
  assert.deepEqual((await diagram.locator('.rd-group-label').allInnerTexts()).map(s => s.trim()), ['Group 1, AND', 'Group 2'])
  assert.equal(await diagram.locator('.rd-or').count(), 1)
  assert.equal((await diagram.locator('.rd-or').innerText()).trim(), 'OR')
  assert.equal(await diagram.locator('.rd-or').evaluate(el => getComputedStyle(el).backgroundColor), 'rgb(71, 85, 105)')
  assert.equal(await diagram.locator('.rd-group').first().evaluate(el => getComputedStyle(el).borderTopColor), 'rgb(147, 197, 253)')
  const read = (await part.locator('.lesson-read').innerText()).replace(/\s+/g, ' ')
  assert.ok(read.startsWith('Read it as:'), read)
  assert.ok(read.includes('Mail Gateway counts as down when'), read)
  assert.ok(read.includes('ping and the web page have both failed 3 checks in a row'), read)
  assert.ok(read.includes('fewer than 14 days left'), read)
})

test('part 2: three lanes drawn from the same ten results, with the counts the engine uses', async t => {
  const page = await openLessonFromNote(t)
  const part = lesson(page).locator('section.lesson-part').nth(1)
  const lane = id => part.locator(`.lesson-lane[data-lane="${id}"]`)
  const marks = id => lane(id).locator('.lesson-mark')
  const labels = id => lane(id).locator('.lesson-step-label')
  // ✓ ✓ ✕ ✕ ✓ ✕ ✕ ✕ ✕ ✓
  const results = (await marks('results').allInnerTexts()).map(s => s.trim())
  assert.deepEqual(results, ['✓', '✓', '✕', '✕', '✓', '✕', '✕', '✕', '✕', '✓'])
  assert.deepEqual((await marks('fc1').allInnerTexts()).map(s => s.trim()), results)
  assert.deepEqual((await marks('fc3').allInnerTexts()).map(s => s.trim()), results)
  const words = async id => (await labels(id).allInnerTexts()).map(s => s.trim())
  // Fail Count 1: every failed check is down
  assert.deepEqual(await words('fc1'), ['', '', 'down', 'down', '', 'down', 'down', 'down', 'down', ''])
  // Fail Count 3: 1, 2, reset, 1, 2, down, down
  assert.deepEqual(await words('fc3'), ['', '', '1', '2', 'reset', '1', '2', 'down', 'down', ''])
  assert.equal((await lane('fc1').locator('.lesson-lane-summary').innerText()).trim(), '2 alerts, 2 recoveries')
  assert.equal((await lane('fc3').locator('.lesson-lane-summary').innerText()).trim(), '1 alert, 1 recovery')
  assert.equal(await lane('results').locator('.lesson-lane-summary').count(), 0)
  const text = (await part.innerText()).replace(/\s+/g, ' ')
  assert.ok(text.includes('The time window must hold the whole run. Bekci sets it for you once Fail Count is above 1: at least Fail Count × Interval, at most 30 minutes.'), text)
  assert.ok(text.includes('checked once a minute'), text)
})

test('parts 3 to 6: AND and OR, groups, when people are told (the code defaults), the mistakes', async t => {
  const page = await openLessonFromNote(t)
  const parts = lesson(page).locator('section.lesson-part')
  const p3 = (await parts.nth(2).innerText()).replace(/\s+/g, ' ')
  assert.ok(p3.includes('Confirm') && p3.includes('All must be failing at the same time'), p3)
  assert.ok(p3.includes('Either') && p3.includes('Any one is enough'), p3)
  const p4 = (await parts.nth(3).innerText()).replace(/\s+/g, ' ')
  assert.ok(p4.includes('each group is a separate way to be down, so adding a group adds alerts'), p4)
  const p5 = (await parts.nth(4).innerText()).replace(/\s+/g, ' ')
  assert.ok(p5.includes('Settings › Alerting'), p5)
  assert.ok(p5.includes('30 minutes by default'), p5)
  assert.ok(p5.includes('every hour by default'), p5)
  assert.ok(p5.includes('a short outage may produce no recovery message'), p5)
  const items = (await parts.nth(5).locator('li').allInnerTexts()).map(s => s.replace(/\s+/g, ' ').trim())
  assert.equal(items.length, 4)
  assert.ok(items[0].includes('OR inside a group when you meant AND'))
  assert.ok(items[1].includes('Fail Count 1 on a link that drops the odd packet'))
  assert.ok(items[2].includes('Two groups when you meant one AND group'))
  assert.ok(items[3].includes('Page Hash on its own'))
})

test('the recipes button opens the window on top of the lesson, read-only, with the footer for an operator', async t => {
  const page = await openLessonFromNote(t, 'operator')
  await lesson(page).getByRole('button', { name: RECIPES_BUTTON, exact: true }).click()
  await recipesWin(page).waitFor()
  assert.equal(await lesson(page).count(), 1, 'the lesson stays under it')
  // On top: the recipes window is the element at its centre
  const onTop = await recipesWin(page).evaluate(el => {
    const r = el.getBoundingClientRect()
    return !!document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2)?.closest('.recipes-window')
  })
  assert.equal(onTop, true)
  assert.equal(await recipesWin(page).getByRole('button', { name: USE }).count(), 0)
  assert.equal((await recipesWin(page).locator('.recipes-readonly').innerText()).trim(), FOOTER)
  assert.equal(await recipesWin(page).getByRole('button', { name: 'Close', exact: true }).count(), 1)
})

test('the same window for an admin has the footer; for a viewer it has none', async t => {
  const admin = await openLessonFromNote(t, 'admin')
  await lesson(admin).getByRole('button', { name: RECIPES_BUTTON, exact: true }).click()
  await recipesWin(admin).waitFor()
  assert.equal((await recipesWin(admin).locator('.recipes-readonly').innerText()).trim(), FOOTER)
})

test('a viewer sees the lesson and the recipes read-only with no footer text', async t => {
  const page = await openLessonFromNote(t, 'viewer')
  await lesson(page).getByRole('button', { name: RECIPES_BUTTON, exact: true }).click()
  await recipesWin(page).waitFor()
  assert.equal(await recipesWin(page).getByRole('button', { name: USE }).count(), 0)
  assert.equal(await recipesWin(page).locator('.recipes-readonly').count(), 0)
  assert.equal((await recipesWin(page).innerText()).includes('To use a recipe'), false)
})

test('Esc closes one layer per press: the recipes window, then the lesson, then the note state, then help', async t => {
  const page = await openLessonFromNote(t)
  await lesson(page).getByRole('button', { name: RECIPES_BUTTON, exact: true }).click()
  await recipesWin(page).waitFor()
  await page.keyboard.press('Escape')
  await recipesWin(page).waitFor({ state: 'detached' })
  assert.equal(await lesson(page).count(), 1, 'the lesson is still open')
  // Focus is back on the button that opened the window
  assert.equal(await page.evaluate(() => document.activeElement.textContent.trim()), RECIPES_BUTTON)
  await page.keyboard.press('Escape')
  await lesson(page).waitFor({ state: 'detached' })
  assert.equal(await page.locator('.help-bar').count(), 1, 'help is still on')
  await page.keyboard.press('Escape')
  await page.locator('.help-bar').waitFor({ state: 'detached' })
})

test('the lesson closes with ✕ and Close, and leaving the page closes it', async t => {
  const page = await newForm(t)
  const open = async () => {
    await page.locator('.rule-card').getByRole('button', { name: LESSON_BUTTON, exact: true }).click()
    await lesson(page).waitFor()
  }
  await open()
  await lesson(page).getByRole('button', { name: 'Close lesson', exact: true }).click()
  await lesson(page).waitFor({ state: 'detached' })
  await open()
  await lesson(page).getByRole('button', { name: 'Close', exact: true }).click()
  await lesson(page).waitFor({ state: 'detached' })
  await open()
  await page.locator('.navbar a[href="/alerts"]').evaluate(el => el.click())
  await lesson(page).waitFor({ state: 'detached' })
})

test('the Search State note links to the lesson too', async t => {
  const page = await stateNote(t, 'operator', '/search')
  await page.locator('.help-note').getByRole('button', { name: 'How alert rules work', exact: true }).click()
  await lesson(page).waitFor()
})

// The target form on New, help on: the card has its two buttons
async function newForm(t, role = 'operator') {
  const { page } = await openPage(t, app, '/targets', { role, viewport: { width: 1440, height: 900 } })
  await page.locator('tbody tr').first().waitFor()
  await turnHelpOn(page)
  await page.getByRole('button', { name: '+ Target', exact: true }).click()
  await page.locator('.rule-card').waitFor()
  return page
}

test('the card has two buttons; the recipes one opens the window that can apply a recipe', async t => {
  const page = await newForm(t)
  const card = page.locator('.rule-card')
  assert.equal(await card.getByRole('button').count(), 2)
  const buttons = (await card.getByRole('button').allInnerTexts()).map(s => s.trim())
  assert.deepEqual(buttons, [RECIPES_BUTTON, LESSON_BUTTON])
  await card.getByRole('button', { name: RECIPES_BUTTON, exact: true }).click()
  await recipesWin(page).waitFor()
  assert.equal(await recipesWin(page).getByRole('button', { name: USE, exact: true }).count(), 1)
  assert.equal(await recipesWin(page).locator('.recipes-readonly').count(), 0)
  await recipesWin(page).getByRole('button', { name: USE, exact: true }).click()
  await recipesWin(page).waitFor({ state: 'detached' })
  assert.equal(await page.locator('.recipe-banner').count(), 1, 'the recipe went into the form')
})

test('the card lesson button opens the lesson; Esc closes it and focus returns to the button', async t => {
  const page = await newForm(t)
  await page.locator('.rule-card').getByRole('button', { name: LESSON_BUTTON, exact: true }).click()
  await lesson(page).waitFor()
  assert.equal(await page.evaluate(() => !!document.activeElement.closest('.lesson-window')), true)
  await page.keyboard.press('Escape')
  await lesson(page).waitFor({ state: 'detached' })
  assert.equal(await page.evaluate(() => document.activeElement.textContent.trim()), LESSON_BUTTON)
  assert.equal(await page.locator('.rule-card').count(), 1, 'help and the form are still up')
})

test('opened from the form, the lesson reaches a recipes window that can apply, and using one closes both', async t => {
  const page = await newForm(t)
  await page.locator('.rule-card').getByRole('button', { name: LESSON_BUTTON, exact: true }).click()
  await lesson(page).waitFor()
  await lesson(page).getByRole('button', { name: RECIPES_BUTTON, exact: true }).click()
  await recipesWin(page).waitFor()
  await recipesWin(page).getByRole('button', { name: USE, exact: true }).click()
  await recipesWin(page).waitFor({ state: 'detached' })
  await lesson(page).waitFor({ state: 'detached' })
  assert.equal(await page.locator('.recipe-banner').count(), 1)
})

test('the Condition Groups note in the form links to "Learn alert rules: the full lesson"', async t => {
  const page = await newForm(t)
  await page.locator('[data-help="targetForm.groups"]').first().evaluate(el => el.scrollIntoView({ block: 'center' }))
  await mark(page, 'targetForm.groups').click()
  await page.locator('.help-note').getByRole('button', { name: LESSON_BUTTON, exact: true }).click()
  await lesson(page).waitFor()
})

test('nothing in the lesson, or the card buttons, is orange', async t => {
  const page = await newForm(t)
  assert.deepEqual(await orangeIn(page, '.rule-card'), [])
  await page.locator('.rule-card').getByRole('button', { name: LESSON_BUTTON, exact: true }).click()
  await lesson(page).waitFor()
  assert.deepEqual(await orangeIn(page, '.lesson-window'), [])
})
