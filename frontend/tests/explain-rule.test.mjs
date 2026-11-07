// Pins explainRule (src/help/explainRule.js): the plain-words reading of an alert rule
// and the false-alarm checks. Pure functions, Node only.
import test from 'node:test'
import assert from 'node:assert/strict'
import { explainRule, outageDelay, failCountTimeline } from '../src/help/explainRule.js'
import { getDefaultCondition, getDefaultConfig } from '../src/conditionRules.js'

function cond(over = {}, group = 0, op = 'AND') {
  const c = getDefaultCondition(group, op)
  const type = over.check_type || c.check_type
  return { ...c, config: getDefaultConfig(type), ...over }
}
const sentence = (r, i = 0) => r.groups[i].sentence
const texts = (r) => r.checks.map((c) => c.text)

test('B1: ping, 3 in a row, every minute', () => {
  const r = explainRule([cond({ fail_count: 3, fail_window: 180 })], 'Core switch', 1)
  assert.equal(r.opening, 'Core switch counts as down when:')
  assert.match(sentence(r), /ping has failed 3 checks in a row$/)
  assert.equal(r.groups[0].delay, '2 to 3 minutes into an outage, at one check a minute')
  assert.equal(r.closing, 'It counts as up again on the first check where this is no longer true.')
})

test('empty name falls back to "This target"', () => {
  assert.equal(explainRule([cond()], '', 1).opening, 'This target counts as down when:')
  assert.equal(explainRule([cond()], '   ', 1).opening, 'This target counts as down when:')
})

test('Fail Count 1 reads "on the latest check" with no delay', () => {
  const r = explainRule([cond()], 'T', 1)
  assert.ok(sentence(r).endsWith('on the latest check'))
  assert.equal(r.groups[0].delay, null)
})

test('two groups close with "none of these is true"', () => {
  const r = explainRule([cond({}, 0), cond({ check_type: 'tls_cert', config: { warn_days: 14 } }, 1)], 'T', 1)
  assert.equal(r.groups.length, 2)
  assert.equal(r.closing, 'It counts as up again on the first check where none of these is true.')
})

test('delays: whole minutes or seconds', () => {
  assert.equal(outageDelay(3, 45), '90 to 135 seconds into an outage, at one check every 45 seconds')
  assert.equal(outageDelay(2, 300), '5 to 10 minutes into an outage, at one check every 5 minutes')
  assert.equal(outageDelay(3, 60), '2 to 3 minutes into an outage, at one check a minute')
  assert.equal(outageDelay(1, 60), null)
  const r = explainRule([cond({ fail_count: 3, interval_s: 45, fail_window: 135 })], 'T', 1)
  assert.equal(r.groups[0].delay, '90 to 135 seconds into an outage, at one check every 45 seconds')
})

test('AND group of ping and web page', () => {
  const r = explainRule([
    cond({ fail_count: 3, fail_window: 180 }, 0, 'AND'),
    cond({ check_type: 'http', fail_count: 3, fail_window: 180 }, 0, 'AND'),
  ], 'T', 1)
  assert.equal(sentence(r), 'ping and the web page have both failed 3 checks in a row')
})

test('OR group says "any one of"', () => {
  const r = explainRule([
    cond({}, 0, 'OR'),
    cond({ check_type: 'http' }, 0, 'OR'),
  ], 'T', 1)
  assert.ok(sentence(r).includes('any one of'))
})

test('mixed operators in a group: the last condition wins (engine.go:76-79)', () => {
  const r = explainRule([cond({}, 0, 'OR'), cond({ check_type: 'http' }, 0, 'AND')], 'T', 1)
  assert.ok(!sentence(r).includes('any one of'))
  const r2 = explainRule([cond({}, 0, 'AND'), cond({ check_type: 'http' }, 0, 'OR')], 'T', 1)
  assert.ok(sentence(r2).includes('any one of'))
})

test('TCP: refused vs accepted, swapped by expect_closed and by status up', () => {
  const tcp = (config, value = 'down') => cond({ check_type: 'tcp', config, value })
  const open = { port: 3389, timeout_s: 5, expect_closed: false }
  const shut = { port: 3389, timeout_s: 5, expect_closed: true }
  assert.ok(sentence(explainRule([tcp(open)], 'T', 1)).includes('port 3389 has refused connections'))
  assert.ok(sentence(explainRule([tcp(shut)], 'T', 1)).includes('accepted a connection'))
  assert.ok(sentence(explainRule([tcp(open, 'up')], 'T', 1)).includes('accepted a connection'))
  assert.ok(sentence(explainRule([tcp(shut, 'up')], 'T', 1)).includes('refused connections'))
})

test('HTTP endpoint other than "/" names the page', () => {
  const r = explainRule([cond({ check_type: 'http', config: { ...getDefaultConfig('http'), endpoint: '/admin' } })], 'T', 1)
  assert.ok(sentence(r).includes('the page /admin'))
  assert.ok(sentence(r).includes('not answered with 200'))
  const r2 = explainRule([cond({ check_type: 'http' })], 'T', 1)
  assert.ok(sentence(r2).startsWith('the web page'))
})

test('response time and packet loss', () => {
  const r = explainRule([cond({ field: 'response_ms', comparator: 'gt', value: '3000' })], 'T', 1)
  assert.ok(sentence(r).includes('taken longer than 3000 ms'))
  const p = explainRule([cond({ field: 'metrics.packet_loss', comparator: 'gt', value: '30' })], 'T', 1)
  assert.ok(sentence(p).includes('lost more than 30% of its pings'))
})

test('DNS: nameserver, expected value, none', () => {
  const dns = (config) => cond({ check_type: 'dns', config: { ...getDefaultConfig('dns'), ...config } })
  const a = explainRule([dns({ nameserver: '8.8.8.8', expect_value: '1.2.3.4' })], 'T', 1)
  assert.ok(sentence(a).includes('at 8.8.8.8'))
  assert.ok(sentence(a).includes('an address other than 1.2.3.4'))
  assert.ok(sentence(explainRule([dns({})], 'T', 1)).includes('failed to resolve'))
})

test('other subjects and phrases', () => {
  const one = (over) => sentence(explainRule([cond(over)], 'T', 1))
  assert.ok(one({ check_type: 'page_hash' }).includes("the page's content has changed, or could not be read"))
  assert.ok(one({ check_type: 'tls_cert', config: { warn_days: 14 } }).includes('the certificate'))
  assert.ok(one({ check_type: 'tls_cert', config: { warn_days: 14 } }).includes('fewer than 14 days left, or could not be checked'))
  assert.ok(one({ check_type: 'snmp_v2c' }).startsWith('SNMP has not answered'))
  assert.ok(one({ value: 'up' }).includes('answered'))
})

test('unknown field or comparator falls back to plain words', () => {
  const s = sentence(explainRule([cond({ field: 'metrics.foo', comparator: 'gte', value: '5' })], 'T', 1))
  assert.ok(s.includes('metrics.foo') && s.includes('5'))
  const s2 = sentence(explainRule([cond({ field: 'status', comparator: 'neq', value: 'up' })], 'T', 1))
  assert.ok(s2.includes('not equals') && s2.includes('up'))
})

test('checks: two different tests agree, nothing flickers', () => {
  const r = explainRule([
    cond({ fail_count: 3, fail_window: 180 }),
    cond({ check_type: 'http', fail_count: 3, fail_window: 180 }),
  ], 'T', 1)
  assert.deepEqual(r.checks.slice(0, 2), [
    { level: 'ok', text: 'Group 1 needs two different tests to agree before it alerts.' },
    { level: 'ok', text: 'No test that can flicker alerts on a single failure.' },
  ])
})

test('checks: single failure warning names the check', () => {
  const r = explainRule([cond()], 'T', 1)
  const w = r.checks.find((c) => c.level === 'warn')
  assert.equal(w.text, 'Ping alerts on a single failed check. One blip will page people; 3 in a row is safer.')
  assert.ok(!texts(r).includes('No test that can flicker alerts on a single failure.'))
})

test('checks: OR group, Page Hash, TCP, DNS', () => {
  const or = explainRule([cond({ fail_count: 3, fail_window: 180 }, 0, 'OR'), cond({ check_type: 'http', fail_count: 3, fail_window: 180 }, 0, 'OR')], 'T', 1)
  assert.ok(texts(or).includes('Group 1 alerts when any one of its tests fails. If they should agree first, switch it to AND.'))

  const ph = explainRule([cond({ check_type: 'page_hash', fail_count: 3, fail_window: 180 })], 'T', 1)
  assert.ok(texts(ph).includes('Page Hash also fails when the site is simply down. Pair it with "web page answers" in an AND group (recipe: Defacement).'))
  const paired = explainRule([
    cond({ check_type: 'page_hash', fail_count: 3, fail_window: 180 }),
    cond({ check_type: 'http', value: 'up', fail_count: 3, fail_window: 180 }),
  ], 'T', 1)
  assert.ok(!texts(paired).some((t) => t.startsWith('Page Hash also fails')))

  const tcpUp = explainRule([cond({ check_type: 'tcp', value: 'up', fail_count: 3, fail_window: 180 })], 'T', 1)
  assert.ok(texts(tcpUp).includes("This alerts when the port opens, but the target's uptime and SLA will read close to 0%, because a closed port counts as down. Tick Port must stay closed and use Status equals down instead (recipe: A port that must stay closed)."))
  const closedUp = explainRule([cond({ check_type: 'tcp', value: 'up', config: { port: 22, timeout_s: 5, expect_closed: true }, fail_count: 3, fail_window: 180 })], 'T', 1)
  assert.ok(texts(closedUp).includes('This alerts while the port is closed. To alert when it opens, use Status equals down.'))

  const dns = explainRule([cond({ check_type: 'dns', fail_count: 3, fail_window: 180 })], 'T', 1)
  assert.ok(texts(dns).includes('The DNS check only fails if the lookup fails, not if the answer changes.'))
})

test('checks: no recipients is a neutral note, last', () => {
  const none = explainRule([cond({ fail_count: 3, fail_window: 180 })], 'T', 0)
  const last = none.checks[none.checks.length - 1]
  assert.deepEqual(last, { level: 'note', text: 'No one is ticked, so this target sends no email or Signal. The webhook, if it is switched on, still sends.' })
  const some = explainRule([cond({ fail_count: 3, fail_window: 180 })], 'T', 2)
  assert.ok(!some.checks.some((c) => c.level === 'note'))
})

test('fail count timeline', () => {
  const t = failCountTimeline(3, 60)
  assert.deepEqual(t.steps.map((s) => s.result), ['ok', 'fail', 'fail', 'ok', 'fail', 'fail', 'fail', 'fail'])
  assert.deepEqual(t.steps.map((s) => s.label), ['', '1', '2', 'reset', '1', '2', 'down', 'down'])
  assert.equal(t.summary, 'One or two failures in a row never alert. A real outage alerts on the third failed check, 2 to 3 minutes in.')
  assert.deepEqual(failCountTimeline(1, 60).steps.map((s) => s.label), ['', 'down', 'down', '', 'down', 'down', 'down', 'down'])
})

test('returns plain strings: a markup name comes back unchanged', () => {
  const r = explainRule([cond()], '<b>x</b>', 1)
  assert.equal(r.opening, '<b>x</b> counts as down when:')
  assert.equal(typeof r.groups[0].sentence, 'string')
})
