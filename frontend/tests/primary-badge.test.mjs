import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright-core'
import { createServer } from 'vite'
import vue from '@vitejs/plugin-vue'

let server, browser, baseURL
before(async () => {
  // No backend proxy: every API request is fulfilled by the browser fixture.
  server = await createServer({
    configFile: false, root: fileURLToPath(new URL('../', import.meta.url)),
    plugins: [vue()], logLevel: 'error', server: { host: '127.0.0.1', port: 0 },
  })
  await server.listen()
  baseURL = server.resolvedUrls.local[0]
  browser = await chromium.launch(process.env.CHROME_PATH
    ? { executablePath: process.env.CHROME_PATH }
    : { channel: 'chrome' })
})
after(async () => {
  await browser?.close()
  await server?.close()
})

const health = {
  net: { status: 'ok', latency_ms: 1 }, disk: { total_gb: 100, free_gb: 80 },
  cpu: { load1: 0, num_cpu: 4 }, scheduler: { status: 'ok', active_checks: 0, stale_seconds: 0 },
}

// One target with two HTTP checks and a ping check, HTTP preferred. The server
// names a-slow (first by name) as the primary check: only it may be badged,
// and the Dashboard row and SOC wall show its uptime, whatever order the checks
// arrive in. The Dashboard/SOC fixtures list z-page first on purpose; Search
// lists checks in condition order, where z-page also comes first.
const PRIMARY = 'c-slow'
const target = {
  id: 't1', name: 'Shop', host: 'shop.example.com', description: '', enabled: true,
  preferred_check_type: 'http', primary_check_id: PRIMARY, operator: 'AND',
  category: 'Other', tags: [],
}
const check = (id, name, type, uptime) => ({
  id, name, type, enabled: true, interval_s: 300,
  last_status: 'up', last_message: '', response_ms: 10, uptime_90d_pct: uptime,
})
const dashboardTarget = {
  ...target, state: 'healthy', paused: false, sla_status: '', sla_target: 0,
  checks: [check('c-page', 'z-page', 'http', 12.5), check(PRIMARY, 'a-slow', 'http', 100), check('c-ping', 'm-ping', 'ping', 50)],
}
const condition = (check_id, check_name, check_type) => ({
  check_id, check_name, check_type, config: '{}', interval_s: 300, field: 'status',
  comparator: 'eq', value: 'down', fail_count: 1, fail_window: 0, condition_group: 0, group_operator: 'AND',
})
const fixtures = {
  '/api/targets': [{ ...target, condition_count: 3, state: { current_state: 'healthy' } }],
  '/api/targets/t1/checks': [
    { id: PRIMARY, target_id: 't1', type: 'http', name: 'a-slow', config: '{}', interval_s: 300, enabled: true },
    { id: 'c-ping', target_id: 't1', type: 'ping', name: 'm-ping', config: '{}', interval_s: 300, enabled: true },
    { id: 'c-page', target_id: 't1', type: 'http', name: 'z-page', config: '{}', interval_s: 300, enabled: true },
  ],
  '/api/targets/t1': {
    ...target, state: null, recipient_ids: [],
    conditions: [condition('c-page', 'z-page', 'http'), condition(PRIMARY, 'a-slow', 'http'), condition('c-ping', 'm-ping', 'ping')],
  },
  '/api/dashboard/status': [dashboardTarget],
  '/api/soc/status': [dashboardTarget],
}

// afterSave: fixtures served once the target has been saved (PUT /api/targets/t1).
async function newPage(t, afterSave = {}) {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1100 } })
  const browserErrors = []
  page.on('pageerror', error => browserErrors.push(error.message))
  t.after(async () => {
    await page.close()
    assert.deepEqual(browserErrors, [], 'browser JavaScript errors')
  })
  page.setDefaultTimeout(3000)
  let saved = false
  await page.route(`${baseURL}api/**`, async route => {
    const request = route.request()
    const url = new URL(request.url())
    if (request.method() !== 'GET') {
      if (url.pathname === '/api/targets/t1') saved = true
      return route.fulfill({ json: {} })
    }
    let json
    if (saved && url.pathname in afterSave) json = afterSave[url.pathname]
    else if (url.pathname === '/api/me') json = { id: 'admin', username: 'admin', role: 'admin' }
    else if (url.pathname === '/api/system/health') json = health
    else if (url.pathname === '/api/tags') json = url.searchParams.get('group') === 'category' ? [{ id: 1, value: 'Other' }] : []
    else if (url.pathname in fixtures) json = fixtures[url.pathname]
    else json = []
    await route.fulfill({ json })
  })
  return page
}

async function assertOnlyPrimaryBadged(scope, primary = 'a-slow') {
  const badges = scope.locator('.badge-primary-check')
  await badges.first().waitFor()
  assert.equal(await badges.count(), 1, 'one PRIMARY badge')
  // The badge's own row (Targets nests the checks table inside a target row).
  const row = badges.locator('xpath=ancestor::*[self::tr or contains(@class, "check-row")][1]')
  const text = await row.innerText()
  assert.match(text, new RegExp(primary), 'PRIMARY is on the server\'s primary check')
  for (const other of ['a-new', 'a-slow', 'z-page', 'm-ping'].filter(n => n !== primary)) {
    assert.doesNotMatch(text, new RegExp(other), 'the badge row holds one check')
  }
}

test('Targets: PRIMARY marks only the primary check', async t => {
  const page = await newPage(t)
  await page.goto(`${baseURL}targets`)
  await page.locator('tbody tr').filter({ hasText: 'shop.example.com' }).first().click()
  await assertOnlyPrimaryBadged(page.locator('table'))
})

// The open row keeps its check list while the target list (which carries
// primary_check_id) reloads after a save. Adding a check that becomes the new
// primary must not leave the row with no badge: both reload together.
test('Targets: after a save the open row badges the new primary check', async t => {
  const page = await newPage(t, {
    '/api/targets': [{ ...fixtures['/api/targets'][0], primary_check_id: 'c-new' }],
    '/api/targets/t1/checks': [
      { id: 'c-new', target_id: 't1', type: 'http', name: 'a-new', config: '{}', interval_s: 300, enabled: true },
      ...fixtures['/api/targets/t1/checks'],
    ],
  })
  await page.goto(`${baseURL}targets`)
  const row = page.locator('tbody tr').filter({ hasText: 'shop.example.com' }).first()
  await row.click()
  await assertOnlyPrimaryBadged(page.locator('table'))
  await row.getByRole('button', { name: 'Edit', exact: true }).click()
  await page.locator('.modal-card').getByRole('button', { name: 'Save', exact: true }).click()
  await page.getByText('Target updated').waitFor()
  await assertOnlyPrimaryBadged(page.locator('table'), 'a-new')
})

test('Dashboard: PRIMARY marks only the primary check; the row shows its uptime', async t => {
  const page = await newPage(t)
  await page.goto(`${baseURL}`)
  const header = page.locator('.target-header').filter({ hasText: 'Shop' })
  assert.equal((await header.locator('.check-uptime').innerText()).trim(), '100.0%')
  await header.click()
  await assertOnlyPrimaryBadged(page.locator('.expanded-checks'))
})

test('Search: PRIMARY marks only the primary check in condition order', async t => {
  const page = await newPage(t)
  await page.goto(`${baseURL}search`)
  await page.getByPlaceholder('Search by name, host, or IP...').fill('shop')
  await page.locator('tbody tr').filter({ hasText: 'shop.example.com' }).first().click()
  await assertOnlyPrimaryBadged(page.locator('table'))
})

test('SOC: the wall shows the primary check\'s uptime', async t => {
  const page = await newPage(t)
  await page.goto(`${baseURL}soc`)
  assert.equal((await page.locator('.soc-uptime').first().innerText()).trim(), '100.0%')
})
