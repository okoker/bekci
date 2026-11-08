import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright-core'
import { createServer } from 'vite'
import vue from '@vitejs/plugin-vue'

// A-082: the target form refuses a Host the server would refuse, shows the
// address an HTTP or Page Hash check will request, sends the Host as typed, and
// shows a Save refusal next to the Save button.

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

const SCHEME = 'Leave out http:// or https://: each HTTP or Page Hash check has its own Scheme.'

// Opens New target with Name, Category and one condition of checkType filled in.
// saveRefusal: the error the API answers a POST with (status 400), if any.
async function newTarget(t, checkType, saveRefusal) {
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
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
    let json
    if (request.method() !== 'GET') {
      writes.push({ method: request.method(), path: url.pathname, body: request.postDataJSON() })
      if (saveRefusal) return route.fulfill({ status: 400, json: { error: saveRefusal } })
      json = { id: 'saved' }
    } else if (url.pathname === '/api/me') json = { id: 'operator', username: 'tester', role: 'operator' }
    else if (url.pathname === '/api/tags') json = url.searchParams.get('group') === 'category' ? [{ id: 'other', value: 'Other' }] : []
    else if (url.pathname === '/api/users') json = []
    else if (url.pathname === '/api/system/health') json = {
      net: { status: 'ok', latency_ms: 1 }, disk: { total_gb: 100, free_gb: 80 },
      cpu: { load1: 0, num_cpu: 4 }, scheduler: { status: 'ok', active_checks: 0, stale_seconds: 0 },
    }
    else if (url.pathname === '/api/targets') json = []
    else return route.fulfill({ status: 404, json: { error: 'Unexpected fixture request' } })
    await route.fulfill({ json })
  })
  await page.goto(`${baseURL}targets`)
  await page.getByRole('button', { name: '+ Target', exact: true }).click()
  const modal = page.locator('.modal-card')
  const input = label => modal.locator('.form-group').filter({
    has: page.locator('label').filter({ hasText: new RegExp(`^${label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}$`) }),
  }).locator('input, select').first()
  await input('Name').fill('Web site')
  await input('Category').selectOption('Other')
  await modal.getByRole('button', { name: '+ Add Group or Condition', exact: true }).click()
  await input('Check Type').selectOption(checkType)
  await input('Check Name').fill('Site check')
  const addressLines = () => modal.locator('.check-url').allTextContents()
  return { page, modal, input, writes, addressLines }
}

test('a Host with a scheme is refused on the field with the server message, and nothing is sent', async t => {
  const e = await newTarget(t, 'http')
  await e.input('Host').fill('https://example.com')
  assert.equal(await e.input('Host').evaluate(el => el.validationMessage), SCHEME)
  await e.modal.getByRole('button', { name: 'Save', exact: true }).click()
  // Past the browser's popup too: the form's own guard holds a direct submit
  await e.modal.locator('form').evaluate(form => form.dispatchEvent(new Event('submit', { cancelable: true })))
  await e.page.waitForTimeout(200)
  assert.deepEqual(e.writes, [])
  assert.deepEqual(await e.addressLines(), [], 'no address while the Host is refused')

  await e.input('Host').fill('example.com:8443')
  assert.equal(await e.input('Host').evaluate(el => el.validationMessage),
    'Leave out the port: each check has its own Port field. Write an IPv6 address without brackets.')
  await e.input('Host').fill('www.somehost.com/health')
  assert.equal(await e.input('Host').evaluate(el => el.validationMessage), '')
})

test('the HTTP and Page Hash checks show the address they will request', async t => {
  const e = await newTarget(t, 'http')
  assert.deepEqual(await e.addressLines(), [], 'no address while the Host is empty')
  await e.input('Host').fill('www.somehost.com/health')
  assert.deepEqual(await e.addressLines(), ['Checks https://www.somehost.com/health'])
  await e.input('Port (0 = default)').fill('8443')
  assert.deepEqual(await e.addressLines(), ['Checks https://www.somehost.com:8443/health'])
  await e.input('Endpoint').fill('status')
  assert.deepEqual(await e.addressLines(), ['Checks https://www.somehost.com:8443/health/status'])
  await e.input('Scheme').selectOption('http')
  assert.deepEqual(await e.addressLines(), ['Checks http://www.somehost.com:8443/health/status'])

  await e.input('Check Type').selectOption('page_hash')
  assert.deepEqual(await e.addressLines(), ['Checks https://www.somehost.com/health'])
  await e.input('Host').fill('somehost.com/health/')
  assert.deepEqual(await e.addressLines(), ['Checks https://somehost.com/health/'])

  // Other check types have no address line
  await e.input('Check Type').selectOption('tls_cert')
  assert.deepEqual(await e.addressLines(), [])
})

test('Save sends the Host as typed, path and trailing slash included', async t => {
  const e = await newTarget(t, 'http')
  await e.input('Host').fill('somehost.com/health/')
  await e.modal.getByRole('button', { name: 'Save', exact: true }).click()
  await e.page.waitForFunction(() => !document.querySelector('.modal-card'))
  const write = e.writes.find(w => w.method === 'POST' && w.path === '/api/targets')
  assert.ok(write, 'POST /api/targets sent')
  assert.equal(write.body.host, 'somehost.com/health/')
})

test('a Save the server refuses shows its message next to the Save button', async t => {
  const e = await newTarget(t, 'http', 'a target with this name already exists')
  await e.input('Host').fill('example.com')
  const save = e.modal.getByRole('button', { name: 'Save', exact: true })
  await save.click()
  const error = e.modal.locator('.error-msg')
  await error.waitFor({ state: 'visible' })
  assert.equal(await error.textContent(), 'a target with this name already exists')
  // Right above the Save row, in view where Save was pressed
  assert.equal(await error.evaluate(el => el.nextElementSibling?.classList.contains('form-actions')), true)
  assert.equal(await error.isVisible(), true)
  const [errorBox, saveBox] = [await error.boundingBox(), await save.boundingBox()]
  assert.ok(saveBox.y - (errorBox.y + errorBox.height) < 40, 'message sits just above Save')
  assert.equal(await error.evaluate(el => {
    const r = el.getBoundingClientRect()
    return r.top >= 0 && r.bottom <= window.innerHeight
  }), true, 'message is in the window')
})
