// Pins the target form's pure validation rules (src/conditionRules.js). Node only.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import {
  getDefaultConfig, minFailWindow, failWindowError, valueError, packetLossHint,
  hostError, buildHTTPURL, checkURL,
} from '../src/conditionRules.js'

test('tcp default config', () => {
  assert.deepEqual(getDefaultConfig('tcp'), { port: 80, timeout_s: 5, expect_closed: false })
})

test('minFailWindow is count x interval, or 0 for a single failure', () => {
  assert.equal(minFailWindow({ fail_count: 3, interval_s: 60 }), 180)
  assert.equal(minFailWindow({ fail_count: 1, interval_s: 60 }), 0)
})

test('failWindowError names the required window', () => {
  assert.ok(failWindowError({ fail_count: 31, interval_s: 60 })
    .startsWith('These settings require a 31-minute failure window.'))
})

test('valueError checks number fields', () => {
  assert.equal(valueError({ field: 'metrics.packet_loss', value: '150' }),
    'Enter a percentage from 0 to 100, such as 30.')
  assert.equal(valueError({ field: 'status', value: 'up' }), '')
})

test('packetLossHint explains the step', () => {
  assert.ok(packetLossHint({ config: { count: 10 }, comparator: 'gt', value: '30' })
    .includes('4 or more of the 10 lost'))
})

// A-082: the form's Host check and address line mirror the server
// (internal/checker/host.go, url.go). Both read the same tables, so they cannot drift.
const sharedTable = (name) => JSON.parse(readFileSync(
  new URL(`../../internal/checker/testdata/${name}`, import.meta.url), 'utf8')).cases

test('hostError matches the server for every shared Host case', () => {
  const cases = sharedTable('host_rules.json')
  assert.ok(cases.length > 0)
  for (const c of cases) assert.equal(hostError(c.host), c.error, `host ${JSON.stringify(c.host)}`)
})

test('hostError trims like the server before checking', () => {
  assert.equal(hostError('  www.somehost.com/health/  '), '')
  assert.equal(hostError(' https://example.com '), hostError('https://example.com'))
})

test('buildHTTPURL matches the server for every shared URL case', () => {
  const cases = sharedTable('http_urls.json')
  assert.ok(cases.length > 0)
  for (const c of cases) {
    assert.equal(buildHTTPURL(c.scheme, c.host, c.port, c.endpoint), c.want,
      `${c.scheme} ${c.host} ${c.port} ${JSON.stringify(c.endpoint)}`)
  }
})

test('checkURL applies the check defaults the server applies', () => {
  // Missing or empty scheme is https, empty Endpoint is "/", a blank port is 0.
  assert.equal(checkURL({}, 'www.somehost.com/health'), 'https://www.somehost.com/health')
  assert.equal(checkURL({ scheme: '', endpoint: '', port: '' }, 'example.com'), 'https://example.com/')
  assert.equal(checkURL({ scheme: 'http', port: 8080, endpoint: 'status' }, ' example.com/app/ '),
    'http://example.com:8080/app/status')
  assert.equal(checkURL({ port: 8443.9 }, 'example.com'), 'https://example.com:8443/')
  // No address while the Host is empty or refused, or the scheme cannot run.
  assert.equal(checkURL({}, ''), '')
  assert.equal(checkURL({}, 'https://example.com'), '')
  assert.equal(checkURL({ scheme: 'ftp' }, 'example.com'), '')
})

test('hostError and checkURL trim exactly what the server trims (Unicode White_Space)', () => {
  // Go's strings.TrimSpace trims NEL (U+0085) and no-break space, but not a BOM (U+FEFF)
  assert.equal(hostError('\u0085example.com '), '')
  assert.equal(checkURL({}, '\u0085example.com '), 'https://example.com/')
  assert.equal(hostError('﻿example.com'), "Host can't contain spaces or invisible characters.")
  assert.equal(checkURL({}, '﻿example.com'), '')
})
