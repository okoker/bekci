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

async function newPage(t, fixtures) {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1100 } })
  const browserErrors = []
  page.on('pageerror', error => browserErrors.push(error.message))
  t.after(async () => {
    await page.close()
    assert.deepEqual(browserErrors, [], 'browser JavaScript errors')
  })
  page.setDefaultTimeout(3000)
  const writes = []
  await page.route(`${baseURL}api/**`, async route => {
    const request = route.request()
    const url = new URL(request.url())
    if (request.method() !== 'GET') {
      writes.push({ method: request.method(), path: url.pathname, body: request.postDataJSON() })
      return route.fulfill({ json: {} })
    }
    let json
    if (url.pathname === '/api/me') json = { id: 'admin', username: 'admin', role: 'admin' }
    else if (url.pathname === '/api/system/health') json = health
    else if (url.pathname === '/api/tags') json = fixtures.tags[url.searchParams.get('group')] || []
    else if (url.pathname in fixtures) json = fixtures[url.pathname]
    else json = []
    await route.fulfill({ json })
  })
  return { page, writes }
}

// Free tags are matched without caring which of i, ı, İ, I was typed:
// "şişli" finds the stored ŞİŞLİ, "it" finds IT.
test('the tag input finds Turkish and English tags typed in lowercase', async t => {
  const { page } = await newPage(t, {
    tags: { category: [{ id: 1, value: 'Other' }], tag: [{ id: 2, value: 'IT' }, { id: 3, value: 'ŞİŞLİ' }] },
  })
  await page.goto(`${baseURL}targets`)
  await page.getByRole('button', { name: '+ Target', exact: true }).click()
  const modal = page.locator('.modal-card')
  // Not by placeholder: it is cleared once a chip is added.
  const tagInput = modal.locator('.tag-chip-input input.chip-text')

  await tagInput.fill('şişli')
  assert.deepEqual(await modal.locator('.chip-suggest-item').allTextContents(), ['ŞİŞLİ'])
  await tagInput.press('Enter')
  await tagInput.fill('it')
  await tagInput.press('Enter')
  assert.deepEqual(await modal.locator('.chip-label').allTextContents(), ['ŞİŞLİ', 'IT'])
  assert.equal(await modal.locator('.chip-hint').count(), 0)
})

// The Settings save sends every category's SLA key. The key comes from the
// server (sla_key): the browser lowercases "İ" to "i" + U+0307, so deriving it
// here produced "sla_i̇sg", which the server rejected as an unknown setting.
test('Settings save uses the server SLA key for a category with İ', async t => {
  const { page } = await newPage(t, {
    tags: { category: [{ id: 1, value: 'Other', sla_key: 'sla_other' }, { id: 2, value: 'İSG', sla_key: 'sla_isg' }] },
    '/api/settings': { session_timeout_hours: '24' },
  })
  await page.goto(`${baseURL}settings`)
  const put = page.waitForRequest(r => r.method() === 'PUT' && new URL(r.url()).pathname === '/api/settings')
  await page.getByRole('button', { name: 'Save', exact: true }).first().click()
  const slaKeys = Object.keys((await put).postDataJSON()).filter(k => k.startsWith('sla_')).sort()
  assert.deepEqual(slaKeys, ['sla_isg', 'sla_other'])
})
