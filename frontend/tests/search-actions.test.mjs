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
