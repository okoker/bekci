// Shared set-up for the in-app help browser tests (help-*.test.mjs). Not a test
// file itself. Every /api request is answered from the fixtures below: no
// backend, no credentials, no live data.
//
//   let app
//   before(async () => { app = await startApp() })
//   after(() => stopApp(app))
//
//   test('…', async t => {
//     const { page, writes } = await openPage(t, app, '/targets', { role: 'operator' })
//     await turnHelpOn(page)
//   })
import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright-core'
import { createServer } from 'vite'
import vue from '@vitejs/plugin-vue'
import { HELP_VERSION } from '../src/help/content/version.js'

// ── Vite + Chrome ────────────────────────────────────────────────────────────

// Starts a Vite dev server for frontend/ (no backend proxy) and Chrome.
// Chrome comes from CHROME_PATH, or the installed Google Chrome.
export async function startApp() {
  const server = await createServer({
    configFile: false, root: fileURLToPath(new URL('../', import.meta.url)),
    plugins: [vue()], logLevel: 'error', server: { host: '127.0.0.1', port: 0 },
  })
  await server.listen()
  try {
    const browser = await chromium.launch(process.env.CHROME_PATH
      ? { executablePath: process.env.CHROME_PATH }
      : { channel: 'chrome' })
    return { server, browser, baseURL: server.resolvedUrls.local[0] }
  } catch (error) {
    await server.close()
    throw error
  }
}

export async function stopApp(app) {
  await app?.browser.close()
  await app?.server.close()
}

// ── The fixture world ────────────────────────────────────────────────────────
// Three users (one per role), two targets and their three checks:
//   t1 Core switch (Network): one ping check, healthy.
//   t2 Web shop (Key Services): HTTP "Web page" (preferred, PRIMARY, down) AND
//      ping, unhealthy.
// Enough for every page to render rows; a test changes what it needs through
// openPage's `fixtures` option.

const MIN = 60_000
const DAY = 86_400_000
const ago = ms => new Date(Date.now() - ms).toISOString()
const day = n => new Date(Date.now() - n * DAY).toISOString().slice(0, 10)

const user = role => ({
  id: `u-${role}`, username: role, email: `${role}@example.com`, phone: '', role, status: 'active',
  created_at: ago(30 * DAY), updated_at: ago(30 * DAY),
})

const tags = {
  category: [
    { id: 1, group: 'category', value: 'Key Services', sla_key: 'sla_key_services' },
    { id: 2, group: 'category', value: 'Network', sla_key: 'sla_network' },
    { id: 3, group: 'category', value: 'Other', sla_key: 'sla_other' },
  ],
  project: [{ id: 4, group: 'project', value: 'ACME' }],
  location: [{ id: 5, group: 'location', value: 'DC-1' }],
  tag: [{ id: 6, group: 'tag', value: 'P1' }],
}

// What Bekci holds about each check: the condition in the target's rule, and
// its live state on the Dashboard.
const checks = [
  { target: 't1', id: 'c1', name: 'Ping', type: 'ping', config: { count: 3, timeout_s: 5 },
    status: 'up', message: '3/3 replies', ms: 2, uptime: 100 },
  { target: 't2', id: 'c2', name: 'Web page', type: 'http',
    config: { scheme: 'https', port: 0, endpoint: '/', expect_status: 200, skip_tls_verify: false, timeout_s: 10 },
    status: 'down', message: 'HTTP 503', ms: 120, uptime: 98.2 },
  { target: 't2', id: 'c3', name: 'Ping', type: 'ping', config: { count: 3, timeout_s: 5 },
    status: 'up', message: '3/3 replies', ms: 3, uptime: 100 },
]

const targets = [
  { id: 't1', name: 'Core switch', host: 'switch.example.com', category: 'Network', preferred_check_type: 'ping',
    primary_check_id: 'c1', state: 'healthy', sla_status: 'healthy', sla_target: 99.9, recipient_ids: ['u-operator'] },
  { id: 't2', name: 'Web shop', host: 'shop.example.com', category: 'Key Services', preferred_check_type: 'http',
    primary_check_id: 'c2', state: 'unhealthy', sla_status: 'unhealthy', sla_target: 99.5, recipient_ids: ['u-admin'] },
]

const checksOf = target => checks.filter(c => c.target === target.id)

const targetSummary = t => ({
  id: t.id, name: t.name, host: t.host, description: '', enabled: true,
  preferred_check_type: t.preferred_check_type, operator: 'AND', category: t.category, rule_id: `r-${t.id}`,
  notes: null, contacts: null, project: 'ACME', location: 'DC-1', tags: ['P1'],
  created_at: ago(30 * DAY), updated_at: ago(DAY), condition_count: checksOf(t).length,
  state: { rule_id: `r-${t.id}`, current_state: t.state, last_change: ago(DAY), last_evaluated: ago(MIN) },
  primary_check_id: t.primary_check_id,
})

const targetDetail = t => ({
  ...targetSummary(t),
  notes: 'Rack 4', contacts: 'ops@example.com', recipient_ids: t.recipient_ids,
  conditions: checksOf(t).map(c => ({
    check_id: c.id, check_type: c.type, check_name: c.name, config: JSON.stringify(c.config),
    interval_s: 60, field: 'status', comparator: 'eq', value: 'down', fail_count: 3, fail_window: 180,
    condition_group: 0, group_operator: 'AND',
  })),
})

const checkRows = t => checksOf(t).map(c => ({
  id: c.id, target_id: t.id, type: c.type, name: c.name, config: JSON.stringify(c.config),
  interval_s: 60, enabled: true, created_at: ago(30 * DAY), updated_at: ago(DAY),
}))

const statusRow = t => ({
  id: t.id, name: t.name, host: t.host, preferred_check_type: t.preferred_check_type,
  primary_check_id: t.primary_check_id, state: t.state, paused: false, category: t.category, tags: ['P1'],
  sla_status: t.sla_status, sla_target: t.sla_target,
  checks: checksOf(t).map(c => ({
    id: c.id, name: c.name, type: c.type, enabled: true, interval_s: 60, last_status: c.status,
    last_message: c.message, response_ms: c.ms, uptime_90d_pct: c.uptime,
  })),
})

// /history/{checkId}: ?range=4h gives the last runs, otherwise daily uptime.
const history = c => url => url.searchParams.get('range') === '4h'
  ? [3, 2, 1].map(n => ({ status: c.status, response_ms: c.ms, checked_at: ago(n * MIN) }))
  : [3, 2, 1].map(n => ({ date: day(n), uptime_pct: c.uptime, total_checks: 1440 }))

const health = version => ({
  version,
  net: { status: 'ok', latency_ms: 12 },
  disk: { total_gb: 100, free_gb: 80 },
  cpu: { load1: 0.2, num_cpu: 4 },
  scheduler: { status: 'ok', last_tick: ago(10_000), active_checks: checks.length, stale_seconds: 10 },
})

const systemInfo = version => ({
  disk: { total_gb: 100, free_gb: 80, used_pct: 20 },
  ram: { total_gb: 8, used_gb: 1, used_pct: 12.5 },
  cpu: { load1: 0.2, load5: 0.1, load15: 0.1, num_cpu: 4 },
  uptime: { system_sec: 864000, process_sec: 3600 },
  server_ip: '192.0.2.20',
  version,
  build: { commit: 'abc1234', time: ago(DAY) },
  go_version: 'go1.25.0',
  database: { status: 'ok', size_bytes: 10485760, wal_bytes: 0, schema_version: 27 },
  email: { configured: true, provider: 'Resend' },
  updates: { status: 'up_to_date', total: 0, security: 0, checked_at: ago(MIN) },
  backup: { count: 0, last_at: '', last_bytes: 0, encrypted: false, schedule: 'off', time: '03:00' },
  log: { path: '/var/log/bekci/bekci.log', size_bytes: 1048576 },
  scheduler: { status: 'ok', last_tick: ago(10_000), active_checks: checks.length, stale_seconds: 10 },
})

const settings = {
  session_timeout_hours: '24', history_days: '90', audit_retention_days: '91', backup_max_copies: '5',
  soc_public: 'false', api_rate_limit_per_min: '60',
  sla_key_services: '99.5', sla_network: '99.9', sla_other: '99',
  alert_method: 'email', email_provider: 'resend', resend_api_key: '', alert_from_email: 'alerts@example.com',
  alert_cooldown_s: '1800', alert_realert_s: '3600', webhook_enabled: 'false',
  auto_backup_schedule: 'off', auto_backup_time: '03:00',
}

// A fixture answer other than 200 OK: reply(503, { error: '…' }).
class Reply {
  constructor(status, json) { this.status = status; this.json = json }
}
export const reply = (status, json = {}) => new Reply(status, json)

// The default answer to every GET the pages make, by path. A value is the JSON
// body, a Reply, or a function of the request URL returning either.
// `role` signs in as admin, operator or viewer (null: signed out);
// `version` is the running Bekci version /api/system/health reports.
export function apiFixtures({ role = 'admin', version = HELP_VERSION } = {}) {
  const fixtures = {
    '/api/me': role ? user(role) : reply(401, { error: 'unauthorized' }),
    '/api/users': ['admin', 'operator', 'viewer'].map(user),
    '/api/tags': url => tags[url.searchParams.get('group')] ?? [],
    '/api/system/health': health(version),
    '/api/system/info': systemInfo(version),
    '/api/targets': targets.map(targetSummary),
    '/api/dashboard/status': targets.map(statusRow),
    '/api/soc/status': targets.map(statusRow),
    '/api/sla/history': {
      categories: tags.category.map(cat => ({
        name: cat.value, sla_threshold: Number(settings[cat.sla_key]),
        targets: targets.filter(t => t.category === cat.value).map(t => {
          const c = checks.find(c => c.id === t.primary_check_id)
          return { id: t.id, name: t.name, uptime_90d_pct: c.uptime,
            daily_uptime: [3, 2, 1].map(n => ({ date: day(n), uptime_pct: c.uptime })) }
        }),
      })),
      pause_stats: { count: 0, affected_hosts: 0 },
    },
    '/api/alerts': {
      entries: [{ id: 1, rule_id: 'r-t2', target_id: 't2', target_name: 'Web shop', recipient_id: 'u-admin',
        recipient_name: 'admin', alert_type: 'firing', message: 'Target Web shop is unhealthy', sent_at: ago(10 * MIN) }],
      total: 1,
    },
    '/api/audit-log': {
      entries: [{ id: 1, user_id: 'u-admin', username: 'admin', action: 'login', resource_type: 'session',
        resource_id: '', detail: '', ip_address: '192.0.2.10', status: 'success', created_at: ago(60 * MIN) }],
      total: 1, page: 1, limit: 50,
    },
    '/api/settings': settings,
    '/api/settings/webhook-status': { last_error: '', last_success: '' },
    '/api/api-tokens': [{ id: 'k1', name: 'grafana', prefix: 'bk_a1b2c3d4', created_by: 'u-admin', created_at: ago(DAY) }],
    '/api/backup/full/list': [],
    '/api/fail2ban/status': {
      jails: [{ name: 'sshd', currently_failed: 0, total_failed: 3, currently_banned: 0, total_banned: 1, banned_ips: [] }],
      fetched_at: ago(0),
    },
    '/api/fail2ban/bans': { bans: [] },
  }
  for (const t of targets) {
    fixtures[`/api/targets/${t.id}`] = targetDetail(t)
    fixtures[`/api/targets/${t.id}/checks`] = checkRows(t)
  }
  for (const c of checks) {
    fixtures[`/api/dashboard/history/${c.id}`] = history(c)
    fixtures[`/api/soc/history/${c.id}`] = history(c)
  }
  return fixtures
}

// ── A page ───────────────────────────────────────────────────────────────────

// Opens `path` in a new page, signed in as `role`, every API request answered
// from apiFixtures({ role, version }) with `fixtures` laid over it (same keys:
// a GET path, or "METHOD /path" for a write). `delays` holds back the answer
// for a path by that many ms. Writes (anything but GET) are recorded and
// answered {} unless a fixture says otherwise.
// When the test ends the page closes, and the test fails if the page threw a
// JavaScript error or asked for an API path no fixture answers.
export async function openPage(t, app, path, {
  role = 'admin', version = HELP_VERSION, fixtures = {}, delays = {},
  viewport = { width: 1440, height: 1100 },
} = {}) {
  const page = await app.browser.newPage({ viewport })
  const errors = []
  const unexpected = []
  const writes = []
  page.on('pageerror', error => errors.push(error.message))
  t.after(async () => {
    await page.close()
    assert.deepEqual(errors, [], 'browser JavaScript errors')
    assert.deepEqual(unexpected, [], 'API requests no fixture answers')
  })
  page.setDefaultTimeout(3000)
  const answers = { ...apiFixtures({ role, version }), ...fixtures }
  await page.route(`${app.baseURL}api/**`, async route => {
    const request = route.request()
    const url = new URL(request.url())
    const method = request.method()
    let answer
    if (method === 'GET') {
      answer = answers[url.pathname]
    } else {
      let body
      try { body = request.postDataJSON() } catch { body = request.postData() }
      writes.push({ method, path: url.pathname, body })
      answer = answers[`${method} ${url.pathname}`] ?? {}
    }
    if (typeof answer === 'function') answer = answer(url, request)
    if (answer === undefined) {
      unexpected.push(`${method} ${url.pathname}${url.search}`)
      answer = reply(404, { error: 'No fixture for this request' })
    }
    if (delays[url.pathname]) await new Promise(resolve => setTimeout(resolve, delays[url.pathname]))
    try {
      await route.fulfill(answer instanceof Reply ? { status: answer.status, json: answer.json } : { json: answer })
    } catch (error) {
      if (!page.isClosed()) throw error // the test ended while an answer was held back
    }
  })
  await page.goto(new URL(path, app.baseURL).href)
  return { page, writes }
}

// ── Help ─────────────────────────────────────────────────────────────────────

export const helpButton = page => page.locator('.navbar').getByRole('button', { name: 'Help', exact: true })

export async function turnHelpOn(page) {
  await helpButton(page).click()
  await page.locator('.help-bar').waitFor()
}

// Waits until the element's computed `prop` (camelCase) is `value`, riding out
// CSS transitions; fails with the value it ended on.
export async function waitForStyle(locator, prop, value) {
  const el = await locator.elementHandle()
  try {
    await locator.page().waitForFunction(([el, prop, value]) => getComputedStyle(el)[prop] === value,
      [el, prop, value], { timeout: 2000 })
  } catch {
    assert.equal(await locator.evaluate((el, prop) => getComputedStyle(el)[prop], prop), value, prop)
  }
}

// Every use of Bekci's orange (#ea580c, rgb(234, 88, 12)) in the elements
// matching `selector`, their descendants and their ::before/::after: a list of
// "class property", empty when there is none. Help never uses the orange.
export function orangeIn(page, selector) {
  return page.evaluate(selector => {
    const props = ['color', 'backgroundColor', 'borderTopColor', 'borderRightColor', 'borderBottomColor',
      'borderLeftColor', 'outlineColor', 'boxShadow', 'backgroundImage']
    const found = []
    for (const root of document.querySelectorAll(selector)) {
      for (const el of [root, ...root.querySelectorAll('*')]) {
        for (const pseudo of [null, '::before', '::after']) {
          const style = getComputedStyle(el, pseudo)
          for (const prop of props) {
            if (style[prop].includes('234, 88, 12')) {
              found.push(`${el.getAttribute('class') || el.tagName}${pseudo || ''} ${prop}`)
            }
          }
        }
      }
    }
    return found
  }, selector)
}
