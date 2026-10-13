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

async function editor(t, mode = 'new') {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1100 } })
  const browserErrors = []
  page.on('pageerror', error => browserErrors.push(error.message))
  t.after(async () => {
    await page.close()
    assert.deepEqual(browserErrors, [], 'browser JavaScript errors')
  })
  page.setDefaultTimeout(3000)
  const writes = []
  const target = {
    id: 'existing', name: 'Existing target', host: 'example.invalid', category: 'Other',
    enabled: true, tags: [], recipient_ids: [], condition_count: 1,
    conditions: [{ check_id: 'check', check_type: 'ping', check_name: 'Ping', config: '{}',
      interval_s: 300, fail_count: 10, fail_window: 1800, field: 'status',
      comparator: 'eq', value: 'down', condition_group: 0, group_operator: 'AND' }],
  }
  await page.route(`${baseURL}api/**`, async route => {
    const request = route.request()
    const url = new URL(request.url())
    let json
    if (request.method() !== 'GET') {
      writes.push({ method: request.method(), path: url.pathname, body: request.postDataJSON() })
      json = { id: 'saved' }
    } else if (url.pathname === '/api/me') json = { id: 'operator', username: 'tester', role: 'operator' }
    else if (url.pathname === '/api/tags') json = url.searchParams.get('group') === 'category' ? [{ id: 'other', value: 'Other' }] : []
    else if (url.pathname === '/api/users') json = []
    else if (url.pathname === '/api/system/health') json = {
      net: { status: 'ok', latency_ms: 1 }, disk: { total_gb: 100, free_gb: 80 },
      cpu: { load1: 0, num_cpu: 4 }, scheduler: { status: 'ok', active_checks: 0, stale_seconds: 0 },
    }
    else if (url.pathname === '/api/targets') json = mode === 'new' ? [] : [target]
    else if (url.pathname === '/api/targets/existing') json = target
    else return route.fulfill({ status: 404, json: { error: 'Unexpected fixture request' } })
    await route.fulfill({ json })
  })
  await page.goto(`${baseURL}targets`)
  await page.getByRole('button', { name: mode === 'new' ? '+ Target' : mode === 'clone' ? 'Clone' : 'Edit', exact: true }).click()
  const modal = page.locator('.modal-card')
  const input = label => modal.locator('.form-group').filter({
    has: page.locator('label').filter({ hasText: new RegExp(`^${label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}$`) }),
  }).locator('input, select').first()
  if (mode === 'new') {
    await input('Name').fill('SR13 browser fixture')
    await input('Host').fill('example.invalid')
    await input('Category').selectOption('Other')
    await modal.getByRole('button', { name: '+ Add Group or Condition', exact: true }).click()
    await input('Check Name').fill('Ping')
  }
  const set = async (label, value) => {
    await input(label).fill(String(value))
    await input(label).press('Tab')
  }
  return { page, modal, input, set, writes, save: modal.getByRole('button', { name: 'Save', exact: true }) }
}

test('shows inline feedback and blocks Save before submission', async t => {
  const e = await editor(t)
  await e.set('Interval (seconds)', 300)
  await e.set('Fail Count', 10)
  const alert = e.modal.locator('.condition-card [role="alert"]')
  await alert.waitFor({ state: 'visible' })
  assert.match(await alert.textContent(), /1800|30 minutes/)
  assert.match(await alert.textContent(), /[Rr]educe/)
  assert.equal(await e.save.isDisabled(), true)
  assert.equal(e.writes.length, 0)
  if (process.env.SR13_SCREENSHOT_PATH) await e.page.screenshot({ path: process.env.SR13_SCREENSHOT_PATH, fullPage: true })
})

test('submission guard prevents requests even when native validation is bypassed', async t => {
  const e = await editor(t)
  await e.set('Interval (seconds)', 300)
  await e.set('Fail Count', 10)
  await e.modal.locator('form').dispatchEvent('submit')
  // Let the submit handler, Vue updates and pending request interception finish.
  await e.page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
  assert.deepEqual(e.writes, [])
})

for (const [label, value] of [['Fail Count', 6], ['Interval (seconds)', 180]]) {
  test(`correcting ${label} clears the error and permits an actual save at the limit`, async t => {
    const e = await editor(t)
    await e.set('Interval (seconds)', 300)
    await e.set('Fail Count', 10)
    await e.set(label, value)
    assert.equal(await e.modal.locator('.condition-card [role="alert"]').count(), 0)
    assert.equal(await e.save.isEnabled(), true)
    assert.equal(await e.modal.locator('form').evaluate(form => form.checkValidity()), true)
    const sent = e.page.waitForRequest(request => request.method() === 'POST' && new URL(request.url()).pathname === '/api/targets')
    await e.save.click()
    const body = (await sent).postDataJSON()
    assert.equal(body.conditions[0].fail_count * body.conditions[0].interval_s, 1800)
    assert.equal(body.conditions[0].fail_window, 1800)
  })
}

for (const mode of ['edit', 'clone']) {
  test(`${mode} immediately flags a previously saved impossible threshold`, async t => {
    const e = await editor(t, mode)
    await e.modal.locator('.condition-card [role="alert"]').waitFor({ state: 'visible' })
    assert.equal(await e.save.isDisabled(), true)
    await e.set('Fail Count', 6)
    assert.equal(await e.modal.locator('.condition-card [role="alert"]').count(), 0)
    assert.equal(await e.save.isEnabled(), true)
  })
}

test('switching back to one failure permits a long interval and resets the window', async t => {
  const e = await editor(t)
  await e.set('Interval (seconds)', 3600)
  await e.set('Fail Count', 2)
  await e.set('Fail Count', 1)
  assert.equal(await e.modal.locator('.condition-card [role="alert"]').count(), 0)
  assert.equal(await e.input('Time Window').isDisabled(), true)
  assert.equal(await e.input('Time Window').inputValue(), '0')
  assert.equal(await e.save.isEnabled(), true)
  assert.equal(await e.modal.locator('form').evaluate(form => form.checkValidity()), true)
  const sent = e.page.waitForRequest(request => request.method() === 'POST' && new URL(request.url()).pathname === '/api/targets')
  await e.save.click()
  const condition = (await sent).postDataJSON().conditions[0]
  assert.equal(condition.interval_s, 3600)
  assert.equal(condition.fail_count, 1)
  assert.equal(condition.fail_window, 0)
})
