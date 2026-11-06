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

// routes: { 'PUT /api/me/password': [status, json] } overrides the defaults.
async function newPage(t, routes = {}) {
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
    const { pathname } = new URL(request.url())
    const fixture = routes[`${request.method()} ${pathname}`]
    if (fixture) return route.fulfill({ status: fixture[0], json: fixture[1] })
    if (request.method() !== 'GET') return route.fulfill({ json: {} })
    let json = []
    if (pathname === '/api/me') json = { id: 'admin', username: 'admin', role: 'admin' }
    else if (pathname === '/api/system/health') json = health
    else if (pathname === '/api/settings') json = {}
    await route.fulfill({ json })
  })
  return page
}

async function fillPasswordForm(page) {
  const form = page.locator('form').filter({ hasText: 'Current Password' })
  const inputs = form.locator('input[type="password"]')
  await inputs.nth(0).fill('not-my-password-123')
  await inputs.nth(1).fill('a-brand-new-password-1')
  await inputs.nth(2).fill('a-brand-new-password-1')
  return form
}

// The server answers a wrong current password with 401. The browser's 401
// handler took every 401 as an expired session and went to the login page, so
// the person saw no error (and was not really signed out on the server).
test('a wrong current password shows the error and stays on Profile', async t => {
  const page = await newPage(t, {
    'PUT /api/me/password': [401, { error: 'current password is incorrect' }],
  })
  await page.goto(`${baseURL}profile`)
  const form = await fillPasswordForm(page)
  await form.getByRole('button', { name: 'Change Password' }).click()

  await page.waitForFunction(() => location.pathname === '/login' || document.querySelector('.error-msg'))
  assert.equal(new URL(page.url()).pathname, '/profile', 'stays on Profile')
  assert.equal(await form.locator('.error-msg').textContent(), 'current password is incorrect')
  await page.locator('.navbar-user', { hasText: 'admin' }).waitFor() // still signed in
})

// The exemption is for the password change only: any other 401 still means the
// session is gone and goes to the login page.
test('a 401 from another call still goes to the login page', async t => {
  const page = await newPage(t, {
    'GET /api/audit-log': [401, { error: 'not authenticated' }],
  })
  await page.goto(`${baseURL}audit-log`)
  await page.waitForURL(`${baseURL}login`)
})

// Audit Log badges: actionClass tested "login" before "failed", so every
// login_failed entry had the blue sign-in badge instead of the red failure one.
test('Audit Log shows login_failed with the same badge colour as other failures', async t => {
  const entry = (id, action, status) => ({
    id, created_at: '2026-11-06T09:00:00Z', username: 'admin', action, resource_type: 'user',
    resource_id: '', detail: '', status, ip_address: '192.0.2.10',
  })
  const page = await newPage(t, {
    'GET /api/audit-log': [200, {
      total: 3,
      entries: [entry(1, 'login_failed', 'failure'), entry(2, 'change_password_failed', 'failure'), entry(3, 'login', 'success')],
    }],
  })
  await page.goto(`${baseURL}audit-log`)
  const colour = action => page.locator('.audit-row .badge').filter({ hasText: new RegExp(`^${action}$`) })
    .evaluate(el => getComputedStyle(el).backgroundColor)

  const failedLogin = await colour('login_failed')
  assert.equal(failedLogin, await colour('change_password_failed'), 'same colour as the other failure')
  assert.notEqual(failedLogin, await colour('login'), 'not the sign-in colour')
})
