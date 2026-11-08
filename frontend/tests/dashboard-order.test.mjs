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

// A Dashboard row as /api/dashboard/status sends it: one check with the given uptime.
const row = (name, { state, paused = false, sla = '', uptime }) => ({
  id: name, name, host: `${name}.example.com`, description: '', enabled: true,
  preferred_check_type: 'http', primary_check_id: `c-${name}`, operator: 'AND',
  category: 'Other', tags: [], state, paused, sla_status: sla, sla_target: 99,
  checks: [{
    id: `c-${name}`, name: 'page', type: 'http', enabled: true, interval_s: 300,
    last_status: state === 'unhealthy' ? 'down' : 'up', last_message: '', response_ms: 10, uptime_90d_pct: uptime,
  }],
})

async function dashboardNames(t, rows) {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1100 } })
  const browserErrors = []
  page.on('pageerror', error => browserErrors.push(error.message))
  t.after(async () => {
    await page.close()
    assert.deepEqual(browserErrors, [], 'browser JavaScript errors')
  })
  page.setDefaultTimeout(3000)
  await page.route(`${baseURL}api/**`, async route => {
    const { pathname } = new URL(route.request().url())
    let json = []
    if (pathname === '/api/me') json = { id: 'admin', username: 'admin', role: 'admin' }
    else if (pathname === '/api/system/health') json = health
    else if (pathname === '/api/tags') json = [{ id: 1, value: 'Other' }]
    else if (pathname === '/api/dashboard/status') json = rows
    await route.fulfill({ json })
  })
  await page.goto(baseURL)
  await page.locator('.target-name').first().waitFor()
  return page.locator('.target-name').allInnerTexts()
}

// Backlog A-075: a paused target under its SLA goal used to sort above every
// healthy target, because "under its SLA goal" was tested before "paused".
// Paused targets are always last; among themselves the lowest uptime is first.
test('Dashboard: paused targets sort after healthy and down ones, even under their SLA goal', async t => {
  const names = await dashboardNames(t, [
    row('paused-low-sla', { state: 'paused', paused: true, sla: 'unhealthy', uptime: 40 }),
    row('healthy-b', { state: 'healthy', uptime: 100 }),
    row('down', { state: 'unhealthy', sla: 'unhealthy', uptime: 20 }),
    row('healthy-a', { state: 'healthy', uptime: 99.9 }),
    row('healthy-low-sla', { state: 'healthy', sla: 'unhealthy', uptime: 70 }),
  ])
  assert.deepEqual(names, ['down', 'healthy-low-sla', 'healthy-a', 'healthy-b', 'paused-low-sla'])
})

test('Dashboard: paused targets keep the lowest uptime first, then the name', async t => {
  const names = await dashboardNames(t, [
    row('paused-b-sla', { state: 'paused', paused: true, sla: 'unhealthy', uptime: 90 }),
    row('paused-z', { state: 'paused', paused: true, uptime: 60 }),
    row('paused-a', { state: 'paused', paused: true, uptime: 60 }),
    row('healthy', { state: 'healthy', uptime: 100 }),
  ])
  assert.deepEqual(names, ['healthy', 'paused-a', 'paused-z', 'paused-b-sla'])
})
