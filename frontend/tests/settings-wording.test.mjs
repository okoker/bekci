import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { openPage, startApp, stopApp } from './help-fixtures.mjs'

let app
before(async () => { app = await startApp() })
after(() => stopApp(app))

const tab = (page, name) => page.locator('.tabs').getByRole('button', { name, exact: true })

// A-080 (b): UP and DOWN are the target's alert rule state (what the Dashboard and SOC wall show),
// not the result of one check, so the badge guide must not say so.
test('Settings › SLA: the badge guide says UP/DOWN follow the alert rule, not the latest single check', async t => {
  const { page } = await openPage(t, app, '/settings', { role: 'admin' })
  await tab(page, 'SLA').click()
  await page.getByRole('button', { name: 'Badge guide' }).click()
  const meaning = badge => page.locator('.sla-info-table tbody tr').filter({ has: page.locator('.sla-badge-example', { hasText: new RegExp(`^${badge}$`) }) })
    .locator('td').nth(1).innerText()
  const up = await meaning('UP')
  const down = await meaning('DOWN')
  const note = await page.locator('.sla-info-note').innerText()
  assert.match(up, /alert conditions are not met/)
  assert.match(up, /Fail Count/)
  assert.match(down, /alert conditions are met/)
  assert.match(down, /Fail Count/)
  for (const text of [up, down, note]) {
    assert.doesNotMatch(text, /most recent check|state of a single check/, 'no claim that one check decides UP/DOWN')
  }
  assert.match(note, /alert rule/)
  // Unchanged: the 90-day badges
  assert.match(await meaning('HEALTHY'), /90-day uptime percentage meets or exceeds/)
  assert.match(await meaning('UNHEALTHY'), /dropped below/)
})

// A-080 (d): "1 events" read wrong
for (const [total, words] of [[1, '1 event'], [2, '2 events'], [0, '0 events']]) {
  test(`Settings › Audit Log: a total of ${total} reads "${words}"`, async t => {
    const { page } = await openPage(t, app, '/audit-log', { role: 'admin', fixtures: {
      '/api/audit-log': { entries: [], total, page: 1, limit: 50 },
    } })
    const header = page.locator('.audit-header [data-help="auditLog.audit"]')
    await header.waitFor()
    assert.equal((await header.innerText()).trim(), words)
  })
}
