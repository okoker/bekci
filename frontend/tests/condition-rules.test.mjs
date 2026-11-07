// Pins the target form's pure validation rules (src/conditionRules.js). Node only.
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  getDefaultConfig, minFailWindow, failWindowError, valueError, packetLossHint,
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
