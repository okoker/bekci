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

// 30 servers; every fifth one is paused, so Unpause and Pause buttons sit all down the list
const TARGETS = Array.from({ length: 30 }, (_, i) => {
  const n = String(i + 1).padStart(2, '0')
  return {
    id: `t${n}`, name: `Server ${n}`, host: `srv-${n}.example.invalid`, category: 'Other',
    enabled: true, tags: [], recipient_ids: [], condition_count: 1,
    paused_at: (i + 1) % 5 === 0 ? '2026-11-06T10:00:00Z' : null,
    state: { current_state: 'healthy' },
  }
})

// A page whose API calls are answered by the fixture; writes are recorded, not sent
async function fixturePage() {
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
  page.setDefaultTimeout(3000)
  const writes = []
  await page.route(`${baseURL}api/**`, async route => {
    const request = route.request()
    const url = new URL(request.url())
    if (request.method() !== 'GET') {
      writes.push(`${request.method()} ${url.pathname}`)
      return route.fulfill({ json: {} })
    }
    let json = []
    if (url.pathname === '/api/me') json = { id: 'operator', username: 'tester', role: 'operator' }
    else if (url.pathname === '/api/system/health') json = {
      net: { status: 'ok', latency_ms: 1 }, disk: { total_gb: 100, free_gb: 80 },
      cpu: { load1: 0, num_cpu: 4 }, scheduler: { status: 'ok', active_checks: 0, stale_seconds: 0 },
    }
    else if (url.pathname === '/api/tags') json = url.searchParams.get('group') === 'category' ? [{ id: 1, value: 'Other' }] : []
    else if (url.pathname === '/api/targets') json = TARGETS
    await route.fulfill({ json })
  })
  return { page, writes }
}

async function searchPage(t) {
  const { page, writes } = await fixturePage()
  const browserErrors = []
  page.on('pageerror', error => browserErrors.push(error.message))
  t.after(async () => {
    await page.close()
    assert.deepEqual(browserErrors, [], 'browser JavaScript errors')
  })
  await page.goto(`${baseURL}search`)
  await page.getByPlaceholder('Search by name, host, or IP...').fill('server')
  await page.locator('tbody tr').filter({ hasText: 'Server 30' }).waitFor()
  return { page, writes }
}

// Opens a row's dialog with `button`, with the row scrolled to `block` ('center' or 'end'
// of the window), and returns the button's and the dialog card's boxes and card style.
async function openDialog(page, name, button, block) {
  const row = page.locator('tbody tr').filter({ hasText: name })
  await row.evaluate((el, b) => el.scrollIntoView({ block: b }), block)
  const btn = row.getByRole('button', { name: button, exact: true })
  const btnBox = await btn.boundingBox()
  await btn.click()
  const card = page.locator('.modal-overlay .modal-card')
  await card.waitFor()
  const cardBox = await card.boundingBox()
  return { card, btnBox, cardBox, style: await cardStyle(card) }
}

const cardStyle = card => card.evaluate(el => {
  const s = getComputedStyle(el)
  return { background: s.backgroundColor, radius: s.borderRadius, padding: s.padding, shadow: s.boxShadow, width: s.maxWidth }
})

// The look of the same dialog on the Targets page: the site's reference card
let siteCard
async function targetsCard() {
  if (siteCard) return siteCard
  const { page } = await fixturePage()
  await page.goto(`${baseURL}targets`)
  await page.locator('tbody tr').filter({ hasText: 'Server 16' }).getByRole('button', { name: 'Pause', exact: true }).click()
  const card = page.locator('.modal-overlay .modal-card')
  await card.waitFor()
  siteCard = await cardStyle(card)
  await page.close()
  return siteCard
}

function assertCardLooksLikeSite(style, site) {
  assert.equal(site.background, 'rgb(255, 255, 255)', 'reference: Targets shows a white card')
  assert.deepEqual(style, site, 'the Search dialog card looks like the Targets one')
}

function assertNextToButton(btnBox, cardBox, viewportHeight) {
  const btnMid = btnBox.y + btnBox.height / 2
  const cardMid = cardBox.y + cardBox.height / 2
  assert.ok(Math.abs(cardMid - btnMid) <= cardBox.height,
    `dialog level with the clicked button (button middle ${Math.round(btnMid)}, dialog middle ${Math.round(cardMid)})`)
  assert.ok(cardBox.y >= 0 && cardBox.y + cardBox.height <= viewportHeight, 'dialog fully inside the window')
}

for (const [label, rowName, button] of [
  ['Unpause', 'Server 15', 'Unpause'],
  ['Pause', 'Server 16', 'Pause'],
  ['Delete', 'Server 17', 'Delete'],
]) {
  test(`Search: the ${label} dialog is a white card next to the row, and Cancel sends nothing`, async t => {
    const { page, writes } = await searchPage(t)
    for (const block of ['center', 'end']) {
      const { card, btnBox, cardBox, style } = await openDialog(page, rowName, button, block)
      assertCardLooksLikeSite(style, await targetsCard())
      assertNextToButton(btnBox, cardBox, 900)
      await card.getByRole('button', { name: 'Cancel', exact: true }).click()
      await card.waitFor({ state: 'detached' })
    }
    assert.deepEqual(writes, [], 'Cancel sends no request')
  })
}

test('Search: confirming Unpause still unpauses the row it was opened for', async t => {
  const { page, writes } = await searchPage(t)
  const { card } = await openDialog(page, 'Server 20', 'Unpause', 'center')
  await card.getByRole('button', { name: 'Unpause', exact: true }).click()
  await card.waitFor({ state: 'detached' })
  await page.waitForFunction(() => document.querySelector('.success-msg'))
  assert.deepEqual(writes, ['POST /api/targets/t20/unpause'])
})
