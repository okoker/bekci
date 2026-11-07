// The in-app help's anchors and its content agree, and the help code stays out
// of the main bundle (design in_app_help.md §4.3, §6). Node only, no browser.
//
// A view marks a documented control with data-help="<page>.<key>"; the note for
// it is pages[page].items[key] in src/help/content. Keys a view computes come
// from a map in src/help/anchors.js, exported as <page>Anchors.
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join, relative, resolve, sep } from 'node:path'
import { test } from 'node:test'
import { fileURLToPath } from 'node:url'

const src = fileURLToPath(new URL('../src/', import.meta.url))
const helpDir = join(src, 'help') + sep
const rel = file => relative(src, file)

function filesUnder(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    const path = join(dir, entry.name)
    return entry.isDirectory() ? filesUnder(path) : [path]
  })
}
const sources = filesUnder(src).filter(file => /\.(vue|js)$/.test(file))
const views = sources.filter(file => file.endsWith('.vue'))

// data-help="…" (static) and :data-help="…" / v-bind:data-help="…" (computed).
// data-help-page, data-help-args and the like are other attributes.
const DATA_HELP = /(?<=\s)(:|v-bind:)?data-help="([^"]*)"/g

// Every static anchor in the views, by id, with the files it is in.
function staticAnchors() {
  const found = new Map()
  for (const file of views) {
    for (const [, bound, value] of readFileSync(file, 'utf8').matchAll(DATA_HELP)) {
      if (bound) continue
      found.set(value, [...(found.get(value) ?? []), rel(file)])
    }
  }
  return found
}

// The maps in anchors.js: { <page>Anchors: { the view's name: key } }.
async function anchorMaps() {
  const module = await import('../src/help/anchors.js')
  return Object.entries(module).filter(([name]) => name.endsWith('Anchors'))
}

async function allAnchors() {
  const found = staticAnchors()
  for (const [name, map] of await anchorMaps()) {
    const page = name.slice(0, -'Anchors'.length)
    for (const key of Object.values(map)) {
      const id = `${page}.${key}`
      found.set(id, [...(found.get(id) ?? []), `anchors.js ${name}`])
    }
  }
  return found
}

async function contentIds() {
  const { pages } = await import('../src/help/content/index.js')
  return Object.entries(pages).flatMap(([page, { items }]) => Object.keys(items).map(key => `${page}.${key}`))
}

test('every help note has a ? anchor in a view, and every anchor has a note', async () => {
  const anchors = await allAnchors()
  const notes = await contentIds()
  assert.ok(anchors.size > 0, 'no data-help anchor found in src/**/*.vue: the scan is broken')
  assert.ok(notes.length > 0, 'no help notes found in src/help/content: the scan is broken')
  for (const id of anchors.keys()) assert.match(id, /^[a-zA-Z][a-zA-Z0-9]*\.[a-zA-Z0-9]+$/, `data-help="${id}" is not <page>.<key>`)
  assert.deepEqual(notes.filter(id => !anchors.has(id)), [], 'notes no view anchors')
  assert.deepEqual([...anchors].filter(([id]) => !notes.includes(id)).map(([id, files]) => `${id} (${files.join(', ')})`), [],
    'anchors with no note')
})

test('data-help values are static strings, except keys a view takes from a map in anchors.js', async () => {
  const maps = (await anchorMaps()).map(([name]) => name)
  const unmapped = []
  for (const file of views) {
    for (const [, bound, value] of readFileSync(file, 'utf8').matchAll(DATA_HELP)) {
      if (bound && !maps.some(name => new RegExp(`\\b${name}\\b`).test(value))) unmapped.push(`${rel(file)}: ${bound}data-help="${value}"`)
    }
  }
  assert.deepEqual(unmapped, [], 'computed data-help values must come from a <page>Anchors map in anchors.js')
})

// Static imports and re-exports: `import … from '…'`, `import '…'`, `export … from '…'`.
// Dynamic import() is how the rest of help loads on first use, so it isn't listed.
const STATIC_IMPORT = /^\s*(?:import|export)\s[^'"]*?\sfrom\s*['"]([^'"]+)['"]|^\s*import\s*['"]([^'"]+)['"]/gm
const staticImports = file => [...readFileSync(file, 'utf8').matchAll(STATIC_IMPORT)].map(m => m[1] ?? m[2])

// Outside src/help/, a file may import only these from it: the help state (the
// one help module in the main bundle) and anchors.js (plain data a view may need).
const IMPORTABLE = ['useHelp.js', 'anchors.js', 'applyConditions.js']

test('outside src/help/, only useHelp.js, anchors.js and applyConditions.js are imported from it; the rest loads on first use', () => {
  const offending = []
  for (const file of sources.filter(file => !file.startsWith(helpDir))) {
    for (const spec of staticImports(file)) {
      if (!spec.startsWith('.')) continue
      const target = resolve(dirname(file), spec)
      if (target.startsWith(helpDir) && !IMPORTABLE.includes(target.slice(helpDir.length))) {
        offending.push(`${rel(file)}: import '${spec}'`)
      }
    }
  }
  assert.deepEqual(offending, [], 'static imports from help/ that would pull help into the main bundle')
  // The two that are imported stay small: useHelp.js loads everything else with
  // import(), and anchors.js is plain data with no imports at all
  assert.deepEqual(staticImports(join(helpDir, 'useHelp.js')), ['vue'], 'useHelp.js static imports')
  assert.deepEqual(staticImports(join(helpDir, 'anchors.js')), [], 'anchors.js static imports')
})
