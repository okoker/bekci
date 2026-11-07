<script setup>
// The slate help bar under the header (design §4.2): the page summary, the
// version the help was written for, and Done. Its look is in main.css, shared
// with the "could not load" message shown in its place (useHelp.js).
import { computed } from 'vue'
import { help, closeHelp } from './useHelp.js'
import { pages } from './content/index.js'
import { HELP_VERSION } from './content/version.js'

const summary = computed(() => pages[help.page]?.summary)
const running = computed(() =>
  help.runningVersion && help.runningVersion !== HELP_VERSION ? ` (running ${help.runningVersion})` : '')
</script>

<template>
  <div class="help-bar" role="region" aria-label="Help" :data-for-page="help.page">
    <span class="help-disc" aria-hidden="true"></span>
    <p class="help-bar-text">
      <template v-if="summary"><strong>{{ summary.name }}</strong> {{ summary.text }}</template>
    </p>
    <div class="help-bar-side">
      <span class="help-bar-stamp">Written for <b>Bekci {{ HELP_VERSION }}</b>{{ running }}</span>
      <button type="button" class="help-done" @click="closeHelp">Done</button>
    </div>
  </div>
</template>
