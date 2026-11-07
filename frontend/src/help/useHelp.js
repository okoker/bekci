// In-app help: the shared state. This is the only help module in the main
// bundle; the bar, the notes and the help content load on first use, in their
// own chunk (design in_app_help.md §6).
import { defineAsyncComponent, h, reactive } from 'vue'

// Turn help on and off only through toggleHelp() and closeHelp(): they also
// follow the page on screen and listen for Esc.
export const help = reactive({
  on: false,          // help mode is on
  page: '',           // the help page on screen: its data-help-page id ('' if none)
  items: [],          // the notes with a mark on screen, in page order (filled by the help layer)
  current: null,      // the open note, or null
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

// Esc closes one thing per press. Help isn't modal, so a page's own window
// (the SLA zoom, an audit entry) may be open: its Esc handler, on document,
// runs before this one, on window, and marks the key used (preventDefault).
// Esc typed in a field belongs to the field (the Search name box closes on it).
function onKeydown(e) {
  if (e.key !== 'Escape' || e.defaultPrevented || e.isComposing) return
  if (e.target.closest?.('input, textarea, select, [contenteditable]')) return
  closeHelp()
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
