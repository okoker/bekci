// In-app help: the shared state. This is the only help module in the main
// bundle; the bar, the notes and the help content load on first use, in their
// own chunk (design in_app_help.md §6).
import { defineAsyncComponent, h, reactive } from 'vue'

// Turn help on and off only through toggleHelp() and closeHelp(): they also
// follow the page on screen and listen for Esc.
export const help = reactive({
  on: false,          // help mode is on
  page: '',           // the help page on screen: its data-help-page id ('' if none)
  items: [],          // the notes with a mark on screen, in page order, each key once:
                      // [{ id: '<page>.<key>', title }] (kept up to date by the help layer)
  current: null,      // the open note: { id, n } (n: which of the key's marks, 0 = first), or null
  runningVersion: '', // the Bekci version the server reports (GET /api/system/health)
  recipesOpen: false, // the rule recipes window is open
  lessonOpen: false,  // the rule lesson is open
  formApply: null,    // set by an open target form: puts a recipe's conditions into it
})

// The help page on screen is the last element, in document order, that names
// one. A view's root names its page (Settings names the tab on show); the
// target form sits inside its view and names its own, so it wins while open.
function findPage() {
  const named = document.querySelectorAll('[data-help-page]')
  return named.length ? named[named.length - 1].dataset.helpPage : ''
}

// While help is on, a Settings tab switch or the target form opening changes
// the page on screen without a route change: follow it.
let observer = null

// A key pressed in a field you type into belongs to the field: Esc closes the
// Search name box, the arrows move the caret. A checkbox or a button isn't one.
const NOT_TYPED_INTO = ['checkbox', 'radio', 'button', 'submit', 'reset']
function inTextField(el) {
  if (el?.tagName === 'INPUT') return !NOT_TYPED_INTO.includes(el.type)
  return !!el?.closest?.('textarea, select, [contenteditable]')
}

// Focus on help's own parts (a mark, the open note, the bar) or on nothing.
function onHelp(el) {
  return !el?.closest || el === document.body || !!el.closest('.help-mark, .help-note, .help-bar')
}

// Esc closes one thing per press. Help isn't modal, so a page's own window
// (the SLA zoom, an audit entry) may be open: its Esc handler, on document,
// runs before this one, on window, and marks the key used (preventDefault).
// Then the open note closes, and only then help. While a note is open, ← and →
// step through the notes, unless focus is somewhere else on the page.
function onKeydown(e) {
  if (e.defaultPrevented || e.isComposing || inTextField(e.target)) return
  if (e.key === 'Escape') {
    if (help.current) {
      e.preventDefault()
      closeNote()
    } else {
      closeHelp()
    }
  } else if ((e.key === 'ArrowLeft' || e.key === 'ArrowRight') && help.current && onHelp(e.target)
    && !e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey) {
    e.preventDefault()
    stepNote(e.key === 'ArrowRight' ? 1 : -1)
  }
}

export function toggleHelp() {
  if (help.on) {
    closeHelp()
    return
  }
  help.on = true
  help.page = findPage()
  observer = new MutationObserver(() => { help.page = findPage() })
  observer.observe(document.body, { subtree: true, childList: true, attributeFilter: ['data-help-page'] })
  window.addEventListener('keydown', onKeydown)
}

export function closeHelp() {
  if (!help.on) return
  help.on = false
  help.page = ''
  help.items = []
  help.current = null
  observer?.disconnect()
  observer = null
  window.removeEventListener('keydown', onKeydown)
}

// The notes: one open at a time. `n` picks one of a key's marks when the key
// has a mark on every instance (the target form's per-condition fields).
export function openNote(id, n = 0) {
  help.current = { id, n }
}

export function closeNote() {
  help.current = null
}

// The next (1) or previous (-1) note in page order, going round at the ends;
// with none open, the first or the last.
export function stepNote(dir) {
  const count = help.items.length
  if (!count) return
  const at = help.items.findIndex(item => item.id === help.current?.id)
  const next = at < 0 ? (dir > 0 ? 0 : count - 1) : (at + dir + count) % count
  openNote(help.items[next].id)
}

export function openRecipes() {
  help.recipesOpen = true
}

export function openLesson() {
  help.lessonOpen = true
}

// Shown in place of the bar when its chunk can't be fetched: typically an old
// tab after a deploy, whose chunk names no longer exist on the server.
const HelpLoadError = {
  name: 'HelpLoadError',
  setup: () => () => h('div', { class: 'help-bar help-bar-error', role: 'alert' }, [
    h('span', { class: 'help-disc', 'aria-hidden': 'true' }),
    h('p', { class: 'help-bar-text' }, 'Help could not load: reload the page.'),
    h('button', { type: 'button', class: 'help-done', onClick: closeHelp }, 'Done'),
  ]),
}

// The help bar, fetched the first time help is turned on.
export const LazyHelpBar = defineAsyncComponent({
  loader: () => import('./HelpBar.vue'),
  errorComponent: HelpLoadError,
})

// The target form's plain-words card, fetched the first time help is on in the
// form. If its code can't be fetched, the same message as the bar's takes its place.
export const LazyRuleCard = defineAsyncComponent({
  loader: () => import('./RuleCard.vue'),
  errorComponent: HelpLoadError,
})

// The ? marks and their notes, fetched with the bar. When it can't be fetched
// the bar's own message says so, so this shows nothing.
export const LazyHelpLayer = defineAsyncComponent({
  loader: () => import('./HelpLayer.vue'),
  errorComponent: { name: 'HelpLayerLoadError', render: () => null },
})
