<script setup>
// The rule recipes window (design in_app_help.md §4.7): the 12 ready-made alert
// rules on three shelves, the chosen one explained (its rule drawn, what alerts
// and what doesn't, what to check first, what it puts in the form), and the
// button that puts it into the open target form. The help layer draws it, over
// the target form, with help on or off.
//
// It names its own help scope, so while it is open the marks of the page under
// it are not drawn. The recipes' texts come from recipes.js as written; the rest
// is read from their conditions. Everything is rendered with {{ }}.
//
// "Use this recipe" shows only while a target form is open (help.formApply):
// the form decides whether to ask first, and fills itself in.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { help, closeRecipes } from './useHelp.js'
import { recipes } from './recipes.js'
import { HELP_VERSION } from './content/version.js'
import { checkTypes } from '../conditionRules.js'
import RuleDiagram, { isPassing, ruleGroups } from './RuleDiagram.vue'

const SHELVES = [
  { id: 'basic', title: 'Basic: one test', note: 'One test, a few failures in a row.' },
  { id: 'combined', title: 'Combined: several tests', note: "They confirm each other, so one flaky test can't page anyone." },
  { id: 'security', title: 'Security: catch tampering', note: "Set up so an outage can't pass for an attack." },
]
const shelves = SHELVES.map(s => ({ ...s, recipes: recipes.filter(r => r.shelf === s.id) }))

const selectedId = ref(recipes[0].id)
const recipe = computed(() => recipes.find(r => r.id === selectedId.value))
const shelfTitle = computed(() => SHELVES.find(s => s.id === recipe.value.shelf).title)
const hasPassing = computed(() => recipe.value.conditions.some(isPassing))

const PILL = { alerts: 'alerts', no: 'no alert', missed: 'not caught' }

// The form's own words: its Check Type, Field and Comparator labels
const TYPE_LABEL = Object.fromEntries(checkTypes.map(t => [t.value, t.label]))
const FIELD_LABEL = { status: 'Status', response_ms: 'Response (ms)', 'metrics.packet_loss': 'Packet loss (%)' }
const COMPARATOR_LABEL = { eq: 'equals', neq: 'not equals', gt: 'greater than', lt: 'less than' }

// "What it puts in the form": a row per condition, in rule order, with a blue
// row for each group when the recipe has more than one condition
const tableRows = computed(() => {
  const r = recipe.value
  const rows = []
  for (const g of ruleGroups(r.conditions)) {
    if (r.conditions.length > 1) rows.push({ group: g.label })
    for (const c of g.conds) rows.push({ cond: c, settings: settingsOf(r, c) })
  }
  return rows
})

// A condition's settings as the form will show them. A field the person fills in
// shows as its placeholder, marked in turquoise: [{ text, need? }, …]
function settingsOf(r, c) {
  const k = c.config
  const index = r.conditions.indexOf(c)
  const needFor = key => r.needs.find(n => n.condition === index && n.field === `config.${key}`)
  const items = []
  const add = (label, key, show = v => v) => {
    const need = needFor(key)
    items.push(need ? { text: label, need: need.placeholder } : { text: `${label}${show(k[key])}` })
  }
  const seconds = v => `${v} s`
  switch (c.check_type) {
    case 'http':
      items.push({ text: String(k.scheme).toUpperCase() })
      if (k.port) add('port ', 'port')
      add('endpoint ', 'endpoint')
      add('expected status ', 'expect_status')
      if (k.skip_tls_verify) items.push({ text: 'Skip TLS Verify' })
      break
    case 'page_hash':
      items.push({ text: String(k.scheme).toUpperCase() })
      add('endpoint ', 'endpoint')
      items.push({ text: k.baseline_hash ? 'baseline hash set' : 'baseline hash empty = auto-capture' })
      break
    case 'tcp':
      add('port ', 'port')
      add('timeout ', 'timeout_s', seconds)
      if (k.expect_closed) items.push({ text: 'Port must stay closed' })
      break
    case 'ping':
      add('ping count ', 'count')
      add('timeout ', 'timeout_s', seconds)
      break
    case 'dns':
      items.push({ text: k.query ? `query ${k.query}` : 'query empty = target host' })
      add('record ', 'record_type')
      if (k.nameserver) add('nameserver ', 'nameserver')
      if (k.expect_value || needFor('expect_value')) add('expected value ', 'expect_value')
      break
    case 'tls_cert':
      add('port ', 'port')
      add('warning days ', 'warn_days')
      break
    default:
      if (k.port) add('port ', 'port')
      if (k.timeout_s) add('timeout ', 'timeout_s', seconds)
  }
  return items
}

const every = s => (s % 60 === 0 ? `${s / 60} min` : `${s} s`)
const alertWhen = c => `${FIELD_LABEL[c.field] ?? c.field} ${COMPARATOR_LABEL[c.comparator] ?? c.comparator} ${c.value}`
// The form's Time Window is off at Fail Count 1
const windowOf = c => (c.fail_count > 1 && c.fail_window ? `${c.fail_window} s` : 'off')

const dialog = ref(null)
const detail = ref(null)
watch(selectedId, () => nextTick(() => { if (detail.value) detail.value.scrollTop = 0 }))

// The form decides: it asks first when it holds conditions, then fills itself in
function use() {
  const apply = help.formApply
  closeRecipes()
  apply?.(recipe.value)
}

// Focus goes into the window, and back where it was ("Start from a recipe")
// when it closes, unless something else has taken it meanwhile
let opener = null
onMounted(() => {
  opener = document.activeElement
  dialog.value?.focus({ preventScroll: true })
})
onBeforeUnmount(() => {
  const active = document.activeElement
  if ((active === document.body || dialog.value?.contains(active)) && opener?.isConnected) {
    opener.focus({ preventScroll: true })
  }
})
</script>

<template>
  <div class="recipes-backdrop">
    <div ref="dialog" class="recipes-window" role="dialog" aria-modal="true" aria-labelledby="recipes-title"
      tabindex="-1" data-help-scope="recipes">
      <header class="recipes-head">
        <div>
          <h2 id="recipes-title">Rule recipes</h2>
          <p class="recipes-sub">Ready-made alert rules for common jobs. Each one is set up to alert on real problems, not blips.</p>
        </div>
        <span class="recipes-version">Written for Bekci {{ HELP_VERSION }}</span>
        <button type="button" class="recipes-x" aria-label="Close recipes" @click="closeRecipes">✕</button>
      </header>

      <div class="recipes-body">
        <nav class="recipes-shelves" aria-label="Recipes">
          <section v-for="s in shelves" :key="s.id" class="recipe-shelf">
            <div class="recipe-shelf-head">
              <b class="recipe-shelf-title">{{ s.title }}</b>
              <span class="recipe-shelf-note">{{ s.note }}</span>
            </div>
            <button v-for="r in s.recipes" :key="r.id" type="button" class="recipe-row" :data-recipe="r.id"
              :aria-pressed="r.id === selectedId ? 'true' : 'false'" @click="selectedId = r.id">
              <b class="recipe-row-name">{{ r.name }}</b>
              <small class="recipe-row-formula">{{ r.formula }}</small>
            </button>
          </section>
        </nav>

        <article ref="detail" class="recipe-detail">
          <span class="recipe-shelf-tag">{{ shelfTitle }}</span>
          <h2 class="recipe-name">{{ recipe.name }}</h2>
          <p class="recipe-catches">{{ recipe.catches }}</p>

          <div class="recipe-rule">
            <RuleDiagram :conditions="recipe.conditions" />
            <p class="recipe-read"><b>Read it as:</b> {{ recipe.readAs }}</p>
            <p v-if="hasPassing" class="recipe-legend">
              <span class="recipe-legend-swatch" aria-hidden="true"></span>Green tests must be passing for the rule to fire, so an outage can't set it off.
            </p>
          </div>

          <div class="recipe-boxes">
            <section class="recipe-box recipe-situations">
              <h3>What alerts, and what doesn't</h3>
              <div v-for="(s, i) in recipe.situations" :key="i" class="recipe-situation">
                <span class="recipe-situation-text">{{ s.text }}</span>
                <span class="recipe-pill" :class="`is-${s.result}`">{{ PILL[s.result] }}</span>
              </div>
            </section>
            <section class="recipe-box recipe-before">
              <h3>Before you use it</h3>
              <ul>
                <li v-for="(b, i) in recipe.before" :key="i">{{ b }}</li>
              </ul>
            </section>
          </div>

          <h3 class="recipe-table-title">What it puts in the form</h3>
          <table class="recipe-table">
            <thead>
              <tr><th>Check</th><th>Settings</th><th>Every</th><th>Alert when</th><th>Fail count</th><th>Window</th></tr>
            </thead>
            <tbody>
              <template v-for="(row, i) in tableRows" :key="i">
                <tr v-if="row.group" class="recipe-group-row"><td colspan="6">{{ row.group }}</td></tr>
                <tr v-else>
                  <td><b>{{ row.cond.check_name }}</b><small>{{ TYPE_LABEL[row.cond.check_type] ?? row.cond.check_type }}</small></td>
                  <td><template v-for="(item, j) in row.settings" :key="j">{{ j > 0 ? ', ' : '' }}{{ item.text }}<span v-if="item.need" class="recipe-need">{{ item.need }}</span></template></td>
                  <td class="recipe-short">{{ every(row.cond.interval_s) }}</td>
                  <td :class="{ 'is-passing': isPassing(row.cond) }">{{ alertWhen(row.cond) }}</td>
                  <td class="recipe-short">{{ row.cond.fail_count }}</td>
                  <td class="recipe-short">{{ windowOf(row.cond) }}</td>
                </tr>
              </template>
            </tbody>
          </table>
        </article>
      </div>

      <footer class="recipes-foot">
        <div v-if="help.formApply" class="recipes-use">
          <button type="button" class="recipes-use-btn" @click="use">Use this recipe for your monitoring target!</button>
          <p class="recipes-use-note">Puts these checks into the form. You fill in anything marked in turquoise, then Save.</p>
        </div>
        <button type="button" class="btn recipes-close" @click="closeRecipes">Close</button>
      </footer>
    </div>
  </div>
</template>

<style scoped>
/* A help window (design §4.7, §5): a full 2px turquoise border with the glow,
   no stripe, nothing orange. It covers the target form with 40px margins and
   draws above every app layer (the form 100, the SLA zoom 1000) and help's
   marks and notes (1100, 1101). */
.recipes-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1102;
  background: rgba(15, 23, 42, 0.38);
}
.recipes-window {
  position: absolute;
  inset: 40px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: #fff;
  color: #334155;
  border: 2px solid var(--help);
  border-radius: 16px;
  box-shadow: var(--help-glow-window), 0 30px 80px rgba(15, 23, 42, 0.45);
  text-align: left;
}
.recipes-window:focus { outline: none; }

.recipes-head {
  display: flex;
  align-items: center;
  gap: 20px;
  padding: 16px 24px;
  border-bottom: 1px solid #e2e8f0;
}
.recipes-head h2 { font-size: 21px; color: #0f172a; }
.recipes-sub { margin-top: 3px; font-size: 14px; }
.recipes-version { margin-left: auto; font-size: 12.5px; color: #64748b; white-space: nowrap; }
.recipes-x {
  flex: none;
  width: 32px;
  height: 32px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
  color: #334155;
  font: inherit;
  font-size: 15px;
  cursor: pointer;
}
.recipes-x:hover { background: #f1f5f9; }

.recipes-body {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: 330px minmax(0, 1fr);
}

/* Left: the shelves, all turquoise */
.recipes-shelves {
  overflow: auto;
  padding: 14px;
  background: #f8fafc;
  border-right: 1px solid #e2e8f0;
}
.recipe-shelf {
  overflow: hidden;
  border: 1px solid var(--help-line);
  border-radius: 10px;
  background: #fff;
}
.recipe-shelf + .recipe-shelf { margin-top: 12px; }
.recipe-shelf-head {
  padding: 9px 13px;
  background: var(--help-tint);
  border-bottom: 1px solid var(--help-line);
}
.recipe-shelf-title { display: block; font-size: 14px; color: var(--help-dk); }
.recipe-shelf-note { font-size: 12.5px; color: #2f6f68; }
.recipe-row {
  display: block;
  width: 100%;
  padding: 7px 13px;
  border: 0;
  border-top: 1px solid #f1f5f9;
  background: none;
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.recipe-shelf-head + .recipe-row { border-top: 0; }
.recipe-row:hover { background: #f8fafc; }
.recipe-row-name { display: block; font-size: 13.5px; font-weight: 600; color: #1e293b; }
.recipe-row-formula { font-size: 12px; color: #64748b; }
/* The chosen recipe: a turquoise tint and a 2px outline all round, no side bar */
.recipe-row[aria-pressed='true'] {
  background: var(--help-tint);
  box-shadow: inset 0 0 0 2px var(--help);
}
.recipe-row[aria-pressed='true'] .recipe-row-name { color: #0f172a; }
.recipe-row:focus-visible,
.recipes-x:focus-visible,
.recipes-use-btn:focus-visible {
  outline: 2px solid var(--help);
  outline-offset: 2px;
}

/* Right: the chosen recipe */
.recipe-detail { overflow: auto; padding: 24px 34px 30px; }
.recipe-shelf-tag {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 6px;
  background: var(--help-tint);
  color: var(--help-dk);
  font-size: 12px;
  font-weight: 600;
}
.recipe-name { margin-top: 8px; font-size: 24px; letter-spacing: -0.01em; color: #0f172a; }
.recipe-catches { margin-top: 4px; font-size: 16px; }
.recipe-rule {
  margin-top: 18px;
  padding: 18px 20px;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  background: #f8fafc;
}
.recipe-read { max-width: 780px; margin-top: 14px; font-size: 14.5px; line-height: 1.6; }
.recipe-read b { color: #0f172a; }
.recipe-legend {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
  font-size: 13px;
  color: #64748b;
}
.recipe-legend-swatch {
  flex: none;
  width: 22px;
  height: 12px;
  border: 1px solid #86efac;
  border-radius: 4px;
  background: #f0fdf4; /* the green chip it explains (RuleDiagram .rd-chip.is-passing) */
}

.recipe-boxes {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 16px;
  margin-top: 16px;
}
.recipe-box {
  padding: 14px 16px;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  background: #fff;
}
.recipe-box h3 { margin-bottom: 6px; font-size: 14.5px; color: #0f172a; }
.recipe-situation {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 12px;
  align-items: center;
  padding: 8px 0;
  border-top: 1px solid #f1f5f9;
  font-size: 13.5px;
  line-height: 1.45;
}
.recipe-box h3 + .recipe-situation { border-top: 0; }
.recipe-pill {
  padding: 2px 9px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
}
.recipe-pill.is-alerts { background: #fee2e2; color: #991b1b; }
.recipe-pill.is-no { background: #dcfce7; color: #166534; }
.recipe-pill.is-missed { background: #fef3c7; color: #92400e; }
.recipe-before ul {
  display: flex;
  flex-direction: column;
  gap: 9px;
  margin-top: 4px;
  list-style: none;
}
.recipe-before li {
  position: relative;
  padding-left: 15px;
  font-size: 13.5px;
  line-height: 1.5;
}
.recipe-before li::before {
  content: '';
  position: absolute;
  top: 7px;
  left: 0;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--help);
}

.recipe-table-title { margin: 22px 0 8px; font-size: 14.5px; color: #0f172a; }
.recipe-table {
  width: 100%;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  border-collapse: separate;
  border-spacing: 0;
  overflow: hidden;
  font-size: 13px;
}
.recipe-table th {
  padding: 8px 10px;
  border-bottom: 1px solid #e2e8f0;
  background: #f8fafc;
  color: #64748b;
  font-size: 12px;
  font-weight: 600;
  text-align: left;
  text-transform: none;
  letter-spacing: normal;
  white-space: nowrap;
}
.recipe-table td.recipe-short { white-space: nowrap; }
.recipe-table td {
  padding: 8px 10px;
  border-bottom: 1px solid #f1f5f9;
  line-height: 1.45;
  vertical-align: top;
}
.recipe-table tr:last-child td { border-bottom: 0; }
.recipe-table td b { font-weight: 600; color: #0f172a; }
.recipe-table td small { display: block; font-size: 12px; color: #64748b; }
.recipe-table td.is-passing { color: #166534; font-weight: 600; }
.recipe-table .recipe-group-row td {
  padding: 5px 10px;
  background: #eff6ff;
  color: #1e40af;
  font-size: 12px;
  font-weight: 600;
}
.recipe-need {
  display: inline-block;
  padding: 0 5px;
  border: 1px dashed var(--help);
  border-radius: 4px;
  background: var(--help-tint);
  color: var(--help-dk);
  font-style: italic;
}

/* The footer: the main button centred, Close at the right */
.recipes-foot {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  gap: 14px;
  padding: 16px 24px;
  border-top: 1px solid var(--help-line);
  background: var(--help-tint);
}
.recipes-use {
  grid-column: 2;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 7px;
}
.recipes-use-btn {
  min-width: 460px;
  padding: 14px 32px;
  border: 0;
  border-radius: 10px;
  background: var(--help-dk);
  color: #fff;
  font: inherit;
  font-size: 16px;
  font-weight: 700;
  box-shadow: 0 8px 22px rgba(15, 118, 110, 0.35);
  cursor: pointer;
}
.recipes-use-btn:hover { background: #115e59; }
.recipes-use-note { font-size: 13px; color: #64748b; }
.recipes-close { grid-column: 3; justify-self: end; }
</style>
