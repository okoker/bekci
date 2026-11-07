// Rule recipes (design in_app_help.md §4.7): "Start from a recipe" in the target
// form (New, Clone and Edit; help on or off) opens a window of the 12 ready-made
// alert rules. "Use this recipe for your monitoring target!" puts the chosen one's
// conditions into the form, asking first when the form has some, with a banner and
// Undo. A field the person must supply is marked in turquoise, and Save is refused
// while one is empty. What Save sends is exactly the form's conditions: the recipe's
// `needs` never reach the server.
import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { apiFixtures, openPage, orangeIn, startApp, stopApp, turnHelpOn } from './help-fixtures.mjs'
import { recipes } from '../src/help/recipes.js'
import { needError, needOf } from '../src/help/applyConditions.js'
import { HELP_VERSION } from '../src/help/content/version.js'

let app
before(async () => { app = await startApp() })
after(() => stopApp(app))

const recipe = id => recipes.find(r => r.id === id)
// The Web shop target as the form loads it: HTTP "Web page" (c2) AND ping (c3)
const EDIT_CONDITIONS = apiFixtures()['/api/targets/t2'].conditions
const USE = 'Use this recipe for your monitoring target!'
const escape = s => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

// Targets as an operator; help on when asked; then the form, opened with `button`
// (+ Target, Edit or Clone), with Web shop's conditions on screen for Edit and Clone
async function openForm(t, { button = '+ Target', help = false, viewport = { width: 1440, height: 900 } } = {}) {
  const { page, writes } = await openPage(t, app, '/targets', { role: 'operator', viewport })
  const row = page.locator('tbody tr', { hasText: 'Web shop' })
  await row.waitFor()
  if (help) await turnHelpOn(page)
  if (button === '+ Target') await page.getByRole('button', { name: '+ Target', exact: true }).click()
  else await row.getByRole('button', { name: button, exact: true }).click()
  const overlay = page.locator('.modal-overlay').filter({ has: page.locator('.modal-wide') })
  const modal = overlay.locator('.modal-card')
  await modal.waitFor()
  if (button !== '+ Target') await modal.locator('.condition-card').nth(1).waitFor()
  return { page, writes, overlay, modal }
}

const startButton = modal => modal.getByRole('button', { name: 'Start from a recipe', exact: true })
const win = page => page.locator('.recipes-window')
const useButton = page => win(page).getByRole('button', { name: USE, exact: true })
const banner = modal => modal.locator('.recipe-banner')
const confirmBox = page => page.locator('.recipe-confirm')
const cards = modal => modal.locator('.condition-card')

async function openRecipes(modal) {
  await startButton(modal).click()
  await win(modal.page()).waitFor()
}

async function pick(page, id) {
  await win(page).locator(`.recipe-row[data-recipe="${id}"]`).click()
  await win(page).locator('.recipe-name', { hasText: new RegExp(`^${escape(recipe(id).name)}$`) }).waitFor()
}

// Opens the window, picks the recipe and presses the main button
async function useRecipe(modal, id) {
  await openRecipes(modal)
  await pick(modal.page(), id)
  await useButton(modal.page()).click()
  await win(modal.page()).waitFor({ state: 'detached' })
}

// A target field (Name, Host, Category), or the input of a condition's field
const topField = (modal, label) => modal.locator('.form-group')
  .filter({ has: modal.page().locator('label').filter({ hasText: new RegExp(`^${escape(label)}$`) }) })
  .locator('input, select').first()
const condField = (modal, label, n = 0) => cards(modal).nth(n).locator('.form-group')
  .filter({ has: modal.page().locator('label').filter({ hasText: new RegExp(`^${escape(label)}$`) }) })
  .locator('input, select').first()

async function fillTarget(modal) {
  await topField(modal, 'Name').fill('Recipe test')
  await topField(modal, 'Host').fill('example.invalid')
  await topField(modal, 'Category').selectOption('Other')
}

// Presses Save and returns the body of the target's POST or PUT (the form closes on success)
async function save(modal, writes, method) {
  await modal.getByRole('button', { name: 'Save', exact: true }).click()
  await modal.waitFor({ state: 'detached' })
  const write = writes.find(w => w.method === method && /^\/api\/targets(\/t2)?$/.test(w.path))
  assert.ok(write, `a ${method} of the target (writes: ${JSON.stringify(writes.map(w => `${w.method} ${w.path}`))})`)
  return write.body
}

// What Save sends for a condition a recipe put in: the form's keys and nothing else
// (no check_id for a new check, no `needs`), with the values filled in
const sent = (c, fill = {}) => ({
  check_type: c.check_type, check_name: c.check_name, config: { ...c.config, ...fill },
  interval_s: c.interval_s, field: c.field, comparator: c.comparator, value: c.value,
  fail_count: c.fail_count, fail_window: c.fail_window, condition_group: c.condition_group,
  group_operator: c.group_operator,
})
const parsed = conditions => conditions.map(c => ({ ...c, config: JSON.parse(c.config) }))

// Lets Vue update and any request leave
const settle = page => page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))

// A field's validity as the browser sees it on Save
const validity = locator => locator.evaluate(el => ({
  message: el.validationMessage, customError: el.validity.customError, focused: el === document.activeElement,
}))

test('needError and needOf: the recipe\'s message while a needed field is empty or doesn\'t match its pattern', () => {
  const port = { condition: 0, field: 'config.port', placeholder: "the service's port", message: "This recipe needs the service's port here.", pattern: null }
  const path = { field: 'config.endpoint', placeholder: 'x', message: 'Needs a path.', pattern: '^/\\S' }
  const tcp = config => ({ check_type: 'tcp', config, needs: [port] })
  assert.equal(needOf(tcp({ port: '' }), 'config.port'), port)
  assert.equal(needOf(tcp({ port: '' }), 'config.timeout_s'), null)
  assert.equal(needOf({ config: {} }, 'config.port'), null, 'a condition no recipe put in')
  assert.equal(needError(tcp({ port: '' }), 'config.port'), port.message)
  assert.equal(needError(tcp({ port: '  ' }), 'config.port'), port.message, 'blank is empty')
  assert.equal(needError(tcp({}), 'config.port'), port.message, 'missing is empty')
  assert.equal(needError(tcp({ port: 22 }), 'config.port'), '')
  assert.equal(needError({ check_type: 'tcp', config: { port: '' } }, 'config.port'), '', 'no need: nothing to say')
  const http = endpoint => ({ check_type: 'http', config: { endpoint }, needs: [path] })
  assert.equal(needError(http('admin'), 'config.endpoint'), 'Needs a path.')
  assert.equal(needError(http('/'), 'config.endpoint'), 'Needs a path.', 'the home page is not a path')
  assert.equal(needError(http('/admin'), 'config.endpoint'), '')
})

test('"Start from a recipe" sits next to "+ Add Group or Condition" on New, Clone and Edit, in help\'s turquoise', async t => {
  const { page, modal } = await openForm(t)
  for (const button of ['+ Target', 'Clone', 'Edit']) {
    if (button !== '+ Target') {
      await page.locator('tbody tr', { hasText: 'Web shop' }).getByRole('button', { name: button, exact: true }).click()
      await modal.locator('.condition-card').nth(1).waitFor()
    }
    const header = modal.locator('.conditions-header').first()
    await startButton(modal).waitFor()
    assert.equal(await header.getByRole('button', { name: 'Start from a recipe', exact: true }).count(), 1, `${button}: in the Condition Groups header`)
    assert.equal(await header.getByRole('button', { name: '+ Add Group or Condition', exact: true }).count(), 1)
    const style = await startButton(modal).evaluate(el => {
      const s = getComputedStyle(el)
      return [s.color, s.borderTopColor, s.backgroundColor]
    })
    assert.deepEqual(style, ['rgb(15, 118, 110)', 'rgb(153, 246, 228)', 'rgb(240, 253, 250)'], `${button}: turquoise`)
    assert.deepEqual(await orangeIn(page, '.modal-card .btn-recipe'), [])
    await modal.getByRole('button', { name: 'Cancel', exact: true }).click()
    await modal.waitFor({ state: 'detached' })
  }
})

test('with help off, the window opens from the form: header, three shelves of twelve recipes, the chosen one\'s details, the main button; Esc closes it', async t => {
  const { page, modal } = await openForm(t)
  await openRecipes(modal)
  const w = win(page)
  await settle(page) // the layer has measured the page
  assert.equal(await page.locator('.help-bar').count(), 0, 'help stays off')
  assert.equal(await page.locator('.help-mark').count(), 0, 'no marks with help off')
  assert.equal(await w.getAttribute('role'), 'dialog')

  // Header
  assert.equal(await w.locator('#recipes-title').innerText(), 'Rule recipes')
  assert.equal(await w.locator('.recipes-sub').innerText(), 'Ready-made alert rules for common jobs. Each one is set up to alert on real problems, not blips.')
  assert.equal(await w.locator('.recipes-version').innerText(), `Written for Bekci ${HELP_VERSION}`)
  await w.locator('.recipes-x').waitFor()

  // The frame: over the form with 40px margins, a 2px turquoise border all round, radius 16
  const box = await w.boundingBox()
  assert.ok(Math.abs(box.x - 40) <= 1 && Math.abs(box.y - 40) <= 1 && Math.abs(box.width - 1360) <= 1 && Math.abs(box.height - 820) <= 1,
    `40px margins (${JSON.stringify(box)})`)
  const frame = await w.evaluate(el => {
    const s = getComputedStyle(el)
    return [s.borderTopWidth, s.borderRightWidth, s.borderBottomWidth, s.borderLeftWidth, s.borderTopColor, s.borderLeftColor, s.borderTopLeftRadius]
  })
  assert.deepEqual(frame, ['2px', '2px', '2px', '2px', 'rgb(13, 148, 136)', 'rgb(13, 148, 136)', '16px'])
  const onTop = await page.evaluate(() => {
    const el = document.elementFromPoint(innerWidth / 2, innerHeight / 2)
    return !!el?.closest('.recipes-window')
  })
  assert.ok(onTop, 'the window draws above the form')

  // Three shelves, twelve rows in the recipes' order
  const shelves = w.locator('.recipe-shelf')
  assert.equal(await shelves.count(), 3)
  assert.deepEqual(await shelves.locator('.recipe-shelf-title').allInnerTexts(), ['Basic: one test', 'Combined: several tests', 'Security: catch tampering'])
  assert.deepEqual(await shelves.locator('.recipe-shelf-note').allInnerTexts(), [
    'One test, a few failures in a row.',
    "They confirm each other, so one flaky test can't page anyone.",
    "Set up so an outage can't pass for an attack.",
  ])
  const rows = w.locator('.recipe-row')
  assert.equal(await rows.count(), 12)
  assert.deepEqual(await rows.evaluateAll(els => els.map(el => el.dataset.recipe)), recipes.map(r => r.id))
  assert.deepEqual(await rows.locator('.recipe-row-name').allInnerTexts(), recipes.map(r => r.name))
  assert.deepEqual(await rows.locator('.recipe-row-formula').allInnerTexts(), recipes.map(r => r.formula))
  assert.equal(await shelves.nth(2).locator('.recipe-row').count(), 4, 'four on the security shelf')
  const shelfBox = await w.locator('.recipes-shelves').boundingBox()
  assert.ok(Math.abs(shelfBox.width - 330) <= 1, `the shelves column is 330px (${shelfBox.width})`)

  // S3 chosen: its details
  await pick(page, 'S3')
  const s3 = recipe('S3')
  assert.equal(await w.locator('.recipe-row[data-recipe="S3"]').getAttribute('aria-pressed'), 'true')
  assert.equal(await w.locator('.recipe-row[aria-pressed="true"]').count(), 1, 'one row chosen')
  assert.equal(await w.locator('.recipe-shelf-tag').innerText(), 'Security: catch tampering')
  assert.equal(await w.locator('.recipe-name').innerText(), 'A port that must stay closed')
  assert.equal(await w.locator('.recipe-catches').innerText(), s3.catches)
  assert.equal(await w.locator('.recipe-read').innerText(), `Read it as: ${s3.readAs}`)
  assert.equal(await w.locator('.recipe-situations h3').innerText(), "What alerts, and what doesn't")
  assert.deepEqual(await w.locator('.recipe-situation-text').allInnerTexts(), s3.situations.map(s => s.text))
  assert.deepEqual(await w.locator('.recipe-pill').allInnerTexts(), ['no alert', 'no alert', 'alerts'])
  assert.equal(await w.locator('.recipe-before h3').innerText(), 'Before you use it')
  assert.deepEqual(await w.locator('.recipe-before li').allInnerTexts(), s3.before)
  assert.equal(await w.locator('.recipe-table-title').innerText(), 'What it puts in the form')
  assert.deepEqual(await w.locator('.recipe-table th').allInnerTexts(), ['Check', 'Settings', 'Every', 'Alert when', 'Fail count', 'Window'])
  const tableRows = w.locator('.recipe-table tbody tr')
  assert.equal(await tableRows.count(), 1, 'one condition, so no group row')
  assert.deepEqual(await tableRows.first().locator('td').allInnerTexts(), [
    'Remote desktop port\nTCP', 'port 3389, timeout 5 s, Port must stay closed', '5 min', 'Status equals down', '2', '600 s',
  ])

  // The main button: centred, at least 460px wide, the user's label
  const use = useButton(page)
  assert.equal(await use.innerText(), USE)
  const useBox = await use.boundingBox()
  assert.ok(useBox.width >= 460, `at least 460px wide (${useBox.width})`)
  assert.ok(Math.abs(useBox.x + useBox.width / 2 - (box.x + box.width / 2)) <= 2, 'centred in the window')
  const look = await use.evaluate(el => {
    const s = getComputedStyle(el)
    return [s.backgroundColor, s.color, s.fontSize, s.fontWeight, s.borderTopLeftRadius]
  })
  assert.deepEqual(look, ['rgb(15, 118, 110)', 'rgb(255, 255, 255)', '16px', '700', '10px'])
  assert.equal(await w.locator('.recipes-use-note').innerText(), 'Puts these checks into the form. You fill in anything marked in turquoise, then Save.')
  assert.equal(await w.locator('.recipes-close').innerText(), 'Close')

  // Esc closes it, with help off too; nothing changed in the form
  await page.keyboard.press('Escape')
  await w.waitFor({ state: 'detached' })
  assert.equal(await cards(modal).count(), 0)
  assert.equal(await banner(modal).count(), 0)
  assert.equal(await page.locator('.help-bar').count(), 0, 'help still off')
  await modal.waitFor() // the form is still open
})

test('the diagram: groups in blue boxes, AND between conditions, slate OR pills between groups; tests that must pass are green, with the legend', async t => {
  const { page, modal } = await openForm(t)
  await openRecipes(modal)
  const w = win(page)
  const legend = "Green tests must be passing for the rule to fire, so an outage can't set it off."

  await pick(page, 'C2')
  const groups = w.locator('.rule-diagram .rd-group')
  assert.equal(await groups.count(), 2)
  assert.deepEqual(await groups.locator('.rd-group-label').allInnerTexts(), ['Group 1, AND', 'Group 2'])
  assert.deepEqual(await groups.nth(0).locator('.rd-chip').allInnerTexts(), ['Ping\n3 in a row', 'Web page\n3 in a row'])
  assert.deepEqual(await groups.nth(0).locator('.rd-join').allInnerTexts(), ['AND'])
  assert.deepEqual(await groups.nth(1).locator('.rd-chip').allInnerTexts(), ['Web page, patient\n5 in a row'])
  const or = w.locator('.rule-diagram .rd-or')
  assert.equal(await or.count(), 1)
  assert.deepEqual(await or.evaluate(el => [el.innerText, getComputedStyle(el).backgroundColor, getComputedStyle(el).color]),
    ['OR', 'rgb(71, 85, 105)', 'rgb(255, 255, 255)'])
  assert.equal(await groups.first().evaluate(el => getComputedStyle(el).borderTopColor), 'rgb(147, 197, 253)')
  assert.equal(await w.locator('.rd-chip.is-passing').count(), 0)
  assert.equal(await w.locator('.recipe-legend').count(), 0, 'no legend without a green test')
  // The table has a blue row per group
  assert.deepEqual(await w.locator('.recipe-table tr.recipe-group-row').allInnerTexts(), ['Group 1, AND', 'Group 2'])
  assert.equal(await w.locator('.recipe-table tbody tr').count(), 5)

  await pick(page, 'S2')
  assert.deepEqual(await w.locator('.rd-chip').allInnerTexts(), ['DNS at Google\n2 in a row', 'DNS at Cloudflare\n2 in a row', 'Internet canary\nlatest check'])
  assert.deepEqual(await w.locator('.rd-chip.is-passing').allInnerTexts(), ['Internet canary\nlatest check'])
  assert.equal(await w.locator('.rd-chip.is-passing').evaluate(el => getComputedStyle(el).backgroundColor), 'rgb(240, 253, 244)')
  assert.equal(await w.locator('.recipe-legend').innerText(), legend)
  // A field the person fills in shows as its placeholder, marked in turquoise
  assert.deepEqual(await w.locator('.recipe-table .recipe-need').allInnerTexts(), ['your public address', 'your public address'])

  await pick(page, 'S1')
  assert.deepEqual(await w.locator('.rd-chip.is-passing').allInnerTexts(), ['Web page answers\n2 in a row'])
  assert.equal(await w.locator('.recipe-legend').innerText(), legend)
  const upCell = w.locator('.recipe-table tbody tr').nth(2).locator('td').nth(3)
  assert.equal(await upCell.innerText(), 'Status equals up')

  await pick(page, 'B1')
  assert.equal(await w.locator('.recipe-legend').count(), 0)
  assert.deepEqual(await w.locator('.rd-group-label').allInnerTexts(), ['Group 1'])
})

test('New + S3: the banner and Undo, the form scrolled to Condition Groups, and the POST sends exactly the recipe\'s condition', async t => {
  const { page, writes, overlay, modal } = await openForm(t)
  await fillTarget(modal)
  await useRecipe(modal, 'S3')
  assert.equal(await confirmBox(page).count(), 0, 'an empty form is not asked about')
  await banner(modal).waitFor()
  assert.equal(await banner(modal).locator('.recipe-banner-text').innerText(),
    'Filled in from the recipe A port that must stay closed. Check the settings, then Save.')
  await banner(modal).getByRole('button', { name: 'Undo', exact: true }).waitFor()
  assert.deepEqual(await orangeIn(page, '.recipe-banner'), [])

  // The form holds the recipe's check
  assert.equal(await cards(modal).count(), 1)
  assert.equal(await condField(modal, 'Check Name').inputValue(), 'Remote desktop port')
  assert.equal(await condField(modal, 'Port').inputValue(), '3389')
  assert.equal(await cards(modal).first().getByLabel('Port must stay closed').isChecked(), true)

  // Scrolled to Condition Groups: to the top of the window, or as far as the form scrolls
  const scrolled = await overlay.evaluate(el => ({
    scrollTop: el.scrollTop, end: el.scrollHeight - el.clientHeight,
    top: el.querySelector('.conditions-section').getBoundingClientRect().top,
  }))
  assert.ok(scrolled.scrollTop > 0 && scrolled.top >= -1 && (scrolled.top < 40 || scrolled.scrollTop >= scrolled.end - 1)
    && scrolled.top < 900 / 2, `Condition Groups brought up (${JSON.stringify(scrolled)})`)

  // Undo puts back what the form had: nothing
  await banner(modal).getByRole('button', { name: 'Undo', exact: true }).click()
  await banner(modal).waitFor({ state: 'detached' })
  assert.equal(await cards(modal).count(), 0)
  await modal.locator('.validation-warning').waitFor()

  // Again, and Save
  await useRecipe(modal, 'S3')
  await banner(modal).waitFor()
  const body = await save(modal, writes, 'POST')
  assert.deepEqual(parsed(body.conditions), recipe('S3').conditions.map(c => sent(c)))
  assert.equal(JSON.parse(body.conditions[0].config).expect_closed, true)
  assert.ok(body.conditions.every(c => !('needs' in c)), 'no needs key')
  assert.equal(body.preferred_check_type, 'tcp')
})

test('New + B3: the port is marked in turquoise; Save is refused with the recipe\'s message, nothing is sent; 22 is sent', async t => {
  const { page, writes, modal } = await openForm(t)
  await fillTarget(modal)
  await useRecipe(modal, 'B3')
  await banner(modal).waitFor()
  assert.equal(await banner(modal).locator('.recipe-banner-text').innerText(),
    'Filled in from the recipe Service port open. Fill in the fields marked in turquoise, then Save.')

  const port = condField(modal, 'Port')
  assert.equal(await port.inputValue(), '')
  assert.equal(await port.getAttribute('placeholder'), "the service's port")
  const marking = () => port.evaluate(el => {
    const s = getComputedStyle(el)
    return { style: s.borderTopStyle, color: s.borderTopColor, background: s.backgroundColor,
      placeholder: getComputedStyle(el, '::placeholder').fontStyle }
  })
  assert.deepEqual(await marking(), { style: 'dashed', color: 'rgb(13, 148, 136)', background: 'rgb(240, 253, 250)', placeholder: 'italic' })
  await port.focus()
  assert.deepEqual(await orangeIn(page, '.condition-card input.need-field'), [], 'focused, the field stays turquoise')
  await port.blur()

  await modal.getByRole('button', { name: 'Save', exact: true }).click()
  await settle(page)
  assert.deepEqual(await validity(port), { message: "This recipe needs the service's port here.", customError: true, focused: true })
  assert.deepEqual(writes, [], 'nothing sent')

  await port.fill('22')
  const filled = await marking()
  assert.equal(filled.style, 'solid', 'the marking clears once the field has a value')
  assert.notEqual(filled.background, 'rgb(240, 253, 250)')
  assert.equal((await validity(port)).message, '')
  const body = await save(modal, writes, 'POST')
  assert.deepEqual(parsed(body.conditions), [sent(recipe('B3').conditions[0], { port: 22 })])
  assert.ok(body.conditions.every(c => !('needs' in c)), 'no needs key')
})

test('the Save guard refuses an empty needed field even when the browser\'s own check is bypassed', async t => {
  const { page, writes, modal } = await openForm(t)
  await fillTarget(modal)
  await useRecipe(modal, 'B3')
  await modal.locator('form').dispatchEvent('submit')
  await settle(page)
  assert.deepEqual(writes, [])
})

test('S4: the admin path must start with "/": admin is refused, /admin is sent', async t => {
  const { page, writes, modal } = await openForm(t)
  await fillTarget(modal)
  await useRecipe(modal, 'S4')
  assert.equal(await cards(modal).count(), 2)
  const endpoint = condField(modal, 'Endpoint')
  assert.equal(await endpoint.getAttribute('placeholder'), "the admin page's path, such as /admin")
  assert.equal(await condField(modal, 'Endpoint', 1).getAttribute('placeholder'), '/', 'the second check needs nothing')
  await endpoint.fill('admin')
  await modal.getByRole('button', { name: 'Save', exact: true }).click()
  await settle(page)
  assert.deepEqual(await validity(endpoint), {
    message: "This recipe needs the admin page's path here, starting with /.", customError: true, focused: true,
  })
  assert.deepEqual(writes, [])
  await endpoint.fill('/admin')
  assert.equal((await validity(endpoint)).message, '')
  const body = await save(modal, writes, 'POST')
  assert.deepEqual(parsed(body.conditions), recipe('S4').conditions.map((c, i) => sent(c, i === 0 ? { endpoint: '/admin' } : {})))
  assert.ok(body.conditions.every(c => !('needs' in c)), 'no needs key')
  assert.equal(body.preferred_check_type, 'http')
})

test('Edit with 2 conditions: asks first; Cancel keeps them; Replace replaces; Undo puts back the originals and the PUT keeps their check ids', async t => {
  const { page, writes, modal } = await openForm(t, { button: 'Edit' })
  const names = () => cards(modal).locator('.form-group')
    .filter({ has: page.locator('label').filter({ hasText: /^Check Name$/ }) }).locator('input').evaluateAll(els => els.map(el => el.value))
  assert.deepEqual(await names(), ['Web page', 'Ping'])

  await useRecipe(modal, 'S3')
  const ask = confirmBox(page)
  await ask.waitFor()
  assert.equal(await ask.getAttribute('data-help-scope'), 'dialog')
  assert.equal(await ask.locator('.recipe-confirm-text').innerText(),
    'Replace the 2 conditions in this form with this recipe? On a saved target, Save then deletes the replaced checks and their history, SLA included.')
  assert.deepEqual(await ask.getByRole('button').allInnerTexts(), ['Replace', 'Cancel'])
  await page.waitForFunction(() => document.activeElement?.textContent === 'Cancel', null, { timeout: 2000 })
  assert.deepEqual(await orangeIn(page, '.recipe-confirm'), [])

  // Cancel: nothing changes
  await ask.getByRole('button', { name: 'Cancel', exact: true }).click()
  await ask.waitFor({ state: 'detached' })
  assert.deepEqual(await names(), ['Web page', 'Ping'])
  assert.equal(await banner(modal).count(), 0)

  // Replace
  await useRecipe(modal, 'S3')
  await ask.getByRole('button', { name: 'Replace', exact: true }).click()
  await ask.waitFor({ state: 'detached' })
  await banner(modal).waitFor()
  assert.deepEqual(await names(), ['Remote desktop port'])
  assert.equal(await condField(modal, 'Port').inputValue(), '3389')

  // Undo: the originals, with their check ids, and the Preferred Check they had
  await banner(modal).getByRole('button', { name: 'Undo', exact: true }).click()
  await banner(modal).waitFor({ state: 'detached' })
  assert.deepEqual(await names(), ['Web page', 'Ping'])
  assert.equal(await topField(modal, 'Preferred Check (for SLA & dashboard)').inputValue(), 'http')
  const body = await save(modal, writes, 'PUT')
  assert.deepEqual(body.conditions, EDIT_CONDITIONS, 'the original conditions, check ids c2 and c3 included')
  assert.equal(body.preferred_check_type, 'http')
})

test('✕, Close and Esc close the window and change nothing in the form', async t => {
  const { page, writes, modal } = await openForm(t, { button: 'Edit' })
  const closers = {
    '✕': () => win(page).locator('.recipes-x').click(),
    Close: () => win(page).locator('.recipes-close').click(),
    Esc: () => page.keyboard.press('Escape'),
  }
  for (const [name, close] of Object.entries(closers)) {
    await openRecipes(modal)
    await pick(page, 'S3')
    await close()
    await win(page).waitFor({ state: 'detached' })
    assert.equal(await confirmBox(page).count(), 0, `${name}: nothing asked`)
    assert.equal(await banner(modal).count(), 0, `${name}: no banner`)
    assert.equal(await cards(modal).count(), 2, `${name}: the form's conditions stay`)
  }
  const body = await save(modal, writes, 'PUT')
  assert.deepEqual(body.conditions, EDIT_CONDITIONS)
})

test('with help on, the window and the replace question hide the form\'s marks; Esc closes the window first, then help', async t => {
  const { page, modal } = await openForm(t, { button: 'Edit', help: true })
  const marks = page.locator('.help-mark')
  await marks.first().waitFor()
  // The ? mark left of "+ Add Group or Condition" keeps clear of "Start from a recipe"
  const addMark = await page.locator('.help-mark[data-help-mark="targetForm.addgroup"]').boundingBox()
  const recipeBox = await startButton(modal).boundingBox()
  const addBox = await modal.getByRole('button', { name: '+ Add Group or Condition', exact: true }).boundingBox()
  assert.ok(addMark.x >= recipeBox.x + recipeBox.width + 2 && addMark.x + addMark.width <= addBox.x,
    `the mark sits between the two buttons (${JSON.stringify({ addMark, recipeBox, addBox })})`)
  // Wait until nothing else on the page is changing (the form's recipients are listed, the
  // header's health dots have finished their colour change), so the marks must go because
  // the window opened, not because a late change elsewhere made the layer look again
  await modal.locator('.recipient-item').nth(2).waitFor()
  await page.waitForFunction(() => document.getAnimations().length === 0, null, { timeout: 2000 })
  await settle(page)
  await openRecipes(modal)
  await page.waitForFunction(() => document.querySelectorAll('.help-mark').length === 0, null, { timeout: 2000 })
  assert.equal(await page.evaluate(() => {
    const el = document.elementFromPoint(innerWidth / 2, innerHeight / 2)
    return !!el?.closest('.recipes-window')
  }), true, 'the window draws above the form and its help')

  await page.keyboard.press('Escape')
  await win(page).waitFor({ state: 'detached' })
  await page.locator('.modal-overlay .help-bar').waitFor()
  await marks.first().waitFor()

  // The replace question is a dialog: no marks while it is open
  await useRecipe(modal, 'B1')
  await confirmBox(page).waitFor()
  await page.waitForFunction(() => document.querySelectorAll('.help-mark').length === 0, null, { timeout: 2000 })
  await confirmBox(page).getByRole('button', { name: 'Cancel', exact: true }).click()
  await marks.first().waitFor()

  // The next Esc turns help off
  await startButton(modal).focus()
  await page.keyboard.press('Escape')
  await page.locator('.help-bar').first().waitFor({ state: 'detached' })
  assert.equal(await page.locator('.help-bar').count(), 0)
})

test('leaving the page closes the window', async t => {
  const { page } = await openPage(t, app, '/', { role: 'operator', viewport: { width: 1440, height: 900 } })
  await page.locator('.navbar').getByRole('link', { name: 'Targets', exact: true }).click()
  await page.getByRole('button', { name: '+ Target', exact: true }).click()
  const modal = page.locator('.modal-card')
  await modal.waitFor()
  await openRecipes(modal)
  await page.goBack()
  await page.waitForURL(url => new URL(url).pathname === '/')
  await win(page).waitFor({ state: 'detached' })
})

test('nothing in the window, the banner, the recipe button or a marked field is Bekci orange', async t => {
  const { page, modal } = await openForm(t)
  await openRecipes(modal)
  for (const id of ['S2', 'C2', 'S4']) {
    await pick(page, id)
    assert.deepEqual(await orangeIn(page, '.recipes-window'), [], id)
  }
  await useButton(page).click()
  await banner(modal).waitFor()
  assert.deepEqual(await orangeIn(page, '.recipe-banner'), [])
  assert.deepEqual(await orangeIn(page, '.btn-recipe'), [])
  assert.deepEqual(await orangeIn(page, '.condition-card input.need-field'), [])
})

test('changing the type of a check a recipe put in drops its fields to fill in, so Save is not held up by a field that has gone', async t => {
  const { writes, modal } = await openForm(t)
  await fillTarget(modal)
  await useRecipe(modal, 'B3')
  await condField(modal, 'Check Type').selectOption('dns')
  await condField(modal, 'Expected Value (optional)').waitFor()
  assert.equal(await condField(modal, 'Expected Value (optional)').getAttribute('placeholder'), 'e.g. 1.2.3.4', 'no recipe placeholder on the new type')
  const body = await save(modal, writes, 'POST')
  assert.equal(body.conditions[0].check_type, 'dns')
  assert.ok(!('needs' in body.conditions[0]))
})

test('when the recipes code cannot be fetched, the window says to reload, Close closes it, and the form keeps working', async t => {
  for (const file of ['RecipesWindow.vue', 'HelpLayer.vue']) {
    const { page, writes, modal } = await openForm(t)
    let asked = 0
    await page.route(url => url.pathname === `/src/help/${file}`, route => {
      asked++
      return route.fulfill({ status: 404, body: 'Not Found' })
    })
    await startButton(modal).click()
    const failed = page.locator('.help-bar-error')
    await failed.waitFor()
    assert.equal(asked, 1, `${file}: the code was asked for`)
    assert.equal(await failed.getAttribute('role'), 'alert')
    assert.equal(await failed.locator('.help-bar-text').innerText(), 'Help could not load: reload the page.', file)
    assert.equal(await page.evaluate(() => {
      const el = document.elementFromPoint(innerWidth / 2, 20)
      return !!el?.closest('.help-bar-error')
    }), true, `${file}: the message shows above the form`)
    await failed.getByRole('button', { name: 'Close', exact: true }).click()
    await failed.waitFor({ state: 'detached' })
    // The form still works: a condition by hand, and Save
    await fillTarget(modal)
    await modal.getByRole('button', { name: '+ Add Group or Condition', exact: true }).click()
    await condField(modal, 'Check Name').fill('Ping')
    const body = await save(modal, writes, 'POST')
    assert.equal(body.conditions.length, 1, file)
  }
})
