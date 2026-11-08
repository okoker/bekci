import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { openPage, startApp, stopApp } from './help-fixtures.mjs'

let app
before(async () => { app = await startApp() })
after(() => stopApp(app))

// Search for "Core", leaving the one row "Core switch" on screen
async function searchCore(t) {
  const { page, writes } = await openPage(t, app, '/search', { role: 'operator' })
  await page.getByPlaceholder('Search by name, host, or IP...').fill('core')
  const row = page.locator('tbody tr').filter({ hasText: 'Core switch' })
  await row.waitFor()
  return { page, writes, row }
}

// Presses a row button, saves the form that opens, and returns the green message
async function saveFrom(page, row, button) {
  await row.getByRole('button', { name: button, exact: true }).click()
  const modal = page.locator('.modal-card')
  // Wait for the target to load into the form: an empty form can't be saved
  await page.waitForFunction(() => document.querySelector('.modal-card input')?.value)
  // A clone starts with no host: fill it, as a person must
  if (button === 'Clone') await modal.getByPlaceholder('e.g. google.com').fill('clone.example.com')
  await modal.getByRole('button', { name: 'Save', exact: true }).click()
  const message = page.locator('.success-msg')
  await message.waitFor()
  return message.innerText()
}

test('Search: saving a Clone says the target was created', async t => {
  const { page, writes, row } = await searchCore(t)
  assert.equal(await saveFrom(page, row, 'Clone'), 'Target created')
  assert.ok(writes.some(w => w.method === 'POST' && w.path === '/api/targets'), 'the clone was sent as a new target')
})

test('Search: saving an Edit says the target was updated', async t => {
  const { page, writes, row } = await searchCore(t)
  assert.equal(await saveFrom(page, row, 'Edit'), 'Target updated')
  assert.ok(writes.some(w => w.method === 'PUT' && w.path === '/api/targets/t1'), 'the edit was sent as an update')
})

// How a button looks: the colours a person sees
const look = button => button.evaluate(el => {
  const s = getComputedStyle(el)
  return { color: s.color, border: s.borderTopColor }
})

test('Search: the Delete button is red, like the Delete button on Targets', async t => {
  const targets = await openPage(t, app, '/targets', { role: 'operator' })
  const reference = await look(targets.page.locator('tbody tr').filter({ hasText: 'Core switch' })
    .getByRole('button', { name: 'Delete', exact: true }))
  assert.equal(reference.color, 'rgb(153, 27, 27)', 'reference: Targets shows a red Delete')

  const { page, row } = await searchCore(t)
  assert.deepEqual(await look(row.getByRole('button', { name: 'Delete', exact: true })), reference, 'row Delete')
  await row.getByRole('button', { name: 'Delete', exact: true }).click()
  const confirm = page.locator('.modal-card').getByRole('button', { name: 'Delete', exact: true })
  assert.deepEqual(await look(confirm), reference, 'Delete in the confirm dialog')
})

// The Actions cell is a real table cell: level with its row's other cells (so the row lines
// under it line up), its five buttons inside the card, and no sideways scroll
for (const [width, height] of [[1440, 900], [1100, 800]]) {
  test(`Search: at ${width} px the Actions cell lines up with its row and its buttons stay inside the card`, async t => {
    const { page } = await openPage(t, app, '/search', { role: 'operator', viewport: { width, height } })
    await page.getByPlaceholder('Search by name, host, or IP...').fill('core')
    const row = page.locator('tbody tr').filter({ hasText: 'Core switch' })
    await row.waitFor()
    const m = await row.evaluate(tr => {
      const tds = [...tr.querySelectorAll(':scope > td')]
      const first = tds[0].getBoundingClientRect(), last = tds[tds.length - 1].getBoundingClientRect()
      const card = tr.closest('.card').getBoundingClientRect()
      const buttons = [...tds[tds.length - 1].querySelectorAll('button')].map(b => b.getBoundingClientRect())
      const el = document.scrollingElement
      return { top: [first.top, last.top], bottom: [first.bottom, last.bottom], cardLeft: card.left, cardRight: card.right,
        buttons: buttons.map(b => [b.left, b.right]), sideways: el.scrollWidth > el.clientWidth }
    })
    assert.equal(m.buttons.length, 5)
    assert.ok(Math.abs(m.top[1] - m.top[0]) <= 0.5, `Actions cell top ${m.top[1]} vs row ${m.top[0]}`)
    assert.ok(Math.abs(m.bottom[1] - m.bottom[0]) <= 0.5, `Actions cell bottom ${m.bottom[1]} vs row ${m.bottom[0]}`)
    for (const [l, r] of m.buttons) assert.ok(l >= m.cardLeft && r <= m.cardRight, `button ${l}-${r} inside the card ${m.cardLeft}-${m.cardRight}`)
    assert.ok(!m.sideways, 'no sideways scroll')
  })
}
