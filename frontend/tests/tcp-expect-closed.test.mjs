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

const HINT = 'Up while the port is closed or does not answer, down when it accepts a connection. The usual Status equals down then alerts when the port opens.'

// Opens the target editor. 'new' starts a target with one TCP condition;
// 'edit' opens a saved target whose TCP check expects port 3389 to be closed.
async function editor(t, mode) {
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
    id: 'existing', name: 'Office server', host: 'server.example.invalid', category: 'Other',
    enabled: true, tags: [], recipient_ids: [], condition_count: 1,
    conditions: [{ check_id: 'check', check_type: 'tcp', check_name: 'Remote desktop port',
      config: '{"port":3389,"timeout_s":5,"expect_closed":true}', interval_s: 300, fail_count: 2,
      fail_window: 600, field: 'status', comparator: 'eq', value: 'down', condition_group: 0, group_operator: 'AND' }],
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
  await page.getByRole('button', { name: mode === 'new' ? '+ Target' : 'Edit', exact: true }).click()
  const modal = page.locator('.modal-card')
  const input = label => modal.locator('.form-group').filter({
    has: page.locator('label').filter({ hasText: new RegExp(`^${label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}$`) }),
  }).locator('input, select').first()
  if (mode === 'new') {
    await input('Name').fill('Office server')
    await input('Host').fill('server.example.invalid')
    await input('Category').selectOption('Other')
    await modal.getByRole('button', { name: '+ Add Group or Condition', exact: true }).click()
    await input('Check Type').selectOption('tcp')
    await input('Check Name').fill('Remote desktop port')
  }
  const box = modal.getByLabel('Port must stay closed')
  await box.waitFor({ state: 'visible' })
  // Saves and returns the one saved condition, its config parsed
  const save = async (method, path) => {
    await modal.getByRole('button', { name: 'Save', exact: true }).click()
    await page.waitForFunction(() => !document.querySelector('.modal-card'))
    const write = writes.find(w => w.method === method && w.path === path)
    assert.ok(write, `${method} ${path} sent`)
    assert.equal(write.body.conditions.length, 1)
    const cond = write.body.conditions[0]
    return { ...cond, config: JSON.parse(cond.config) }
  }
  return { modal, box, save }
}

test('a new TCP check can be set to expect its port to be closed', async t => {
  const e = await editor(t, 'new')
  assert.equal(await e.box.isChecked(), false, 'off by default')
  assert.equal(await e.modal.getByText(HINT).count(), 0, 'no hint while off')
  await e.box.check()
  assert.equal(await e.modal.getByText(HINT).isVisible(), true)
  const cond = await e.save('POST', '/api/targets')
  assert.equal(cond.check_type, 'tcp')
  assert.deepEqual(cond.config, { port: 80, timeout_s: 5, expect_closed: true })
  // The default rule already alerts when the port opens
  assert.deepEqual([cond.field, cond.comparator, cond.value], ['status', 'eq', 'down'])
})

test('a saved closed-port check opens ticked, and unticking it saves false', async t => {
  const e = await editor(t, 'edit')
  assert.equal(await e.box.isChecked(), true)
  assert.equal(await e.modal.getByText(HINT).isVisible(), true)
  await e.box.uncheck()
  assert.equal(await e.modal.getByText(HINT).count(), 0)
  const cond = await e.save('PUT', '/api/targets/existing')
  assert.deepEqual(cond.config, { port: 3389, timeout_s: 5, expect_closed: false })
})
