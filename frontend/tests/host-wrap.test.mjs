import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { openPage, startApp, stopApp } from './help-fixtures.mjs'

// A-082: a Host may carry a path, so long Hosts are normal. On Targets and Search
// the Host cell wraps a long one instead of pushing the table out of its card.

let app
const LONG = 'portal.example.com/a/rather/long/path/that/keeps/going/to/test/the/layout/index.html'
const row = (n, host) => ({
  id: `hw${n}`, name: `Web portal ${n}`, host,
  description: '', enabled: true, preferred_check_type: 'http', operator: 'AND', category: 'Key Services', rule_id: `r-hw${n}`,
  notes: null, contacts: null, project: 'ACME Headquarters', location: 'Datacenter 1',
  tags: ['production', 'web'], condition_count: 1,
  state: { rule_id: `r-hw${n}`, current_state: 'healthy' }, primary_check_id: `c-hw${n}`,
})
const fixtures = { '/api/targets': [row(1, LONG), row(2, 'portal.example.com'), row(3, 'portal.example.com/health/')] }
before(async () => { app = await startApp() })
after(() => stopApp(app))

// Where the long Host's cell and the row's last cell end, against the card, and the page's sideways scroll
async function measure(page) {
  const cell = page.locator('td', { hasText: LONG })
  await cell.waitFor()
  return page.evaluate(long => {
    const td = [...document.querySelectorAll('td')].find(c => c.textContent.trim() === long)
    const card = td.closest('.card').getBoundingClientRect()
    const last = td.closest('tr').lastElementChild.getBoundingClientRect()
    const el = document.scrollingElement
    return {
      cellRight: td.getBoundingClientRect().right, lastRight: last.right, cardRight: card.right,
      text: td.textContent.trim(), scrollWidth: el.scrollWidth, clientWidth: el.clientWidth,
    }
  }, LONG)
}

for (const [page, width] of [['/targets', 1440], ['/targets', 1280], ['/targets', 1100], ['/search', 1440], ['/search', 1100]]) {
  test(`${page}: at ${width} px a long Host wraps inside the card`, async t => {
    const { page: p } = await openPage(t, app, page, { role: 'operator', fixtures, viewport: { width, height: 900 } })
    if (page === '/search') await p.getByPlaceholder('Search by name, host, or IP...').fill('portal')
    const m = await measure(p)
    assert.equal(m.text, LONG, 'the whole Host is shown')
    assert.ok(m.scrollWidth <= m.clientWidth, `no sideways scroll (page ${m.scrollWidth} wide, window ${m.clientWidth})`)
    assert.ok(m.cellRight <= m.cardRight, `the Host cell ends inside the card (${Math.round(m.cellRight)} vs ${Math.round(m.cardRight)})`)
    assert.ok(m.lastRight <= m.cardRight + 0.5, `the row's last cell ends inside the card (${Math.round(m.lastRight)} vs ${Math.round(m.cardRight)})`)
  })
}
