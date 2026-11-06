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

const target = (id, name, tags, project, location) => ({
  id, name, host: `${id}.example.invalid`, category: 'Other', project, location,
  enabled: true, tags, recipient_ids: [], condition_count: 0,
})
const TARGETS = [
  target('a', 'Web A', ['P1'], 'ALPHA', 'IZMIR'),
  target('b', 'Web B', ['P1'], 'ALPHA', 'ISTANBUL'),
  target('c', 'Web C', ['P1'], 'BETA', 'IZMIR'),
  target('d', 'Web D', ['IT'], 'ALPHA', 'IZMIR'),
  target('e', 'Web E', ['P1', 'IT'], 'ALPHA', 'IZMIR'),
]

test('the Targets tag filter narrows with Location and Project, and its chips have no pin', async t => {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } })
  const browserErrors = []
  page.on('pageerror', error => browserErrors.push(error.message))
  t.after(async () => {
    await page.close()
    assert.deepEqual(browserErrors, [], 'browser JavaScript errors')
  })
  page.setDefaultTimeout(3000)
  await page.route(`${baseURL}api/**`, async route => {
    const url = new URL(route.request().url())
    const group = url.searchParams.get('group')
    let json
    if (url.pathname === '/api/me') json = { id: 'operator', username: 'tester', role: 'operator' }
    else if (url.pathname === '/api/tags') json = {
      category: [{ id: 'other', value: 'Other' }],
      project: [{ id: 'alpha', value: 'ALPHA' }, { id: 'beta', value: 'BETA' }],
      location: [{ id: 'izmir', value: 'IZMIR' }, { id: 'istanbul', value: 'ISTANBUL' }],
      tag: [{ id: 'p1', value: 'P1' }, { id: 'it', value: 'IT' }],
    }[group] || []
    else if (url.pathname === '/api/system/health') json = {
      net: { status: 'ok', latency_ms: 1 }, disk: { total_gb: 100, free_gb: 80 },
      cpu: { load1: 0, num_cpu: 4 }, scheduler: { status: 'ok', active_checks: 0, stale_seconds: 0 },
    }
    else if (url.pathname === '/api/targets') json = TARGETS
    else return route.fulfill({ status: 404, json: { error: 'Unexpected fixture request' } })
    await route.fulfill({ json })
  })
  await page.goto(`${baseURL}targets`)

  // The names in the table, once they settle on the expected list
  const shown = async expected => {
    const names = () => page.$$eval('.card table tbody tr', rows =>
      rows.map(r => r.cells[0].textContent.replace(/[▶▼]/g, '').trim()))
    await page.waitForFunction(want => JSON.stringify([...document.querySelectorAll('.card table tbody tr')]
      .map(r => r.cells[0].textContent.replace(/[▶▼]/g, '').trim())) === want, JSON.stringify(expected))
      .catch(() => {})
    assert.deepEqual(await names(), expected)
  }
  await shown(['Web A', 'Web B', 'Web C', 'Web D', 'Web E'])

  const filters = page.locator('.filter-bar-secondary')
  const tagInput = filters.locator('.tag-chip-input input.chip-text')
  const addTag = async tag => { await tagInput.fill(tag); await tagInput.press('Enter') }

  await addTag('P1')
  await shown(['Web A', 'Web B', 'Web C', 'Web E'])
  // A chip only filters: its one button removes it, there is no pin
  const chip = filters.locator('.chip').filter({ hasText: 'P1' })
  assert.deepEqual(await chip.locator('button').evaluateAll(bs => bs.map(b => b.title)), ['Remove'])

  await filters.locator('select').nth(1).selectOption('IZMIR')
  await shown(['Web A', 'Web C', 'Web E'])
  await filters.locator('select').nth(0).selectOption('ALPHA')
  await shown(['Web A', 'Web E'])
  await addTag('IT')
  await shown(['Web E'])
  assert.equal(await filters.locator('.chip button').count(), 2, 'one Remove per chip')

  // Removing a chip widens the list again
  await filters.locator('.chip').filter({ hasText: 'IT' }).getByTitle('Remove').click()
  await shown(['Web A', 'Web E'])
})
