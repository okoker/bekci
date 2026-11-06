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

const tags = {
  tag: [{ id: 11, group_name: 'tag', value: 'P1' }],
  project: [{ id: 21, group_name: 'project', value: 'APOLLO' }],
  location: [{ id: 31, group_name: 'location', value: 'IST' }],
  category: [
    { id: 41, group_name: 'category', value: 'Network', sla_key: 'sla_network' },
    { id: 1, group_name: 'category', value: 'Other', sla_key: 'sla_other' },
  ],
}

async function newPage(t) {
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
    let json = []
    if (url.pathname === '/api/me') json = { id: 'admin', username: 'admin', role: 'admin' }
    else if (url.pathname === '/api/system/health') json = health
    else if (url.pathname === '/api/settings') json = {}
    else if (url.pathname === '/api/tags') json = tags[url.searchParams.get('group')] || []
    await route.fulfill({ json })
  })
  return page
}

// One function deletes free tags, projects, locations and categories, and it
// always asked "Are you sure you want to delete this category?".
test('every tag list asks "Really delete this?" before deleting', async t => {
  const page = await newPage(t)
  const deletes = []
  page.on('request', r => { if (r.method() === 'DELETE') deletes.push(new URL(r.url()).pathname) })
  await page.goto(`${baseURL}settings`)
  await page.getByRole('button', { name: 'Tags', exact: true }).click()

  const sections = [
    ['Free Tags', 'P1'], ['Project Tags', 'APOLLO'], ['Location Tags', 'IST'], ['Categories', 'Network'],
  ]
  for (const [title, value] of sections) {
    const card = page.locator('.collapsible-card').filter({ has: page.getByRole('heading', { name: title, exact: true }) })
    if (!(await card.evaluate(el => el.classList.contains('expanded')))) await card.locator('.collapsible-header').click()
    const item = card.locator('.tag-item').filter({ hasText: value })
    let message
    page.once('dialog', d => { message = d.message(); d.dismiss() }) // the click waits until it is answered
    await item.getByRole('button', { name: 'Delete', exact: true }).click()
    assert.equal(message, 'Really delete this?', `${title}: confirm text`)
  }

  // Accepting deletes the tag; the dismissed ones above sent nothing.
  const free = page.locator('.collapsible-card').filter({ has: page.getByRole('heading', { name: 'Free Tags', exact: true }) })
  page.once('dialog', d => d.accept())
  const del = page.waitForRequest(r => r.method() === 'DELETE')
  await free.getByRole('button', { name: 'Delete', exact: true }).click()
  await del
  assert.deepEqual(deletes, ['/api/tags/11'])
})
