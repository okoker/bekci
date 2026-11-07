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

// ---- fix round 1 ----
const fc = (n, i = 60) => ({ fail_count: n, interval_s: i, fail_window: n > 1 ? n * i : 0 })

test('blank or missing fields never print "undefined" or a double space', () => {
  const one = (over) => sentence(explainRule([cond(over)], 'T', 1))
  assert.equal(one({ check_type: 'tcp', config: { port: '' }, ...fc(3) }), 'the port has refused connections 3 checks in a row')
  assert.ok(one({ check_type: 'tls_cert', config: {} }).includes('fewer than 30 days left'))
  assert.ok(one({ check_type: 'tls_cert', config: { warn_days: '' }, value: 'up' }).includes('at least 30 days left'))
  assert.ok(one({ check_type: 'http', config: { expect_status: 0 } }).includes('not answered with 0'))
  assert.ok(one({ check_type: 'http', config: { expect_status: '' } }).includes('not answered with 200'))
  assert.ok(one({ field: 'response_ms', comparator: 'gt', value: '' }).includes('taken longer than the time you set'))
  assert.ok(one({ field: 'metrics.packet_loss', comparator: 'gt', value: '' }).includes('lost more than the share you set of its pings'))
  for (const over of [{ check_type: 'tcp', config: { port: '' } }, { check_type: 'tls_cert', config: {} },
    { field: 'response_ms', comparator: 'gt', value: '' }, { field: 'status', comparator: 'neq', value: '' }]) {
    const s = one(over)
    assert.ok(!s.includes('undefined') && !s.includes('  ') && !s.includes('NaN'), s)
  }
})

// The nine rows of the spec's table, in order. Rows 2 and 3 exclude each other.
test('checks come out in the spec table order', () => {
  const rows = (ping) => [
    cond({ check_name: 'Ping', ...ping }, 0, 'AND'),
    cond({ check_type: 'http', ...fc(2) }, 0, 'AND'),
    cond({ check_type: 'tls_cert', ...fc(2, 600) }, 1, 'OR'),
    cond({ check_type: 'dns', ...fc(2) }, 1, 'OR'),
    cond({ check_type: 'page_hash', ...fc(2) }, 2),
    cond({ check_type: 'tcp', value: 'up', ...fc(2) }, 3),
    cond({ check_type: 'tcp', value: 'up', config: { port: 22, timeout_s: 5, expect_closed: true }, ...fc(2) }, 4),
  ]
  const tail = [
    { level: 'warn', text: 'Group 2 alerts when any one of its tests fails. If they should agree first, switch it to AND.' },
    { level: 'warn', text: 'Page Hash also fails when the site is simply down. Pair it with "web page answers" in an AND group (recipe: Defacement).' },
    { level: 'warn', text: "This alerts when the port opens, but the target's uptime and SLA will read close to 0%, because a closed port counts as down. Tick Port must stay closed and use Status equals down instead (recipe: A port that must stay closed)." },
    { level: 'warn', text: 'This alerts while the port is closed. To alert when it opens, use Status equals down.' },
    { level: 'warn', text: 'The DNS check only fails if the lookup fails, not if the answer changes.' },
    { level: 'note', text: 'No one is ticked, so this target sends no email or Signal. The webhook, if it is switched on, still sends.' },
  ]
  assert.deepEqual(explainRule(rows(fc(2)), 'T', 0).checks, [
    { level: 'ok', text: 'Group 1 needs two different tests to agree before it alerts.' },
    { level: 'ok', text: 'No test that can flicker alerts on a single failure.' },
    ...tail,
  ])
  // R14: the Fail Count 1 ping is AND'd with the web page, which confirms it
  assert.deepEqual(explainRule(rows(fc(1)), 'T', 0).checks, [
    { level: 'ok', text: 'Group 1 needs two different tests to agree before it alerts.' },
    { level: 'ok', text: 'No test that can flicker alerts on a single failure.' },
    ...tail,
  ])
})

test('DNS subject names the query and the nameserver; failure to resolve is not hidden', () => {
  const dns = (config, over = {}) => cond({ check_type: 'dns', config: { ...getDefaultConfig('dns'), ...config }, ...over })
  const s = sentence(explainRule([dns({ query: 'example.com', nameserver: '8.8.8.8', expect_value: '1.2.3.4' })], 'T', 1))
  assert.ok(s.startsWith('the DNS answer for example.com at 8.8.8.8 has given an address other than 1.2.3.4, or failed to resolve'))
  assert.ok(sentence(explainRule([dns({ expect_value: '1.2.3.4' })], 'T', 1)).startsWith('the DNS answer has given'))
  // S2 shape: two resolvers wrong AND the canary passing reads as separate lookups
  const s2 = sentence(explainRule([
    dns({ nameserver: '8.8.8.8', expect_value: '203.0.113.5' }, fc(2, 300)),
    dns({ nameserver: '1.1.1.1', expect_value: '203.0.113.5' }, fc(2, 300)),
    dns({ query: 'one.one.one.one', nameserver: '1.1.1.1', expect_value: '1.1.1.1' }, { value: 'up', ...fc(1, 300) }),
  ], 'T', 1))
  assert.ok(s2.includes('at 8.8.8.8') && s2.includes('at 1.1.1.1') && s2.includes('for one.one.one.one'), s2)
})

test('only Status equals up is exempt from the single-failure checks', () => {
  const r = explainRule([cond(fc(3)), cond({ field: 'metrics.packet_loss', comparator: 'gt', value: '30', check_name: 'Loss', ...fc(1) })], 'T', 1)
  assert.ok(!texts(r).includes('No test that can flicker alerts on a single failure.'))
  assert.ok(texts(r).includes('Loss alerts on a single failed check. One blip will page people; 3 in a row is safer.'))
  const rt = explainRule([cond({ field: 'response_ms', comparator: 'gt', value: '500', check_name: 'Slow', ...fc(1) })], 'T', 1)
  assert.ok(texts(rt).includes('Slow alerts on a single failed check. One blip will page people; 3 in a row is safer.'))
  // a canary that must pass (Status equals up, Fail Count 1) is neither warned about nor counted
  const canary = explainRule([cond(fc(2)), cond({ check_type: 'http', value: 'up', ...fc(1) })], 'T', 1)
  assert.ok(texts(canary).includes('No test that can flicker alerts on a single failure.'))
  assert.ok(!canary.checks.some((c) => c.level === 'warn'))
})

test('mixed intervals: AND = largest bounds, OR = smallest bounds', () => {
  const a = cond(fc(3, 60), 0, 'AND')
  const b = cond({ check_type: 'http', ...fc(2, 100) }, 0, 'AND')
  assert.equal(explainRule([a, b], 'T', 1).groups[0].delay, '120 to 200 seconds into an outage, at checks every 60 and 100 seconds')
  const a2 = { ...a, group_operator: 'OR' }
  const b2 = { ...b, group_operator: 'OR' }
  assert.equal(explainRule([a2, b2], 'T', 1).groups[0].delay, '100 to 180 seconds into an outage, at checks every 60 and 100 seconds')
  // a Fail Count 1 member counts as 0 to its interval
  const one = cond({ check_type: 'http', ...fc(1, 60) }, 0, 'AND')
  assert.equal(explainRule([cond(fc(3, 60), 0, 'AND'), one], 'T', 1).groups[0].delay, '2 to 3 minutes into an outage, at one check a minute')
  assert.equal(explainRule([cond(fc(1)), cond({ check_type: 'http', ...fc(1) })], 'T', 1).groups[0].delay, null)
})

test('ordinals in the timeline summary', () => {
  const nth = (n) => failCountTimeline(n, 60).summary
  for (const [n, w] of [[11, '11th'], [12, '12th'], [13, '13th'], [21, '21st'], [22, '22nd'], [23, '23rd'], [101, '101st'], [111, '111th'], [112, '112th'], [113, '113th']]) {
    assert.ok(nth(n).includes(`on the ${w} failed check`), `${n}: ${nth(n)}`)
  }
})

test('Status equals up reads right for every check type', () => {
  const one = (over) => sentence(explainRule([cond({ value: 'up', ...over })], 'T', 1))
  assert.equal(one({}), 'ping has answered on the latest check')
  assert.equal(one({ check_type: 'http' }), 'the web page has answered with 200 on the latest check')
  assert.equal(one({ check_type: 'tcp', config: { port: 25, expect_closed: false } }), 'port 25 has accepted a connection on the latest check')
  assert.equal(one({ check_type: 'dns', config: { ...getDefaultConfig('dns'), expect_value: '1.1.1.1' } }), 'the DNS answer has given 1.1.1.1 on the latest check')
  assert.equal(one({ check_type: 'dns' }), 'the DNS answer has resolved on the latest check')
  assert.equal(one({ check_type: 'page_hash' }), "the page's content has matched its baseline on the latest check")
  assert.equal(one({ check_type: 'tls_cert', config: { warn_days: 14 } }), 'the certificate has shown at least 14 days left on the latest check')
  assert.equal(one({ check_type: 'snmp_v3' }), 'SNMP has answered on the latest check')
})

test('interleaved groups: one sentence per group, last operator per group', () => {
  const r = explainRule([
    cond({ ...fc(3) }, 1, 'AND'),
    cond({ check_type: 'tls_cert', config: { warn_days: 14 } }, 2, 'AND'),
    cond({ check_type: 'http', ...fc(3) }, 1, 'OR'),
  ], 'T', 1)
  assert.equal(r.groups.length, 2)
  assert.equal(sentence(r, 0), 'any one of ping or the web page has failed 3 checks in a row')
  assert.ok(sentence(r, 1).startsWith('the certificate has shown fewer than 14 days left'))
})

// Ruling R14: a Fail Count 1 condition in an AND group with a test of another
// check type is confirmed by it (the pairing that earns the group's "two
// different tests" tick), so it is neither warned about nor counted against the
// "nothing flickers" tick. Everywhere else rows 2-3 apply as before.
const PING_WARN = (name) => `${name} alerts on a single failed check. One blip will page people; 3 in a row is safer.`
const NO_FLICKER = 'No test that can flicker alerts on a single failure.'

test('R14: a Fail Count 1 ping AND\'d with a web page on 2 in a row is confirmed by it, so no single-failure warning', () => {
  const r = explainRule([
    cond({ check_name: 'Ping', ...fc(1) }, 0, 'AND'),
    cond({ check_type: 'http', check_name: 'Web page', ...fc(2, 300) }, 0, 'AND'),
  ], 'T', 1)
  assert.deepEqual(r.checks, [
    { level: 'ok', text: 'Group 1 needs two different tests to agree before it alerts.' },
    { level: 'ok', text: NO_FLICKER },
  ])
})

test('R14 tightened: two AND\'d Fail Count 1 tests of different types do not confirm each other, so both are warned about', () => {
  const r = explainRule([
    cond({ check_name: 'Ping', ...fc(1) }, 0, 'AND'),
    cond({ check_type: 'http', check_name: 'Web page', ...fc(1, 300) }, 0, 'AND'),
  ], 'T', 1)
  assert.deepEqual(r.checks, [
    { level: 'ok', text: 'Group 1 needs two different tests to agree before it alerts.' },
    { level: 'warn', text: PING_WARN('Ping') },
    { level: 'warn', text: PING_WARN('Web page') },
  ])
  assert.ok(!texts(r).includes(NO_FLICKER))
  // a partner that needs 3 in a row confirms just as 2 does
  const three = explainRule([
    cond({ check_name: 'Ping', ...fc(1) }, 0, 'AND'),
    cond({ check_type: 'http', check_name: 'Web page', ...fc(3, 300) }, 0, 'AND'),
  ], 'T', 1)
  assert.ok(!three.checks.some((c) => c.level === 'warn'))
  assert.ok(texts(three).includes(NO_FLICKER))
})

test('R14: a Fail Count 1 ping alone, in an OR group, or in a group of its own is still warned about', () => {
  const alone = explainRule([cond({ check_name: 'Ping', ...fc(1) })], 'T', 1)
  assert.ok(texts(alone).includes(PING_WARN('Ping')))
  assert.ok(!texts(alone).includes(NO_FLICKER))
  const or = explainRule([
    cond({ check_name: 'Ping', ...fc(1) }, 0, 'OR'),
    cond({ check_type: 'http', ...fc(2, 300) }, 0, 'OR'),
  ], 'T', 1)
  assert.ok(texts(or).includes(PING_WARN('Ping')))
  assert.ok(!texts(or).includes(NO_FLICKER))
  const split = explainRule([cond({ check_name: 'Ping', ...fc(1) }, 0), cond({ check_type: 'http', ...fc(2, 300) }, 1)], 'T', 1)
  assert.ok(texts(split).includes(PING_WARN('Ping')))
  assert.ok(!texts(split).includes(NO_FLICKER))
})

test('R14: two pings AND\'d, both Fail Count 1, are the same test: both are warned about', () => {
  const r = explainRule([
    cond({ check_name: 'Ping A', ...fc(1) }, 0, 'AND'),
    cond({ check_name: 'Ping B', ...fc(1) }, 0, 'AND'),
  ], 'T', 1)
  assert.deepEqual(r.checks, [
    { level: 'warn', text: PING_WARN('Ping A') },
    { level: 'warn', text: PING_WARN('Ping B') },
  ])
})

test('the timeline uses the interval Bekci runs (10 to 86400 seconds) and gives the figures it used', () => {
  const t = failCountTimeline(3, 5)
  assert.deepEqual([t.failCount, t.interval], [3, 10])
  assert.ok(t.summary.endsWith('A real outage alerts on the third failed check, 20 to 30 seconds in.'), t.summary)
  assert.equal(failCountTimeline(2, 100000).interval, 86400)
  assert.equal(failCountTimeline(2, '').interval, 60)
  assert.deepEqual([failCountTimeline('', 60).failCount, failCountTimeline(0, 60).failCount, failCountTimeline(2.7, 60).failCount], [1, 1, 2])
  assert.deepEqual([failCountTimeline(3, 60).failCount, failCountTimeline(3, 60).interval], [3, 60])
})

test('R14 keeps the table order: the ✓ for two tests, then a single-failure warning that still stands, then the rest', () => {
  const r = explainRule([
    cond({ check_type: 'http', ...fc(2) }, 0, 'AND'),
    cond({ check_type: 'tls_cert', ...fc(2, 600) }, 0, 'AND'),
    cond({ check_name: 'Ping', ...fc(1) }, 1),
    cond({ check_type: 'dns', ...fc(2) }, 2, 'OR'),
    cond({ check_type: 'dns', check_name: 'DNS B', ...fc(2) }, 2, 'OR'),
  ], 'T', 0)
  assert.deepEqual(r.checks, [
    { level: 'ok', text: 'Group 1 needs two different tests to agree before it alerts.' },
    { level: 'warn', text: PING_WARN('Ping') },
    { level: 'warn', text: 'Group 3 alerts when any one of its tests fails. If they should agree first, switch it to AND.' },
    { level: 'warn', text: 'The DNS check only fails if the lookup fails, not if the answer changes.' },
    { level: 'note', text: 'No one is ticked, so this target sends no email or Signal. The webhook, if it is switched on, still sends.' },
  ])
})

// R14 as amended: only a failing test of another type confirms. A partner that
// must be passing (Status equals up) is the normal state, so it confirms nothing.
test('R14: Page Hash on the latest check AND\'d with "web page answers" is still warned about: a passing test confirms nothing', () => {
  const r = explainRule([
    cond({ check_type: 'page_hash', check_name: 'Page content', ...fc(1) }, 0, 'AND'),
    cond({ check_type: 'http', check_name: 'Web page answers', value: 'up', ...fc(2, 300) }, 0, 'AND'),
  ], 'T', 1)
  assert.ok(texts(r).includes(PING_WARN('Page content')), texts(r).join(' | '))
  assert.ok(!texts(r).includes(NO_FLICKER))
  // a failing partner of another type still confirms: ping on the latest check AND web page down 2 in a row
  const pingWeb = explainRule([cond({ check_name: 'Ping', ...fc(1) }, 0, 'AND'), cond({ check_type: 'http', ...fc(2, 300) }, 0, 'AND')], 'T', 1)
  assert.ok(!pingWeb.checks.some((c) => c.level === 'warn'))
  assert.ok(texts(pingWeb).includes(NO_FLICKER))
})

// ---- final review ----
// The card's delay uses the interval Bekci runs (checkInterval: 10 to 86400 seconds,
// 60 when blank), as the Fail Count note does, so the two never disagree.
test('the card\'s delay uses the interval Bekci runs, so it matches the Fail Count note', () => {
  const r = explainRule([cond(fc(3, 5))], 'T', 1)
  assert.equal(r.groups[0].delay, '20 to 30 seconds into an outage, at one check every 10 seconds')
  assert.ok(failCountTimeline(3, 5).summary.endsWith('A real outage alerts on the third failed check, 20 to 30 seconds in.'))
  // 5 and 10 seconds both run every 10, so the group reads as one shared phrase and one interval
  const and = explainRule([cond(fc(3, 5)), cond({ check_type: 'http', ...fc(3, 10) })], 'T', 1)
  assert.equal(sentence(and), 'ping and the web page have both failed 3 checks in a row')
  assert.equal(and.groups[0].delay, '20 to 30 seconds into an outage, at one check every 10 seconds')
  assert.equal(explainRule([cond({ ...fc(3), interval_s: '' })], 'T', 1).groups[0].delay,
    '2 to 3 minutes into an outage, at one check a minute')
})

// A "what happens" phrase that ends in a number (or N ms) or in an "or …" clause is
// kept apart from the count phrase, so the two don't run together. The pinned
// wordings ("ping has failed 3 checks in a row", "on the latest check") stay.
test('the count phrase stays apart from a phrase ending in a number or an "or" clause', () => {
  const one = (over) => sentence(explainRule([cond(over)], 'T', 1))
  const dns = (expect_value, over = {}) => one({ check_type: 'dns', config: { ...getDefaultConfig('dns'), expect_value }, ...over })
  assert.equal(one({ check_type: 'http', ...fc(3) }), 'the web page has not answered with 200 on 3 checks in a row')
  assert.equal(one({ check_type: 'http' }), 'the web page has not answered with 200 on the latest check')
  assert.equal(one({ check_type: 'tls_cert', config: { warn_days: 14 } }),
    'the certificate has shown fewer than 14 days left, or could not be checked, on the latest check')
  assert.equal(one({ check_type: 'tls_cert', config: { warn_days: 14 }, ...fc(3, 600) }),
    'the certificate has shown fewer than 14 days left, or could not be checked, on 3 checks in a row')
  assert.equal(one({ check_type: 'page_hash', ...fc(2, 300) }), "the page's content has changed, or could not be read, on 2 checks in a row")
  assert.equal(dns('1.2.3.4', fc(2)), 'the DNS answer has given an address other than 1.2.3.4, or failed to resolve, on 2 checks in a row')
  assert.equal(dns('1.1.1.1', { value: 'up', ...fc(2) }), 'the DNS answer has given 1.1.1.1 on 2 checks in a row')
  assert.equal(one({ field: 'response_ms', comparator: 'gt', value: '5000', ...fc(5) }), 'ping has taken longer than 5000 ms on 5 checks in a row')
  // Several conditions sharing a phrase that ends in a number
  assert.equal(sentence(explainRule([
    cond({ check_type: 'http', ...fc(3) }),
    cond({ check_type: 'http', config: { ...getDefaultConfig('http'), endpoint: '/shop' }, ...fc(3) }),
  ], 'T', 1)), 'the web page and the page /shop have both not answered with 200 on 3 checks in a row')
  // Unchanged: phrases that end in a word
  assert.equal(one(fc(3)), 'ping has failed 3 checks in a row')
  assert.equal(one({}), 'ping has failed on the latest check')
  assert.equal(one({ check_type: 'tcp', config: { port: 25, timeout_s: 5, expect_closed: false }, ...fc(3) }), 'port 25 has refused connections 3 checks in a row')
  assert.equal(sentence(explainRule([cond(fc(3)), cond({ check_type: 'http', ...fc(3) })], 'T', 1)),
    'ping and the web page have both failed 3 checks in a row')
})
