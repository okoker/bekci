<script setup>
// The help layer (design in_app_help.md §4.3-4.4): a ? mark beside each
// documented control on screen, and the note a mark opens, with a leader line
// to the control and the control outlined. Teleported to <body> and drawn in
// window coordinates, so turning help on never moves the page. It is the one
// host for help drawn over the page: later tasks add the recipes window and the
// rule lesson here.
//
// Marks follow their controls: the layer re-measures on scroll (any element's,
// caught on the way down), window resize, size changes (ResizeObserver) and DOM
// changes (MutationObserver), at most once per frame. A scroll keeps an open
// note where it was relative to its control; anything else places it afresh.
import { computed, nextTick, onMounted, onUnmounted, ref, shallowRef, watch } from 'vue'
import { help, openNote, closeNote, stepNote } from './useHelp.js'
import { pages } from './content/index.js'
import { anchorBox, argsOf, findAnchors, findScope, keepClear, leaderLine, markPosition, placeNote } from './locate.js'

// The windows a note's link can open, by name. A link shows only once its
// window is listed here: the rule lesson and the recipes come in later tasks.
const LINKS = {}

const root = ref(null)
const noteEl = ref(null)
const marks = shallowRef([])       // [{ uid, id, n, title, x, y }]
const noteAt = shallowRef(null)    // { left, top, maxHeight } once the open note is placed
const line = shallowRef(null)      // { x1, y1, x2, y2 } from the note to its control
const noteArgs = shallowRef({})    // the open note's control's data-help-args
const openMark = shallowRef('')    // the uid of the mark whose note is open

let anchors = []        // the documented controls on screen (locate.js findAnchors)
let outlined = null     // the open note's control, outlined
let placement = null    // where the open note sits: { side, dx, dy, maxHeight } from its control
let pointerClosed = false
let focusNote = false   // move focus into the note once it is placed (a hidden note can't take it)
let argsText = ''       // noteArgs as the attribute read it, to notice a change

const note = computed(() => {
  if (!help.current) return null
  const [page, key] = help.current.id.split('.')
  const item = pages[page]?.items[key]
  if (!item) return null
  const value = field => (typeof item[field] === 'function' ? item[field](noteArgs.value) : item[field])
  return {
    title: item.title,
    text: [].concat(value('text') ?? []),
    rows: value('rows') ?? [],
    tip: value('tip'),
    link: LINKS[item.link],
  }
})
const at = computed(() => help.items.findIndex(item => item.id === help.current?.id))
const count = computed(() => help.items.length)
const previous = computed(() => help.items[(at.value - 1 + count.value) % count.value]?.title)
const next = computed(() => help.items[(at.value + 1) % count.value]?.title)
const isOpen = m => m.uid === openMark.value

// ── Measuring ────────────────────────────────────────────────────────────────

let frame = 0
let relayout = false

function schedule(event) {
  if (event?.type !== 'scroll') relayout = true
  if (!frame) frame = requestAnimationFrame(refresh)
}

function refresh() {
  frame = 0
  const keep = !relayout
  relayout = false
  const scope = findScope()
  anchors = scope ? findAnchors(scope, help.page, pages) : []
  if (anchors.map(a => a.id).join() !== help.items.map(item => item.id).join()) {
    help.items = anchors.map(a => ({ id: a.id, title: a.note.title }))
  }
  marks.value = anchors.flatMap(a => a.els.map((el, n) => ({
    uid: `${a.id}#${n}`, id: a.id, n, title: a.note.title, ...markPosition(anchorBox(el, a.note), a.note.place),
  })))
  observeAnchors()
  if (!help.current) return
  const open = anchors.find(a => a.id === help.current.id)
  if (!open) {
    closeNote() // its control has gone: a row folded, a tab switched
    return
  }
  const n = open.els[help.current.n] ? help.current.n : 0
  const el = open.els[n]
  openMark.value = `${open.id}#${n}`
  readArgs(el)
  outline(el)
  placeOpenNote(el, open.note, keep, scope)
}

// A note written from its control's figures follows them while it is open
// (the target form's Fail Count, say, edited with its note open)
function readArgs(el) {
  const text = el?.dataset.helpArgs ?? ''
  if (text === argsText) return
  argsText = text
  noteArgs.value = argsOf(el)
  relayout = true // the note's size may change: place it again on the next frame
  nextTick(() => schedule())
}

function placeOpenNote(el, item, keep, scope) {
  const node = noteEl.value
  if (!node) return
  const box = el.getBoundingClientRect()
  const aim = anchorBox(el, item)
  const width = node.offsetWidth
  const height = node.scrollHeight + node.offsetHeight - node.clientHeight
  const view = document.documentElement
  const inView = box.bottom > 0 && box.top < view.clientHeight && box.right > 0 && box.left < view.clientWidth
  if (!placement || (!keep && inView)) {
    const p = placeNote(box, width, height, aim, keepClear(scope))
    placement = { side: p.side, dx: p.left - box.left, dy: p.top - box.top, maxHeight: p.maxHeight }
  }
  const left = box.left + placement.dx
  const top = box.top + placement.dy
  const shown = placement.maxHeight ? Math.min(height, placement.maxHeight) : height
  noteAt.value = { left, top, maxHeight: placement.maxHeight }
  line.value = leaderLine(placement.side, { left, top, right: left + width, bottom: top + shown }, box, aim)
  if (focusNote) {
    focusNote = false
    nextTick(() => noteEl.value?.focus({ preventScroll: true }))
  }
}

function outline(el) {
  if (el === outlined) return
  outlined?.removeAttribute('data-help-on')
  outlined = el
  outlined?.setAttribute('data-help-on', '')
}

// A control's size can change with no DOM change of its own (a font loading)
const resizes = new ResizeObserver(() => schedule())
let watched = new Set()
function observeAnchors() {
  const now = new Set(anchors.flatMap(a => a.els))
  for (const el of watched) if (!now.has(el)) resizes.unobserve(el)
  for (const el of now) if (!watched.has(el)) resizes.observe(el)
  watched = now
}

// Changes inside the layer itself (marks moving) are its own doing
const mutations = new MutationObserver(records => {
  if (records.some(r => !root.value?.contains(r.target))) schedule()
})

// ── Opening and closing ──────────────────────────────────────────────────────

function onMark(m) {
  if (isOpen(m)) closeNote()
  else openNote(m.id, m.n)
}

function controlOf(id, n) {
  const a = anchors.find(a => a.id === id)
  return a && (a.els[n] ?? a.els[0])
}

// A control out of the window is scrolled to before its note opens, and when
// its mark gets keyboard focus
function bringIntoView(el) {
  const box = el?.getBoundingClientRect()
  const view = document.documentElement
  if (box && (box.top < 0 || box.bottom > view.clientHeight || box.left < 0 || box.right > view.clientWidth)) {
    el.scrollIntoView({ block: 'center', inline: 'nearest' })
  }
}

// A click on the page outside help closes the note (help stays on); dragging
// the window's scrollbar doesn't
function onPointerDown(e) {
  if (!help.current || e.target === document.documentElement) return
  if (e.target.closest?.('.help-note, .help-mark, .help-bar')) return
  pointerClosed = true
  closeNote()
}

// Focus goes into a note when it opens, and back to its mark when it closes,
// unless the note was left by clicking elsewhere. Stepping to another note
// keeps the note on screen and moves it once the new one is measured.
watch(() => help.current, (now, before) => {
  const focusInNote = !!noteEl.value?.contains(document.activeElement)
  const byPointer = pointerClosed
  pointerClosed = false
  placement = null
  if (!now) {
    noteAt.value = null
    line.value = null
    openMark.value = ''
    focusNote = false
    outline(null)
    if (before && focusInNote && !byPointer) {
      nextTick(() => root.value?.querySelector(`[data-help-mark="${before.id}"][data-help-n="${before.n}"]`)?.focus())
    }
    return
  }
  const el = controlOf(now.id, now.n)
  openMark.value = el ? `${now.id}#${now.n}` : ''
  argsText = el?.dataset.helpArgs ?? ''
  noteArgs.value = argsOf(el)
  bringIntoView(el)
  focusNote = true
  nextTick(() => schedule())
}, { flush: 'pre' })

watch(() => help.page, () => schedule())

onMounted(() => {
  mutations.observe(document.body, {
    subtree: true, childList: true, characterData: true, attributes: true,
    attributeFilter: ['class', 'style', 'hidden', 'open', 'data-help', 'data-help-args', 'data-help-scope', 'data-help-page'],
  })
  resizes.observe(document.documentElement)
  window.addEventListener('scroll', schedule, { capture: true, passive: true })
  window.addEventListener('resize', schedule)
  window.addEventListener('load', schedule, true)
  document.addEventListener('transitionend', schedule, true)
  document.addEventListener('pointerdown', onPointerDown, true)
  schedule()
})

onUnmounted(() => {
  cancelAnimationFrame(frame)
  mutations.disconnect()
  resizes.disconnect()
  window.removeEventListener('scroll', schedule, { capture: true })
  window.removeEventListener('resize', schedule)
  window.removeEventListener('load', schedule, true)
  document.removeEventListener('transitionend', schedule, true)
  document.removeEventListener('pointerdown', onPointerDown, true)
  outline(null)
})
</script>

<template>
  <Teleport to="body">
    <div ref="root" class="help-layer">
      <svg v-if="line" class="help-lines" aria-hidden="true">
        <line :x1="line.x1" :y1="line.y1" :x2="line.x2" :y2="line.y2" />
        <circle :cx="line.x2" :cy="line.y2" r="5" />
      </svg>
      <button
        v-for="m in marks"
        :key="m.uid"
        type="button"
        class="help-mark"
        :class="{ 'is-on': isOpen(m) }"
        :style="{ left: `${m.x}px`, top: `${m.y}px` }"
        :title="m.title"
        :aria-label="`Help: ${m.title}`"
        :aria-expanded="isOpen(m) ? 'true' : 'false'"
        :data-help-mark="m.id"
        :data-help-n="m.n"
        @click="onMark(m)"
        @focus="bringIntoView(controlOf(m.id, m.n))"
      >?</button>
      <div
        v-if="note"
        ref="noteEl"
        class="help-note"
        role="dialog"
        aria-labelledby="help-note-title"
        tabindex="-1"
        :style="noteAt
          ? { left: `${noteAt.left}px`, top: `${noteAt.top}px`, maxHeight: noteAt.maxHeight ? `${noteAt.maxHeight}px` : null }
          : { left: '0px', top: '0px', visibility: 'hidden' }"
      >
        <h3 id="help-note-title" class="help-note-title">{{ note.title }}</h3>
        <p v-for="(sentence, i) in note.text" :key="i" class="help-note-text">{{ sentence }}</p>
        <div v-if="note.rows.length" class="help-note-rows">
          <div v-for="(row, i) in note.rows" :key="i" class="help-note-row">
            <span class="help-note-control"><span :class="row.control.className">{{ row.control.label }}</span></span>
            <span>{{ row.text }}</span>
          </div>
        </div>
        <p v-if="note.tip" class="help-note-tip">{{ note.tip }}</p>
        <button v-if="note.link" type="button" class="help-note-link" @click="note.link.open">{{ note.link.label }}</button>
        <div class="help-note-foot">
          <button v-if="count > 1" type="button" class="help-note-step" @click="stepNote(-1)">‹ {{ previous }}</button>
          <span v-else></span>
          <span class="help-note-pos">{{ at + 1 }} of {{ count }}</span>
          <button v-if="count > 1" type="button" class="help-note-step help-note-next" @click="stepNote(1)">{{ next }} ›</button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
/* Help is turquoise and never Bekci's orange; every box has a full border and
   no stripe (design §5). Tokens are on :root in main.css. */
.help-mark {
  position: fixed;
  z-index: 150;
  width: 18px;
  height: 18px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: var(--help-dk);
  color: #fff;
  font-family: inherit;
  font-size: 11.5px;
  font-weight: 800;
  line-height: 18px;
  text-align: center;
  box-shadow: 0 1px 4px rgba(15, 118, 110, 0.45);
  cursor: pointer;
}
.help-mark:hover { background: var(--help); }
.help-mark.is-on {
  background: #0f172a;
  box-shadow: 0 0 0 3px rgba(20, 184, 166, 0.55);
}
.help-mark:focus-visible { outline: 2px solid var(--help); outline-offset: 2px; }

.help-lines {
  position: fixed;
  inset: 0;
  z-index: 150;
  width: 100%;
  height: 100%;
  overflow: visible;
  pointer-events: none;
}
.help-lines line { stroke: var(--help); stroke-width: 1.8; }
.help-lines circle { fill: var(--help); stroke: #fff; stroke-width: 2; }

.help-note {
  position: fixed;
  z-index: 151;
  width: min(400px, calc(100vw - 24px));
  overflow: auto;
  padding: 16px 20px 0;
  background: #fff;
  color: #334155;
  border: 2px solid var(--help);
  border-radius: 14px;
  box-shadow: var(--help-glow), 0 24px 60px rgba(15, 23, 42, 0.3);
  text-align: left;
}
.help-note:focus { outline: none; }
.help-note-title { font-size: 1.2rem; line-height: 1.3; color: #0f172a; }
.help-note-text { margin-top: 6px; font-size: 1rem; line-height: 1.55; }
.help-note-rows { display: flex; flex-direction: column; margin-top: 10px; }
.help-note-row {
  display: grid;
  grid-template-columns: 92px minmax(0, 1fr);
  gap: 12px;
  align-items: start;
  padding: 8px 0;
  border-top: 1px solid #eef2f6;
  font-size: 0.95rem;
  line-height: 1.5;
}
.help-note-control { justify-self: start; }
.help-note-tip {
  margin-top: 10px;
  padding: 9px 12px;
  border-radius: 8px;
  background: #f1f5f9;
  font-size: 0.95rem;
  line-height: 1.5;
}
.help-note-link {
  margin-top: 10px;
  padding: 0;
  border: 0;
  background: none;
  color: var(--help-dk);
  font: inherit;
  font-weight: 600;
  cursor: pointer;
}
.help-note-foot {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  gap: 8px;
  margin: 14px -20px 0;
  padding: 10px 20px;
  border-top: 1px solid var(--help-line);
  border-radius: 0 0 12px 12px;
  background: var(--help-tint);
  font-size: 0.925rem;
}
.help-note-step {
  justify-self: start;
  padding: 0;
  border: 0;
  background: none;
  color: var(--help-dk);
  font: inherit;
  font-weight: 600;
  text-align: left;
  cursor: pointer;
}
.help-note-next { justify-self: end; text-align: right; }
.help-note-step:focus-visible { outline: 2px solid var(--help); outline-offset: 2px; border-radius: 3px; }
.help-note-pos { color: #64748b; white-space: nowrap; }

/* Pictures of the app's own controls in a note's rows. Their classes are
   scoped to the views (Targets, Search, Alerts), so the few that main.css
   doesn't have are copied here, in the app's real colours. */
.badge-paused { background: #e0e7ff; color: #4338ca; }
.badge-unknown { background: #f1f5f9; color: #64748b; }
.badge-firing { background: #fee2e2; color: #991b1b; }
.badge-recovery { background: #dcfce7; color: #166534; }
.badge-realert { background: #fef3c7; color: #92400e; }
.btn-pause { color: #4338ca; border-color: #c7d2fe; }
.btn-unpause { color: #166534; border-color: #bbf7d0; }
.btn-danger { color: #991b1b; border-color: #fca5a5; }
</style>

<style>
/* The control an open note describes: it is the page's own element, so this
   rule can't be scoped. */
[data-help-on] {
  outline: 2px solid var(--help) !important;
  outline-offset: 3px !important;
  box-shadow: 0 0 0 7px rgba(20, 184, 166, 0.16) !important;
}
</style>
