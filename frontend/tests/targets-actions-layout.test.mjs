import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { openPage, startApp, stopApp } from './help-fixtures.mjs'

let app

// Rows like a real list: long names and hosts, a project, a location and several tags
const longTarget = n => ({
  id: `tl${n}`, name: `Distribution switch ${n}`, host: `switch-bld${n}-dist.corp.example.com`,
  description: '', enabled: true, preferred_check_type: 'ping', operator: 'AND', category: 'Key Services', rule_id: `r-tl${n}`,
  notes: null, contacts: null, project: 'ACME', location: 'DC-1',
  tags: ['production', 'tier-1'], condition_count: 2,
  state: { rule_id: `r-tl${n}`, current_state: 'healthy' }, primary_check_id: `c${n}`,
})
const fixtures = { '/api/targets': [1, 2, 3].map(longTarget) }
before(async () => { app = await startApp() })
after(() => stopApp(app))

// Where the five action buttons of the first target's row are, the card they
// should sit in, and how far the page scrolls sideways
async function layout(page) {
  const row = page.locator('tbody tr').first()
  await row.waitFor()
  const buttons = await row.locator('td.actions .btn').evaluateAll(list => list.map(b => {
    const r = b.getBoundingClientRect()
    return { text: b.textContent.trim(), left: r.left, right: r.right, top: r.top, bottom: r.bottom }
  }))
  const card = await page.locator('.card').first().boundingBox()
  const page_ = await page.evaluate(() => {
    const el = document.scrollingElement
    return { scrollWidth: el.scrollWidth, clientWidth: el.clientWidth }
  })
  return { buttons, card, page: page_ }
}

test('Targets: at 1100 px the Actions buttons wrap inside the card and the page does not scroll sideways', async t => {
  const { page } = await openPage(t, app, '/targets', { role: 'operator', fixtures, viewport: { width: 1100, height: 800 } })
  const { buttons, card, page: scroll } = await layout(page)
  assert.deepEqual(buttons.map(b => b.text), ['Pause', 'Run Now', 'Edit', 'Clone', 'Delete'])
  assert.ok(scroll.scrollWidth <= scroll.clientWidth, `no sideways scroll (page ${scroll.scrollWidth} wide, window ${scroll.clientWidth})`)
  for (const b of buttons) {
    assert.ok(b.left >= card.x && b.right <= card.x + card.width, `${b.text} sits inside the card (${Math.round(b.right)} vs card edge ${Math.round(card.x + card.width)})`)
  }
})

test('Targets: at 1440 px the five Actions buttons stay on one line', async t => {
  const { page } = await openPage(t, app, '/targets', { role: 'operator', fixtures, viewport: { width: 1440, height: 900 } })
  const { buttons, card, page: scroll } = await layout(page)
  assert.equal(buttons.length, 5)
  assert.equal(new Set(buttons.map(b => Math.round(b.top))).size, 1, 'one line')
  assert.ok(scroll.scrollWidth <= scroll.clientWidth, 'no sideways scroll')
  assert.ok(buttons.at(-1).right <= card.x + card.width, 'inside the card')
})
