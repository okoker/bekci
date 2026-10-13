import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright-core'
import { createServer } from 'vite'
import vue from '@vitejs/plugin-vue'

let server, browser, baseURL
before(async () => {
  // No backend proxy: every API request is fulfilled by the browser fixture.
  server = await createServer({
    configFile: false, root: fileURLToPath(new URL('../', import.meta.url)),
    plugins: [vue()], logLevel: 'error', server: { host: '127.0.0.1', port: 0 },
  })
  await server.listen()
  baseURL = server.resolvedUrls.local[0]
  browser = await chromium.launch(process.env.CHROME_PATH
    ? { executablePath: process.env.CHROME_PATH }
    : { channel: 'chrome' })
})
after(async () => {
  await browser?.close()
  await server?.close()
})

const autoSaved = {
  filename: 'bekci-full-20261013-035043.tar.gz', sha256: 'fb34ee5e5a0c42ed',
  size: 93714385, created_at: '2026-10-13T00:50:43Z', encrypted: false,
}

// Opens Settings > Backup & Restore > Full Database Backup. The server saves a
// copy of every download, so the saved list holds that copy once the download
// request has been made, whatever happened to the download itself.
async function backupCard(t, fulfillDownload) {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1100 } })
  const browserErrors = []
  page.on('pageerror', error => browserErrors.push(error.message))
  t.after(async () => {
    await page.close()
    assert.deepEqual(browserErrors, [], 'browser JavaScript errors')
  })
  page.setDefaultTimeout(3000)
  let downloadRequested = false
  await page.route(`${baseURL}api/**`, async route => {
    const request = route.request()
    const url = new URL(request.url())
    if (request.method() === 'POST' && url.pathname === '/api/backup/full') {
      downloadRequested = true
      return fulfillDownload(route)
    }
    if (request.method() !== 'GET') return route.fulfill({ status: 404, json: { error: 'Unexpected fixture request' } })
    let json
    if (url.pathname === '/api/me') json = { id: 'admin', username: 'admin', role: 'admin' }
    else if (url.pathname === '/api/backup/full/list') json = downloadRequested ? [autoSaved] : []
    else if (url.pathname === '/api/tags' || url.pathname === '/api/users') json = []
    else if (url.pathname === '/api/system/health') json = {
      net: { status: 'ok', latency_ms: 1 }, disk: { total_gb: 100, free_gb: 80 },
      cpu: { load1: 0, num_cpu: 4 }, scheduler: { status: 'ok', active_checks: 0, stale_seconds: 0 },
    }
    else json = {}
    await route.fulfill({ json })
  })
  await page.goto(`${baseURL}settings`)
  await page.getByRole('button', { name: 'Backup & Restore' }).click()
  await page.getByRole('heading', { name: 'Full Database Backup' }).click()
  await page.getByText('No saved backups').waitFor()
  return page
}

test('a failed download still shows the copy the server saved', async t => {
  const page = await backupCard(t, route => route.abort('connectionreset'))
  await page.getByRole('button', { name: 'Download Backup' }).click()
  await page.getByText('Full backup failed').waitFor()
  await page.getByText(autoSaved.filename).waitFor()
})

test('a completed download shows the copy the server saved', async t => {
  const page = await backupCard(t, route => route.fulfill({
    status: 200, contentType: 'application/octet-stream', body: Buffer.from('archive'),
    headers: { 'Content-Disposition': `attachment; filename="${autoSaved.filename}"` },
  }))
  const download = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Download Backup' }).click()
  await download
  await page.getByText(autoSaved.filename).waitFor()
})
