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

const day = n => new Date(Date.now() - n * 86400000).toISOString().slice(0, 10)

// Opens the SLA page with one category; returns a reader for its insight cards.
async function slaPage(t, category) {
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
    else if (pathname === '/api/sla/history') json = { categories: [category], pause_stats: { count: 0, affected_hosts: 0 } }
    await route.fulfill({ json })
  })
  await page.goto(`${baseURL}sla`)
  await page.locator('.insights-card').waitFor()
  const card = label => page.locator('.icard').filter({ has: page.locator('.icard-label', { hasText: new RegExp(`^${label}$`) }) })
    .locator('.icard-value').textContent()
  return { card }
}

// Backlog A-069: the page averaged each host's daily percentages, so a day of
// 2 runs (1 down, 50%) counted as much as a day of 8 up runs (100%): 75%. The
// server's figure pools the runs, as the Dashboard does: 9 of 10 up, 90%.
test('a host under its goal by daily average but not by runs meets the SLA', async t => {
  const { card } = await slaPage(t, {
    name: 'Network', sla_threshold: 80, targets: [
      { id: 'u', name: 'uneven', uptime_90d_pct: 90, daily_uptime: [{ date: day(2), uptime_pct: 50 }, { date: day(1), uptime_pct: 100 }] },
      { id: 's', name: 'steady', uptime_90d_pct: 100, daily_uptime: [{ date: day(1), uptime_pct: 100 }] },
      { id: 'f', name: 'fresh', uptime_90d_pct: -1, daily_uptime: [] },
    ],
  })
  assert.equal(await card('Hosts Monitored'), '3')
  assert.equal(await card('Not Meeting SLA'), '0', 'uneven is at 90%, over the 80% goal')
  // One vote per host; the host without data is left out
  assert.equal(await card('Network'), '95.0%')
  assert.equal(await card('All'), '95.0%')
})

// The other way round: many down runs on one day, a few up runs on another.
test('a host over its goal by daily average but not by runs does not meet the SLA', async t => {
  const { card } = await slaPage(t, {
    name: 'Servers', sla_threshold: 99.4, targets: [
      { id: 'b', name: 'busy-day', uptime_90d_pct: 99.1, daily_uptime: [{ date: day(2), uptime_pct: 99 }, { date: day(1), uptime_pct: 100 }] },
    ],
  })
  assert.equal(await card('Not Meeting SLA'), '1', 'busy-day is at 99.1%, under the 99.4% goal')
  assert.equal(await card('Servers'), '99.1%')
})
