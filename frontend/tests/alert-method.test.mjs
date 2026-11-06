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

async function newPage(t, settings) {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1100 } })
  const browserErrors = []
  page.on('pageerror', error => browserErrors.push(error.message))
  t.after(async () => {
    await page.close()
    assert.deepEqual(browserErrors, [], 'browser JavaScript errors')
  })
  page.setDefaultTimeout(3000)
  await page.route(`${baseURL}api/**`, async route => {
    const request = route.request()
    const url = new URL(request.url())
    if (request.method() !== 'GET') return route.fulfill({ json: {} })
    let json
    if (url.pathname === '/api/me') json = { id: 'admin', username: 'admin', role: 'admin' }
    else if (url.pathname === '/api/system/health') json = health
    else if (url.pathname === '/api/settings') json = settings
    else if (url.pathname === '/api/settings/webhook-status') json = { last_error: '', last_success: '' }
    else json = []
    await route.fulfill({ json })
  })
  return page
}

// Alert Method "Disabled" is stored as "" (no email or Signal). The form used
// to load it as `|| 'email'`, so after a save or a reload it showed Email, and
// the next Save of that section stored "email" and turned email alerts back on.
test('Alert Method "Disabled" stays Disabled after loading and saving', async t => {
  const page = await newPage(t, { alert_method: '', alert_cooldown_s: '1800', alert_realert_s: '3600' })
  await page.goto(`${baseURL}settings`)
  await page.getByRole('button', { name: 'Alerting', exact: true }).click()
  const card = page.locator('.collapsible-card').filter({ hasText: 'Alert Method & Timing' })
  const method = card.locator('select').first()

  assert.equal(await method.inputValue(), '', 'loaded as Disabled')

  const put = page.waitForRequest(r => r.method() === 'PUT' && new URL(r.url()).pathname === '/api/settings')
  await card.getByRole('button', { name: 'Save', exact: true }).click()
  assert.equal((await put).postDataJSON().alert_method, '', 'saved as Disabled')

  // The section reloads the settings after saving.
  await page.locator('.success-msg', { hasText: 'Alert settings saved' }).waitFor()
  assert.equal(await method.inputValue(), '', 'still Disabled after the save')
})
