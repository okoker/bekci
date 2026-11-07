// What the help layer reads from the page (design in_app_help.md §4.3-4.4):
// which documented controls are on screen, where, and where a note fits beside
// one. It reads the page and never changes it. Only HelpLayer.vue imports it,
// so it loads with the layer, never in the main bundle.
import { HEADER_PAGE } from './anchors.js'

export const MARK = 18   // a ? mark's diameter
const EDGE = 12          // a note keeps this far from the window's edges
const GAP = 28           // from a control to its note: room for the mark and the leader line
const NOTE_WIDTH = 400   // a note's width where there is room (HelpLayer.vue's CSS)
const MARGIN_MIN = 240   // a margin narrower than this holds no note: it opens beside its control
const RUN = 11           // a margin note's line runs this far under its field

// The scope on screen: the last element in document order that names one
// (anchors.js explains why the last).
export function findScope() {
  const scopes = document.querySelectorAll('[data-help-scope]')
  return scopes[scopes.length - 1] ?? null
}

// A control is shown when it has a box, isn't made invisible, and no ancestor
// that cuts off its overflow (hidden or clip) leaves nothing of it. A section
// that scrolls hides nothing: its content can be scrolled to. A collapsed
// section (.collapsible-body without .open) counts as hidden at once, before
// its closing animation has shrunk it.
export function isShown(el) {
  const box = el.getBoundingClientRect()
  if (box.width <= 0 || box.height <= 0) return false
  if (getComputedStyle(el).visibility !== 'visible') return false
  for (let inner = el.closest('.collapsible-inner'); inner; inner = inner.parentElement?.closest('.collapsible-inner')) {
    if (!inner.parentElement?.classList.contains('open')) return false
  }
  let { left, top, right, bottom } = box
  for (let a = el.parentElement; a && a !== document.documentElement; a = a.parentElement) {
    const style = getComputedStyle(a)
    const clipsX = style.overflowX === 'hidden' || style.overflowX === 'clip'
    const clipsY = style.overflowY === 'hidden' || style.overflowY === 'clip'
    if (!clipsX && !clipsY) continue
    const outer = a.getBoundingClientRect()
    if (clipsX) {
      left = Math.max(left, outer.left + a.clientLeft)
      right = Math.min(right, outer.left + a.clientLeft + a.clientWidth)
    }
    if (clipsY) {
      top = Math.max(top, outer.top + a.clientTop)
      bottom = Math.min(bottom, outer.top + a.clientTop + a.clientHeight)
    }
    if (right <= left || bottom <= top) return false
  }
  return true
}

// The documented controls shown in `scope` for `page` (and the header), each
// key once, in order: the page's keys in document order, then the header's.
// For each: { id, page, key, note, els }, where els is the first shown
// instance, or every shown instance when the note says `each`.
export function findAnchors(scope, page, pages) {
  const found = new Map()
  for (const el of scope.querySelectorAll('[data-help]')) {
    const id = el.dataset.help
    const [p, key] = id.split('.')
    if (p !== page && p !== HEADER_PAGE) continue
    const items = Object.hasOwn(pages, p) ? pages[p].items : {}
    const note = Object.hasOwn(items, key) ? items[key] : null
    if (!note) continue
    const known = found.get(id)
    if (known && !note.each) continue
    if (!isShown(el)) continue
    if (known) known.els.push(el)
    else found.set(id, { id, page: p, key, note, els: [el] })
  }
  const all = [...found.values()]
  return [...all.filter(a => a.page !== HEADER_PAGE), ...all.filter(a => a.page === HEADER_PAGE)]
}

// The box a mark goes beside and a leader line points at: the control's, or
// with `box: 'content'` the box around its text and children, or for a form
// field (`box: 'label'`) the box around its label's text and its inputs, as
// narrow as the input may be. (A note keeps clear of the control's whole box,
// which is the part outlined.)
export function anchorBox(el, note) {
  if (note.box === 'label') {
    const parts = [labelBox(el), ...[...el.querySelectorAll('input, select, textarea')].map(c => c.getBoundingClientRect())]
      .filter(r => r && r.width > 0 && r.height > 0)
    if (parts.length) {
      const left = Math.min(...parts.map(r => r.left))
      const top = Math.min(...parts.map(r => r.top))
      return new DOMRect(left, top, Math.max(...parts.map(r => r.right)) - left, Math.max(...parts.map(r => r.bottom)) - top)
    }
  }
  if (note.box === 'content') {
    const range = document.createRange()
    range.selectNodeContents(el)
    const box = range.getBoundingClientRect()
    if (box.width > 0 && box.height > 0) return box
  }
  return el.getBoundingClientRect()
}

// The box a mark goes beside: the control's (anchorBox), or with `box: 'label'`
// (a form field) the text of its label, so the mark sits just past it.
export function markBox(el, note) {
  return (note.box === 'label' && labelBox(el)) || anchorBox(el, note)
}

// The text of a form field's first label, including what the app's CSS adds
// after it (the " *" of a required field); null without one
function labelBox(el) {
  const label = el.querySelector('label')
  if (!label) return null
  const range = document.createRange()
  range.selectNodeContents(label)
  const box = range.getBoundingClientRect()
  return box.width > 0 && box.height > 0 ? new DOMRect(box.left, box.top, box.width + afterWidth(label), box.height) : null
}

// The width of an element's ::after text (a pseudo-element has no box to measure)
let pen = null
function afterWidth(el) {
  const style = getComputedStyle(el, '::after')
  const text = /^"(.*)"$/.exec(style.content)?.[1]
  if (!text) return 0
  pen ??= document.createElement('canvas').getContext('2d')
  pen.font = `${style.fontStyle} ${style.fontWeight} ${style.fontSize} ${style.fontFamily}`
  return pen.measureText(text).width
}

// The mark's top-left corner, in window coordinates. It stays inside the
// window while its control does.
export function markPosition(box, place) {
  let x = box.right + 6
  let y = box.top + box.height / 2 - MARK / 2
  if (place === 'left') x = box.left - MARK - 6
  if (place === 'corner') {
    x = box.right - 8
    y = box.top - 13
  }
  const width = document.documentElement.clientWidth
  if (box.right > 0 && box.left < width) x = Math.min(Math.max(x, 2), width - MARK - 2)
  if (box.top >= 0) y = Math.max(y, 2)
  return { x, y }
}

const clamp = (value, low, high) => Math.min(Math.max(value, low), Math.max(low, high))

// What a note must never cover, in the scope on screen: help's own bar (in the
// layout, and in the target form's overlay its copy), whose ‹, › and Done must
// stay clickable while a note is open. A leader line may cross it.
export function keepClear(scope) {
  return [...(scope?.querySelectorAll('.help-bar') ?? [])]
    .map(el => el.getBoundingClientRect())
    .filter(r => r.width > 0 && r.height > 0)
}

// Where a note of `width` × `height` goes beside the control's `box`: below it,
// right of it, above it, or left of it, the first that fits in the window
// without covering anything in `avoid` (keepClear). A note that would cover
// one moves past it, down (or up, above the control). None covers the control.
// If none fits whole, the roomier of below and above, with the note scrolling
// inside (maxHeight). The note lines up with `aim` (anchorBox), the control's
// content for a full-width bar.
export function placeNote(box, width, height, aim = box, avoid = []) {
  const view = { width: document.documentElement.clientWidth, height: document.documentElement.clientHeight }
  const x = clamp(aim.left - 20, EDGE, view.width - width - EDGE)
  const y = clamp(aim.top + aim.height / 2 - 40, EDGE, view.height - height - EDGE)
  const covered = (t, h) => avoid.find(r => t.left < r.right && r.left < t.left + width && t.top < r.bottom && r.top < t.top + h)
  const clear = (t, h, up) => {
    for (let r = covered(t, h), n = 0; r && n < avoid.length; r = covered(t, h), n++) {
      t = { ...t, top: up ? r.top - EDGE - h : r.bottom + EDGE }
    }
    return t
  }
  const tries = [
    clear({ side: 'below', left: x, top: box.bottom + GAP }, height),
    clear({ side: 'right', left: box.right + GAP, top: y }, height),
    clear({ side: 'above', left: x, top: box.top - GAP - height }, height, true),
    clear({ side: 'left', left: box.left - GAP - width, top: y }, height),
  ]
  const fits = t => t.left >= EDGE && t.top >= EDGE && t.left + width <= view.width - EDGE
    && t.top + height <= view.height - EDGE && !covered(t, height)
  const fit = tries.find(fits)
  if (fit) return { ...fit, maxHeight: null }
  const belowTop = clear({ left: x, top: box.bottom + GAP }, 1).top
  const aboveBottom = clear({ left: x, top: box.top - GAP - 1 }, 1, true).top + 1
  const below = view.height - EDGE - belowTop
  const above = aboveBottom - EDGE
  if (below >= above) return { side: 'below', left: x, top: belowTop, maxHeight: Math.max(below, 120) }
  const maxHeight = Math.max(above, 120)
  return { side: 'above', left: x, top: aboveBottom - maxHeight, maxHeight }
}

// The target form at full width keeps the right margin for notes (design §4.4):
// right of the element marked data-help-margin (the form's card). The slot a
// note takes there, { left, width }, or null with no margin or too narrow a one.
export function marginSlot(scope) {
  const el = scope?.querySelector('[data-help-margin]')
  const box = el?.getBoundingClientRect()
  if (!box || box.width <= 0) return null
  const left = box.right + GAP
  const width = Math.floor(Math.min(NOTE_WIDTH, document.documentElement.clientWidth - EDGE - left))
  return width >= MARGIN_MIN ? { left, width } : null
}

// A note of `height` in the margin `slot`: a little above the line that runs
// under the control's `box`, inside the window and clear of `avoid` (keepClear),
// scrolling inside (maxHeight) when it doesn't fit. It can't cover the control:
// the margin is beside the form.
export function placeInMargin(box, slot, height, avoid = []) {
  const view = document.documentElement.clientHeight
  let top = clamp(box.bottom + RUN - 90, EDGE, view - height - EDGE)
  for (let n = 0; n < avoid.length; n++) {
    const r = avoid.find(r => slot.left < r.right && r.left < slot.left + slot.width && top < r.bottom && r.top < top + height)
    if (!r) break
    top = r.bottom + EDGE
  }
  const room = view - EDGE - top
  return { side: 'margin', left: slot.left, top, maxHeight: height > room ? Math.max(room, 120) : null }
}

// The leader line from the note (`rect`, on `side` of the control) to the
// control's `box`, ending in a dot on its edge, level with the middle of `aim`
// (anchorBox: the control's content, for a full-width bar). Its points, the
// last one the dot. From the margin it runs under the field and turns up into
// it at a right angle, so it never crosses an input; where the note sits lower
// or higher than that run, it first steps across in the gap beside the note.
export function leaderLine(side, rect, box, aim = box) {
  const cx = aim.left + aim.width / 2
  const cy = aim.top + aim.height / 2
  const alongX = clamp(cx, rect.left + 24, rect.right - 24)
  const alongY = clamp(cy, rect.top + 20, rect.bottom - 20)
  if (side === 'below') return [[alongX, rect.top], [cx, box.bottom + 4]]
  if (side === 'above') return [[alongX, rect.bottom], [cx, box.top - 4]]
  if (side === 'right') return [[rect.left, alongY], [box.right + 4, cy]]
  if (side === 'left') return [[rect.right, alongY], [box.left - 4, cy]]
  const run = box.bottom + RUN
  const end = [cx, box.bottom + 4]
  const start = clamp(run, rect.top + 20, rect.bottom - 20)
  if (start === run) return [[rect.left, run], [cx, run], end]
  const step = rect.left - GAP / 2
  return [[rect.left, start], [step, start], [step, run], [cx, run], end]
}

// The parsed data-help-args of a control: what a note written as a function
// of its control needs (the target form's Fail Count and Time Window).
export function argsOf(el) {
  try {
    return JSON.parse(el?.dataset.helpArgs || '{}')
  } catch {
    return {}
  }
}
