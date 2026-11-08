import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { openPage, startApp, stopApp } from './help-fixtures.mjs'

let app

// Rows like a real list: long names and hosts, a project, a location and several tags
const longTarget = n => ({
  id: `tl${n}`, name: `Distribution switch building ${n} east wing`, host: `switch-bld${n}-dist.corp.example.com`,
  description: '', enabled: true, preferred_check_type: 'ping', operator: 'AND', category: 'Key Services', rule_id: `r-tl${n}`,
  notes: null, contacts: null, project: 'ACME Headquarters', location: 'Datacenter 1',
  tags: ['production', 'network', 'tier-1'], condition_count: 2,
  state: { rule_id: `r-tl${n}`, current_state: 'healthy' }, primary_check_id: `c${n}`,
})
const fixtures = { '/api/targets': [1, 2, 3].map(longTarget) }
before(async () => { app = await startApp() })
after(() => stopApp(app))

// What the Actions cell of the first row looks like: the five buttons, the card they should sit in,
// the cell's and the row's first cell's tops, and how far the page scrolls sideways
async function layout(page) {
  const row = page.locator('tbody tr').first()
  await row.waitFor()
  const buttons = await row.locator('td:last-child .btn').evaluateAll(list => list.map(b => {
    const r = b.getBoundingClientRect()
    return { text: b.textContent.trim(), left: r.left, right: r.right, top: r.top }
  }))
  const tops = await row.evaluate(r => ({
    first: r.querySelector('td:first-child').getBoundingClientRect().top,
    actions: r.querySelector('td:last-child').getBoundingClientRect().top,
  }))
  const card = await page.locator('.card').first().boundingBox()
  const scroll = await page.evaluate(() => {
    const el = document.scrollingElement
    return { scrollWidth: el.scrollWidth, clientWidth: el.clientWidth }
  })
  return { buttons, card, tops, scroll }
}

// The page is capped at 1100 px, so the table has the same room in every window of 1200 px and more.
for (const [width, height] of [[1440, 900], [1280, 800], [1100, 800]]) {
  test(`Targets: at ${width} px the Actions buttons sit in their cell, inside the card, on at most two lines`, async t => {
    const { page } = await openPage(t, app, '/targets', { role: 'operator', fixtures, viewport: { width, height } })
    const { buttons, card, tops, scroll } = await layout(page)
    assert.deepEqual(buttons.map(b => b.text), ['Pause', 'Run Now', 'Edit', 'Clone', 'Delete'])
    assert.ok(scroll.scrollWidth <= scroll.clientWidth, `no sideways scroll (page ${scroll.scrollWidth} wide, window ${scroll.clientWidth})`)
    for (const b of buttons) {
      assert.ok(b.left >= card.x && b.right <= card.x + card.width,
        `${b.text} sits inside the card (right edge ${Math.round(b.right)}, card edge ${Math.round(card.x + card.width)})`)
    }
    assert.ok(new Set(buttons.map(b => Math.round(b.top))).size <= 2, 'at most two lines of buttons')
    assert.ok(Math.abs(tops.actions - tops.first) <= 2,
      `the Actions cell is in the grid, level with the row's first cell (${tops.actions} vs ${tops.first})`)
    if (process.env.TARGETS_SHOTS) await page.screenshot({ path: `${process.env.TARGETS_SHOTS}/targets-${width}.png` })
  })
}
