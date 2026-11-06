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

// Opens Settings > Backup & Restore with these saved settings; returns the
// Automated Backups card and the bodies of every PUT /api/settings.
async function automatedBackups(t, settings) {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1100 } })
  const browserErrors = []
  page.on('pageerror', error => browserErrors.push(error.message))
  t.after(async () => {
    await page.close()
    assert.deepEqual(browserErrors, [], 'browser JavaScript errors')
  })
  page.setDefaultTimeout(3000)
  const saved = []
  await page.route(`${baseURL}api/**`, async route => {
    const request = route.request()
    const { pathname } = new URL(request.url())
    if (request.method() === 'PUT' && pathname === '/api/settings') {
      saved.push(request.postDataJSON())
      return route.fulfill({ json: { message: 'settings updated' } })
    }
    if (request.method() !== 'GET') return route.fulfill({ json: {} })
    let json = []
    if (pathname === '/api/me') json = { id: 'admin', username: 'admin', role: 'admin' }
    else if (pathname === '/api/system/health') json = health
    else if (pathname === '/api/settings') json = settings
    await route.fulfill({ json })
  })
  await page.goto(`${baseURL}settings`)
  await page.getByRole('button', { name: 'Backup & Restore' }).click()
  const card = page.locator('.card').filter({ has: page.getByRole('heading', { name: 'Automated Backups' }) })
  await card.waitFor()
  return { card, saved }
}

const hours = Array.from({ length: 24 }, (_, h) => `${String(h).padStart(2, '0')}:00`)

// Backlog A-067: the time was a free-text box; 25:00 saved and then never ran,
// and minutes were ignored. It is now a choice of whole hours.
test('the backup time is one of the 24 whole hours, and Save sends the one chosen', async t => {
  const { card, saved } = await automatedBackups(t, { auto_backup_schedule: 'weekly', auto_backup_time: '22:00' })
  const time = card.locator('.form-group', { hasText: 'Time (24h)' })
  assert.equal(await time.locator('select').count(), 1, 'a dropdown, not a text box')
  assert.deepEqual(await time.locator('option').allTextContents(), hours)
  assert.equal(await time.locator('select').inputValue(), '22:00', 'shows the saved hour')
  await card.getByText('starts some time within the chosen hour').waitFor()

  await time.locator('select').selectOption('04:00')
  await card.getByRole('button', { name: 'Save' }).click()
  await card.getByText('Auto backup settings saved').waitFor()
  assert.deepEqual(saved, [{ auto_backup_schedule: 'weekly', auto_backup_time: '04:00' }])
})

// A time saved before (by the text box, or in a restored backup) shows as the
// hour the server runs it in, so the page and the schedule agree.
for (const [stored, shown] of [['04:30', '04:00'], ['25:00', '03:00'], ['', '03:00']]) {
  test(`a saved time of "${stored}" shows as ${shown}`, async t => {
    const { card } = await automatedBackups(t, { auto_backup_schedule: 'weekly', auto_backup_time: stored })
    const select = card.locator('.form-group', { hasText: 'Time (24h)' }).locator('select')
    assert.equal(await select.count(), 1, 'a dropdown, not a text box')
    assert.equal(await select.inputValue(), shown)
  })
}
