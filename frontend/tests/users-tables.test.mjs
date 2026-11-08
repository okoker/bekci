import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { openPage, startApp, stopApp } from './help-fixtures.mjs'

let app

const DAY = 86_400_000
const ago = ms => new Date(Date.now() - ms).toISOString()
// A revoked token (empty Actions cell) between two active ones (a Revoke button each)
const fixtures = {
  '/api/api-tokens': [
    { id: 'k1', name: 'grafana', prefix: 'bk_a1b2c3d4', created_by: 'u-admin', created_at: ago(3 * DAY), last_used_at: ago(DAY) },
    { id: 'k2', name: 'old-zabbix', prefix: 'bk_e5f6a7b8', created_by: 'u-admin', created_at: ago(9 * DAY), revoked_at: ago(5 * DAY) },
    { id: 'k3', name: 'soar-integration', prefix: 'bk_c9d0e1f2', created_by: 'u-admin', created_at: ago(2 * DAY) },
  ],
}
before(async () => { app = await startApp() })
after(() => stopApp(app))

// Users › API Access: the Actions cell is a real table cell, so its top and bottom (and so the
// row line under it) match the row's other cells, whether it holds a Revoke button or nothing.
test('API Access: every token row\'s Actions cell lines up with the rest of its row', async t => {
  const { page } = await openPage(t, app, '/users', { role: 'admin', fixtures, viewport: { width: 1440, height: 900 } })
  await page.getByRole('button', { name: 'API Access', exact: true }).click()
  const rows = page.locator('tbody tr', { hasText: 'bk_' })
  await rows.first().waitFor()
  assert.equal(await rows.count(), 3)
  const cells = await rows.evaluateAll(list => list.map(tr => {
    const first = tr.querySelector('td:first-child').getBoundingClientRect()
    const last = tr.querySelector('td:last-child').getBoundingClientRect()
    return { name: tr.querySelector('td').textContent.trim(), top: [first.top, last.top], bottom: [first.bottom, last.bottom] }
  }))
  for (const c of cells) {
    assert.ok(Math.abs(c.top[1] - c.top[0]) <= 0.5, `${c.name}: Actions cell top ${c.top[1]} vs row ${c.top[0]}`)
    assert.ok(Math.abs(c.bottom[1] - c.bottom[0]) <= 0.5, `${c.name}: Actions cell bottom ${c.bottom[1]} vs row ${c.bottom[0]}`)
  }
  assert.equal(await rows.nth(0).locator('td:last-child button', { hasText: 'Revoke' }).count(), 1)
  assert.equal(await rows.nth(1).locator('td:last-child button').count(), 0)
})

test('Humans: every user row\'s Actions cell lines up with the rest of its row', async t => {
  const { page } = await openPage(t, app, '/users', { role: 'admin', viewport: { width: 1440, height: 900 } })
  const rows = page.locator('tbody tr').filter({ has: page.getByRole('button', { name: 'Edit', exact: true }) })
  await rows.first().waitFor()
  const cells = await rows.evaluateAll(list => list.map(tr => {
    const first = tr.querySelector('td:first-child').getBoundingClientRect()
    const last = tr.querySelector('td:last-child').getBoundingClientRect()
    return { name: tr.querySelector('td').textContent.trim(), top: [first.top, last.top], bottom: [first.bottom, last.bottom] }
  }))
  assert.ok(cells.length >= 2, `user rows on screen: ${cells.length}`)
  for (const c of cells) {
    assert.ok(Math.abs(c.top[1] - c.top[0]) <= 0.5, `${c.name}: Actions cell top ${c.top[1]} vs row ${c.top[0]}`)
    assert.ok(Math.abs(c.bottom[1] - c.bottom[0]) <= 0.5, `${c.name}: Actions cell bottom ${c.bottom[1]} vs row ${c.bottom[0]}`)
  }
})
