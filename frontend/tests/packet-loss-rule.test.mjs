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

const LOSS_MESSAGE = 'Enter a percentage from 0 to 100, such as 30.'
const MS_MESSAGE = 'Enter a number of milliseconds, such as 500.'

function savedTarget(condition) {
  return {
    id: 'existing', name: 'Branch router', host: 'router.example.invalid', category: 'Other',
    enabled: true, tags: [], recipient_ids: [], condition_count: 1,
    conditions: [{ check_id: 'check', check_name: 'Ping', interval_s: 60, fail_count: 3,
      fail_window: 180, condition_group: 0, group_operator: 'AND', ...condition }],
  }
}

// Opens the target editor. 'new' starts a target with one new ping condition;
// otherwise `saved` is the target the editor opens.
async function editor(t, saved) {
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
    else if (url.pathname === '/api/targets') json = saved ? [saved] : []
    else if (url.pathname === '/api/targets/existing') json = saved
    else return route.fulfill({ status: 404, json: { error: 'Unexpected fixture request' } })
    await route.fulfill({ json })
  })
  await page.goto(`${baseURL}targets`)
  await page.getByRole('button', { name: saved ? 'Edit' : '+ Target', exact: true }).click()
  const modal = page.locator('.modal-card')
  const input = label => modal.locator('.form-group').filter({
    has: page.locator('label').filter({ hasText: new RegExp(`^${label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}$`) }),
  }).locator('input, select').first()
  if (!saved) {
    await input('Name').fill('Branch router')
    await input('Host').fill('router.example.invalid')
    await input('Category').selectOption('Other')
    await modal.getByRole('button', { name: '+ Add Group or Condition', exact: true }).click()
    await input('Check Type').selectOption('ping')
    await input('Check Name').fill('Ping')
  }
  await input('Field').waitFor({ state: 'visible' })
  const fieldOptions = () => input('Field').locator('option').allTextContents()
  // [field, comparator, value] as the form holds them
  const rule = async () => [await input('Field').inputValue(), await input('Comparator').inputValue(), await input('Value').inputValue()]
  const hint = () => modal.locator('.input-hint').filter({ hasText: 'pings that got no reply' })
  const save = () => modal.getByRole('button', { name: 'Save', exact: true }).click()
  // Save refused by the browser: Value carries the message and nothing is sent
  const refused = async message => {
    await save()
    assert.equal(await input('Value').evaluate(el => el.validationMessage), message)
    assert.equal(writes.length, 0, 'nothing sent')
  }
  // Saves and returns the one saved condition
  const saved1 = async (method, path) => {
    await save()
    await page.waitForFunction(() => !document.querySelector('.modal-card'))
    const write = writes.find(w => w.method === method && w.path === path)
    assert.ok(write, `${method} ${path} sent`)
    assert.equal(write.body.conditions.length, 1)
    return write.body.conditions[0]
  }
  return { modal, input, fieldOptions, rule, hint, refused, saved1 }
}

test('a ping condition can alert on packet loss, with greater than and a number', async t => {
  const e = await editor(t)
  assert.deepEqual(await e.fieldOptions(), ['Status', 'Response (ms)', 'Packet loss (%)'])
  assert.deepEqual(await e.rule(), ['status', 'eq', 'down'])
  await e.input('Field').selectOption('metrics.packet_loss')
  assert.deepEqual(await e.rule(), ['metrics.packet_loss', 'gt', ''], 'greater than, Value emptied')
  assert.match(await e.hint().textContent(), /3 pings that got no reply, in steps of 33\.3%/)
  await e.refused(LOSS_MESSAGE)
  await e.input('Value').fill('down')
  await e.refused(LOSS_MESSAGE)
  await e.input('Value').fill('150')
  await e.refused(LOSS_MESSAGE)
  await e.input('Value').fill('30')
  assert.match(await e.hint().textContent(), /Greater than 30 = 1 or more of the 3 lost/)
  const cond = await e.saved1('POST', '/api/targets')
  assert.equal(cond.check_type, 'ping')
  assert.deepEqual([cond.field, cond.comparator, cond.value], ['metrics.packet_loss', 'gt', '30'])
})

test('packet loss is offered for ping only; leaving ping resets the rule to Status equals down', async t => {
  const e = await editor(t)
  assert.ok((await e.fieldOptions()).includes('Packet loss (%)'), 'offered while the check is ping')
  await e.input('Field').selectOption('metrics.packet_loss')
  await e.input('Value').fill('30')
  await e.input('Check Type').selectOption('http')
  assert.deepEqual(await e.fieldOptions(), ['Status', 'Response (ms)'])
  assert.deepEqual(await e.rule(), ['status', 'eq', 'down'])
  assert.equal(await e.hint().count(), 0)
})

test('Response (ms) also gets greater than and must hold a number; Status goes back to equals down', async t => {
  const e = await editor(t)
  await e.input('Field').selectOption('response_ms')
  assert.deepEqual(await e.rule(), ['response_ms', 'gt', ''])
  await e.refused(MS_MESSAGE)
  await e.input('Value').fill('down')
  await e.refused(MS_MESSAGE)
  await e.input('Field').selectOption('status')
  assert.deepEqual(await e.rule(), ['status', 'eq', 'down'])
  await e.input('Field').selectOption('response_ms')
  await e.input('Value').fill('500')
  const cond = await e.saved1('POST', '/api/targets')
  assert.deepEqual([cond.field, cond.comparator, cond.value], ['response_ms', 'gt', '500'])
})

test('a saved packet-loss rule opens as stored, with steps for its Ping Count', async t => {
  const e = await editor(t, savedTarget({ check_type: 'ping', config: '{"count":10,"timeout_s":5}',
    field: 'metrics.packet_loss', comparator: 'gt', value: '30' }))
  assert.deepEqual(await e.rule(), ['metrics.packet_loss', 'gt', '30'])
  const text = await e.hint().textContent()
  assert.match(text, /10 pings that got no reply, in steps of 10%/)
  assert.match(text, /Greater than 30 = 4 or more of the 10 lost/)
  const cond = await e.saved1('PUT', '/api/targets/existing')
  assert.deepEqual([cond.field, cond.comparator, cond.value], ['metrics.packet_loss', 'gt', '30'])
})

test('a saved Response (ms) rule without a number is refused on Save until it is fixed', async t => {
  const e = await editor(t, savedTarget({ check_type: 'http', config: '{"scheme":"https","port":0,"endpoint":"/","expect_status":200,"skip_tls_verify":false,"timeout_s":10}',
    field: 'response_ms', comparator: 'eq', value: 'down' }))
  assert.deepEqual(await e.rule(), ['response_ms', 'eq', 'down'], 'opens as stored')
  await e.refused(MS_MESSAGE)
  await e.input('Value').fill('3000')
  const cond = await e.saved1('PUT', '/api/targets/existing')
  assert.deepEqual([cond.field, cond.comparator, cond.value], ['response_ms', 'eq', '3000'])
})
