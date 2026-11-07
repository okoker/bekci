<script setup>
// The slate help bar under the header (design §4.2): the page summary, "Click
// any ? to read it" and the stepper through the notes, the version the help was
// written for, and Done. Its look is in main.css, shared with the "could not
// load" message shown in its place (useHelp.js); the hint and the stepper,
// which only the bar has, are styled here.
import { computed } from 'vue'
import { help, closeHelp, stepNote } from './useHelp.js'
import { pages } from './content/index.js'
import { HELP_VERSION } from './content/version.js'

const summary = computed(() => pages[help.page]?.summary)
const running = computed(() =>
  help.runningVersion && help.runningVersion !== HELP_VERSION ? ` (running ${help.runningVersion})` : '')

// "5 of 12" while a note is open; before that, how many there are
const position = computed(() => {
  const count = help.items.length
  const at = help.items.findIndex(item => item.id === help.current?.id)
  return at < 0 ? `${count} ${count === 1 ? 'note' : 'notes'}` : `${at + 1} of ${count}`
})
</script>

<template>
  <div class="help-bar" role="region" aria-label="Help" :data-for-page="help.page">
    <span class="help-disc" aria-hidden="true"></span>
    <p class="help-bar-text">
      <template v-if="summary"><strong>{{ summary.name }}</strong> {{ summary.text }}</template>
    </p>
    <div class="help-bar-side">
      <template v-if="help.items.length">
        <span class="help-bar-hint">Click any ? to read it</span>
        <span class="help-stepper">
          <button type="button" aria-label="Previous note" @click="stepNote(-1)">‹</button>
          <span class="help-stepper-pos">{{ position }}</span>
          <button type="button" aria-label="Next note" @click="stepNote(1)">›</button>
        </span>
      </template>
      <span class="help-bar-stamp">Written for <b>Bekci {{ HELP_VERSION }}</b>{{ running }}</span>
      <button type="button" class="help-done" @click="closeHelp">Done</button>
    </div>
  </div>
</template>

<style scoped>
.help-bar-hint { font-size: 0.875rem; color: #94a3b8; }
.help-stepper {
  display: inline-flex;
  align-items: center;
  border: 1px solid #475569;
  border-radius: 8px;
  overflow: hidden;
}
.help-stepper button {
  width: 34px;
  height: 32px;
  border: 0;
  background: none;
  color: #fff;
  font-family: inherit;
  font-size: 16px;
  cursor: pointer;
}
.help-stepper button:hover { background: #334155; }
.help-stepper button:focus-visible { outline: 2px solid var(--help-line); outline-offset: -2px; }
.help-stepper-pos {
  min-width: 70px;
  padding: 0 10px;
  border-left: 1px solid #475569;
  border-right: 1px solid #475569;
  color: #e2e8f0;
  font-size: 0.875rem;
  line-height: 32px;
  text-align: center;
  white-space: nowrap;
}
</style>
