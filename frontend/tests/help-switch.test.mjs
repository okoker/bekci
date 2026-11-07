import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { HELP_VERSION } from '../src/help/content/version.js'
import {
  helpButton, openPage, orangeIn, startApp, stopApp, turnHelpOn, waitForStyle,
} from './help-fixtures.mjs'

let app
before(async () => { app = await startApp() })
after(() => stopApp(app))

const navItems = page => page.locator('.navbar-left .nav-link').evaluateAll(els => els.map(e => e.textContent.trim()))
const navLink = (page, name) => page.locator('.navbar').getByRole('link', { name, exact: true })
const near = (a, b) => Math.abs(a - b) < 1

test('Help is the last item in the header, after the last link the role sees', async t => {
  const expected = {
    admin: ['Dashboard', 'Search', 'SLA', 'Targets', 'SOC', 'Alerts', 'Audit Log', 'Users', 'Settings', 'Help'],
    operator: ['Dashboard', 'Search', 'SLA', 'Targets', 'SOC', 'Alerts', 'Audit Log', 'Help'],
    viewer: ['Dashboard', 'SLA', 'Targets', 'SOC', 'Alerts', 'Help'],
  }
  for (const [role, items] of Object.entries(expected)) {
    const { page } = await openPage(t, app, '/', { role })
    await page.locator('.navbar').waitFor()
    assert.deepEqual(await navItems(page), items, role)
  }
})

test('Help turns on a slate bar under the header with the page summary; the Help item turns white and turquoise', async t => {
  const { page } = await openPage(t, app, '/')
  await page.locator('[data-help-page="dashboard"]').waitFor()
  const help = helpButton(page)
  assert.equal(await help.getAttribute('aria-pressed'), 'false')
  await help.click()
  const bar = page.locator('.help-bar')
  await bar.waitFor()
  assert.match(await bar.innerText(), /^Dashboard shows every switched-on target's state right now/)
  assert.equal(await help.getAttribute('aria-pressed'), 'true')
  await waitForStyle(help, 'backgroundColor', 'rgb(255, 255, 255)')
  await waitForStyle(help, 'color', 'rgb(15, 118, 110)')
  await waitForStyle(bar, 'backgroundColor', 'rgb(30, 41, 59)')
  await waitForStyle(bar, 'color', 'rgb(203, 213, 225)')
  // In the page flow, directly under the header: the page moves down, nothing is covered
  const nav = await page.locator('.navbar').boundingBox()
  const box = await bar.boundingBox()
  const main = await page.locator('main.content').boundingBox()
  assert.ok(near(box.y, nav.y + nav.height), `bar top ${box.y}, header bottom ${nav.y + nav.height}`)
  assert.ok(near(main.y, box.y + box.height), `page top ${main.y}, bar bottom ${box.y + box.height}`)
  assert.ok(near(box.width, nav.width), 'full width')
})

test('the bar says which Bekci version the help was written for, and the running one when it differs', async t => {
  const cases = [
    [HELP_VERSION, `Written for Bekci ${HELP_VERSION}`],
    ['9.9.9', `Written for Bekci ${HELP_VERSION} (running 9.9.9)`],
  ]
  for (const [running, stamp] of cases) {
    const { page } = await openPage(t, app, '/', { version: running })
    await page.locator('.app-version', { hasText: `v${running}` }).waitFor()
    await turnHelpOn(page)
    assert.equal((await page.locator('.help-bar-stamp').innerText()).trim(), stamp)
  }
})

test('Done, Esc, Help again and going to another page each turn help off', async t => {
  const ways = {
    Done: page => page.locator('.help-bar').getByRole('button', { name: 'Done', exact: true }).click(),
    Esc: page => page.keyboard.press('Escape'),
    'Help again': page => helpButton(page).click(),
    'another page': async page => {
      await navLink(page, 'Targets').click()
      await page.locator('[data-help-page="targets"]').waitFor()
    },
  }
  for (const [way, turnOff] of Object.entries(ways)) {
    const { page } = await openPage(t, app, '/')
    await turnHelpOn(page)
    await turnOff(page)
    await page.locator('.help-bar').waitFor({ state: 'detached' })
    assert.equal(await helpButton(page).getAttribute('aria-pressed'), 'false', way)
  }
})

// Help isn't modal, so a page's own windows are used while it is on. Esc closes
// one thing per press: the page's window first, then help.
test('Esc closes a page window opened while help is on, and only the next Esc turns help off', async t => {
  const windows = {
    'SLA zoom': { path: '/sla', id: 'sla', open: page => page.locator('.sla-card').filter({ hasText: 'Network' }).click(),
      window: '.modal-backdrop' },
    'audit entry': { path: '/audit-log', id: 'auditLog', open: page => page.locator('.audit-row').first().click(),
      window: '.modal-overlay' },
  }
  for (const [name, w] of Object.entries(windows)) {
    const { page } = await openPage(t, app, w.path)
    await page.locator(`[data-help-page="${w.id}"]`).waitFor()
    await turnHelpOn(page)
    await w.open(page)
    const opened = page.locator(w.window)
    await opened.waitFor()
    await page.keyboard.press('Escape')
    await opened.waitFor({ state: 'detached' })
    assert.equal(await page.locator('.help-bar').count(), 1, `${name}: help still on after the window closed`)
    assert.equal(await helpButton(page).getAttribute('aria-pressed'), 'true', name)
    await page.keyboard.press('Escape')
    await page.locator('.help-bar').waitFor({ state: 'detached' })
    assert.equal(await helpButton(page).getAttribute('aria-pressed'), 'false', name)
  }
})

test('Esc typed in a text box belongs to the box, not to help', async t => {
  const { page } = await openPage(t, app, '/search', { role: 'operator' })
  await page.locator('[data-help-page="search"]').waitFor()
  await turnHelpOn(page)
  // A search makes "+ Save" appear; its name box closes on Esc
  await page.locator('.search-input').fill('shop')
  await page.keyboard.press('Escape')
  assert.equal(await page.locator('.help-bar').count(), 1, 'help still on after Esc in the search box')
  await page.getByRole('button', { name: '+ Save', exact: true }).click()
  const nameBox = page.locator('.save-popover-input')
  await nameBox.click()
  await page.keyboard.press('Escape')
  await nameBox.waitFor({ state: 'detached' })
  assert.equal(await page.locator('.help-bar').count(), 1, 'help still on after Esc in the name box')
  assert.equal(await helpButton(page).getAttribute('aria-pressed'), 'true')
})

test('the SOC wall and the login page have no Help item', async t => {
  const soc = (await openPage(t, app, '/soc')).page
  await soc.locator('.soc-page').waitFor()
  assert.equal(await soc.getByRole('button', { name: 'Help', exact: true }).count(), 0, 'SOC')
  const login = (await openPage(t, app, '/login', { role: null })).page
  await login.locator('.login-page').waitFor()
  assert.equal(await login.getByRole('button', { name: 'Help', exact: true }).count(), 0, 'Login')
})

test('leaving for the SOC wall turns help off, so it is off on the way back', async t => {
  const { page } = await openPage(t, app, '/')
  await turnHelpOn(page)
  await navLink(page, 'SOC').click()
  await page.locator('.soc-page').waitFor()
  assert.equal(await page.locator('.help-bar').count(), 0, 'no bar on the SOC wall')
  await page.goBack()
  await page.locator('[data-help-page="dashboard"]').waitFor()
  assert.equal(await page.locator('.help-bar').count(), 0, 'no bar back on the Dashboard')
  assert.equal(await helpButton(page).getAttribute('aria-pressed'), 'false')
})

test('nothing in the bar or the Help item is Bekci orange', async t => {
  const { page } = await openPage(t, app, '/')
  await page.locator('[data-help-page="dashboard"]').waitFor()
  assert.deepEqual(await orangeIn(page, '.nav-help'), [], 'Help item, help off')
  await turnHelpOn(page)
  await waitForStyle(helpButton(page), 'backgroundColor', 'rgb(255, 255, 255)')
  assert.deepEqual(await orangeIn(page, '.nav-help, .help-bar'), [], 'Help item and bar, help on')
})

// Review Focus 1: an old tab after a deploy asks for a help chunk the server no
// longer has. In the Vite dev server the bar's lazy chunk is the module
// /src/help/HelpBar.vue. A built Bekci answers a missing file with index.html
// (200, text/html), which a browser refuses as a module, so that is tried too.
test('when the help code cannot be fetched, Help says to reload and the page keeps working', async t => {
  const answers = {
    404: { status: 404, body: 'Not Found' },
    'index.html': { status: 200, contentType: 'text/html', body: '<!doctype html><title>Bekci</title>' },
  }
  for (const [name, answer] of Object.entries(answers)) {
    const { page } = await openPage(t, app, '/')
    await page.locator('[data-help-page="dashboard"]').waitFor()
    let asked = 0
    await page.route(url => url.pathname === '/src/help/HelpBar.vue', route => {
      asked++
      return route.fulfill(answer)
    })
    await helpButton(page).click()
    const failed = page.locator('.help-bar-error')
    await failed.waitFor()
    assert.equal(asked, 1, `${name}: the chunk was asked for`)
    assert.equal(await failed.getAttribute('role'), 'alert')
    assert.equal((await failed.locator('.help-bar-text').innerText()).trim(), 'Help could not load: reload the page.', name)
    // The page keeps working: its rows are there, Done closes the message, the header still navigates
    await page.getByText('Web shop').first().waitFor()
    await failed.getByRole('button', { name: 'Done', exact: true }).click()
    await failed.waitFor({ state: 'detached' })
    assert.equal(await helpButton(page).getAttribute('aria-pressed'), 'false', name)
    await navLink(page, 'Targets').click()
    await page.locator('[data-help-page="targets"]').getByText('Core switch').waitFor()
  }
})

test('each page names its help page, and the bar follows a Settings tab or a Users sub-tab', async t => {
  const { page } = await openPage(t, app, '/')
  const onPage = async id => {
    await page.locator(`[data-help-page="${id}"]`).waitFor()
    assert.equal(await page.locator('[data-help-page]').count(), 1, `one page id on ${id}`)
  }
  await onPage('dashboard')
  for (const [link, id] of [['Targets', 'targets'], ['Search', 'search'], ['SLA', 'sla'], ['Alerts', 'alerts'],
    ['Audit Log', 'auditLog'], ['Users', 'users'], ['Settings', 'settingsGeneral'], ['Dashboard', 'dashboard']]) {
    await navLink(page, link).click()
    await onPage(id)
  }
  await page.locator('.user-menu-trigger').click()
  await page.locator('.user-dropdown').getByRole('link', { name: 'Profile', exact: true }).click()
  await onPage('profile')

  // While help is on, the bar follows the tab on show
  const barFor = id => page.locator(`.help-bar[data-for-page="${id}"]`).waitFor()
  await navLink(page, 'Settings').click()
  await onPage('settingsGeneral')
  await turnHelpOn(page)
  await barFor('settingsGeneral')
  for (const [tab, id] of [['Tags', 'settingsTags'], ['SLA', 'settingsSla'], ['Alerting', 'settingsAlerting'],
    ['Backup & Restore', 'settingsBackup'], ['Fail2Ban', 'settingsFail2ban'], ['System Health', 'settingsHealth'],
    ['General', 'settingsGeneral']]) {
    await page.locator('.tabs').getByRole('button', { name: tab, exact: true }).click()
    await onPage(id)
    await barFor(id)
  }
  await navLink(page, 'Users').click()
  await onPage('users')
  await turnHelpOn(page)
  await barFor('users')
  await page.getByRole('button', { name: 'API Access', exact: true }).click()
  await onPage('apiAccess')
  await barFor('apiAccess')
})
