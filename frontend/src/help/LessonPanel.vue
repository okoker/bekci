<script>
// The Mail Gateway rule: the mail server is down when ping and the web page have
// both failed 3 checks in a row, or its certificate has had fewer than 14 days left
// on 3 checks in a row. The settings are recipe C3's (recipes.js), so the lesson
// teaches nothing the plain-words card warns about.
const down = (over) => ({
  check_id: '', config: {}, interval_s: 60, field: 'status', comparator: 'eq', value: 'down',
  fail_count: 3, fail_window: 180, condition_group: 0, group_operator: 'AND', ...over,
})
// Exported so a test can read it back with explainRule
export const mailGateway = [
  down({ check_type: 'ping', check_name: 'Ping', group_operator: 'AND' }),
  down({ check_type: 'http', check_name: 'Web page', group_operator: 'AND' }),
  down({ check_type: 'tls_cert', check_name: 'Certificate', config: { warn_days: 14 }, interval_s: 600, fail_window: 1800, condition_group: 1 }),
]
</script>

<script setup>
// The rule lesson (design in_app_help.md §4.8): six short parts on how an alert
// rule works, in a window with the recipes window's frame. The help layer draws it,
// with help on or off. The Mail Gateway rule is drawn by RuleDiagram and read back
// by explainRule; the Fail Count lanes come from failCountTimeline; so neither can
// drift from the card and the notes. Everything is rendered with {{ }}.
//
// Facts the text rests on (checked against the code):
// - groups are OR'd; inside a group the operator is the LAST condition's, and a
//   condition with Fail Count N needs N matching results in a row
//   (internal/engine/engine.go evaluateRule, evaluateCondition)
// - the form sets the time window once Fail Count is above 1: at least Fail Count
//   x Interval, at most 30 minutes (conditionRules.js minFailWindow, maxFailWindow)
// - a down or recovery message is skipped inside the cooldown, 30 minutes by
//   default; a reminder goes out every hour by default
//   (internal/alerter/alerter.go Dispatch, CheckRealerts; alert_cooldown_s 1800 and
//   alert_realert_s 3600 in internal/store/store.go)
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { closeLesson, openRecipes } from './useHelp.js'
import { explainRule, failCountTimeline } from './explainRule.js'
import { HELP_VERSION } from './content/version.js'
import RuleDiagram from './RuleDiagram.vue'

const PARTS = [
  'How a rule is built',
  'Fail count: riding out blips',
  'AND or OR inside a group?',
  'Groups never confirm each other',
  'When people are told',
  'Mistakes that cause false alarms',
]

const reading = explainRule(mailGateway, 'Mail Gateway', null)

// Ten minutes of a flaky link, checked once a minute: ✓ ✓ ✕ ✕ ✓ ✕ ✕ ✕ ✕ ✓
const SAMPLE = [true, true, false, false, true, false, false, false, false, true]
const INTERVAL = 60

// A lane: the ten results with what Bekci says under each, and how often the target
// goes down and comes back up (down where a run of "down" starts, up where it ends).
// These are changes of state, not messages: inside the cooldown the alerter skips a
// message (alerter.go Dispatch), which part 5 explains (ruling R19).
function lane(id, title, failCount) {
  const { steps } = failCountTimeline(failCount, INTERVAL, SAMPLE)
  let downs = 0
  let ups = 0
  steps.forEach((step, i) => {
    const wasDown = i > 0 && steps[i - 1].label === 'down'
    if (step.label === 'down' && !wasDown) downs++
    if (step.result === 'ok' && wasDown) ups++
  })
  const times = n => (n === 1 ? 'once' : n === 2 ? 'twice' : `${n} times`)
  return { id, title, steps, summary: `down ${times(downs)}, up ${times(ups)}` }
}
const results = failCountTimeline(1, INTERVAL, SAMPLE).steps.map(step => ({ ...step, label: '' }))
const lanes = [
  { id: 'results', title: 'The results', steps: results, summary: '' },
  lane('fc1', 'Fail Count 1', 1),
  lane('fc3', 'Fail Count 3', 3),
]

const dialog = ref(null)

// Focus goes into the window, and back where it was when it closes, unless
// something else has taken it meanwhile
let opener = null
onMounted(() => {
  opener = document.activeElement
  nextTick(() => dialog.value?.focus({ preventScroll: true }))
})
onBeforeUnmount(() => {
  const active = document.activeElement
  if ((active === document.body || dialog.value?.contains(active)) && opener?.isConnected) {
    opener.focus({ preventScroll: true })
  }
})
</script>

<template>
  <div class="lesson-backdrop">
    <div ref="dialog" class="lesson-window" role="dialog" aria-modal="true" aria-labelledby="lesson-title"
      tabindex="-1" data-help-scope="lesson">
      <header class="lesson-head">
        <div>
          <h2 id="lesson-title">How alert rules work</h2>
          <p class="lesson-sub">From one test to the whole rule, in six short parts.</p>
        </div>
        <span class="lesson-version">Written for Bekci {{ HELP_VERSION }}</span>
        <button type="button" class="lesson-x" aria-label="Close lesson" @click="closeLesson">✕</button>
      </header>

      <div class="lesson-body">
        <div class="lesson-column">
          <!-- 1 -->
          <section class="lesson-part">
            <h3 class="lesson-heading">{{ PARTS[0] }}</h3>
            <div class="lesson-terms">
              <div class="lesson-term">
                <b>Condition</b>
                <span>One test plus what counts as failing, for example "ping, status equals down, 3 in a row".</span>
              </div>
              <div class="lesson-term">
                <b>Group</b>
                <span>One or more conditions joined by AND (all failing) or OR (any one failing).</span>
              </div>
              <div class="lesson-term">
                <b>Rule</b>
                <span>One or more groups, always joined by OR. The target is down when any group is met.</span>
              </div>
            </div>
            <p class="lesson-lead">An example, the rule for a mail gateway:</p>
            <div class="lesson-figure">
              <RuleDiagram :conditions="mailGateway" />
              <div class="lesson-read">
                <p><b>Read it as:</b> {{ reading.opening }}</p>
                <template v-for="(g, i) in reading.groups" :key="i">
                  <p v-if="i > 0" class="lesson-or-line"><span class="lesson-or">OR</span></p>
                  <p class="lesson-clause">{{ g.sentence }}</p>
                </template>
                <p>{{ reading.closing }}</p>
              </div>
            </div>
          </section>

          <!-- 2 -->
          <section class="lesson-part">
            <h3 class="lesson-heading">{{ PARTS[1] }}</h3>
            <p>Ten minutes of a flaky link, checked once a minute. Here is what Bekci does with the same ten results at Fail Count 1 and at Fail Count 3; part 5 says which changes send a message.</p>
            <div class="lesson-lanes">
              <div v-for="l in lanes" :key="l.id" class="lesson-lane" :data-lane="l.id">
                <div class="lesson-lane-title">{{ l.title }}</div>
                <ol class="lesson-steps">
                  <li v-for="(step, i) in l.steps" :key="i" class="lesson-step">
                    <span :class="['lesson-mark', `is-${step.result}`]" role="img"
                      :aria-label="step.result === 'ok' ? 'passed' : 'failed'">{{ step.result === 'ok' ? '✓' : '✕' }}</span>
                    <span :class="['lesson-step-label', { 'is-down': step.label === 'down' }]">{{ step.label }}</span>
                  </li>
                </ol>
                <p v-if="l.summary" class="lesson-lane-summary">{{ l.summary }}</p>
              </div>
            </div>
            <p>The time window must hold the whole run. Bekci sets it for you once Fail Count is above 1: at least Fail Count × Interval, at most 30 minutes.</p>
          </section>

          <!-- 3 -->
          <section class="lesson-part">
            <h3 class="lesson-heading">{{ PARTS[2] }}</h3>
            <div class="lesson-pair">
              <div class="lesson-term">
                <b>AND: Confirm</b>
                <span>All must be failing at the same time. Use it when two different tests should agree.</span>
              </div>
              <div class="lesson-term">
                <b>OR: Either</b>
                <span>Any one is enough. Separate groups do the same job and read more clearly.</span>
              </div>
            </div>
          </section>

          <!-- 4 -->
          <section class="lesson-part">
            <h3 class="lesson-heading">{{ PARTS[3] }}</h3>
            <p>Groups never confirm each other: each group is a separate way to be down, so adding a group adds ways to be alerted.</p>
          </section>

          <!-- 5 -->
          <section class="lesson-part">
            <h3 class="lesson-heading">{{ PARTS[4] }}</h3>
            <p>Recipients get a message when the target goes down and when it recovers. If it changes again within the cooldown (Settings › Alerting, 30 minutes by default) that message is skipped, so a short outage may produce no recovery message. While it stays down, a reminder goes out every hour by default.</p>
          </section>

          <!-- 6 -->
          <section class="lesson-part">
            <h3 class="lesson-heading">{{ PARTS[5] }}</h3>
            <ul class="lesson-mistakes">
              <li>OR inside a group when you meant AND.</li>
              <li>Fail Count 1 on a link that drops the odd packet.</li>
              <li>Two groups when you meant one AND group.</li>
              <li>Page Hash on its own: an outage looks like defacement.</li>
            </ul>
          </section>

          <!-- 7 -->
          <div class="lesson-more">
            <button type="button" class="lesson-recipes-btn" @click="openRecipes">Rule recipes: ready-made rules</button>
            <span>Twelve rules that already avoid these mistakes.</span>
          </div>
        </div>
      </div>

      <footer class="lesson-foot">
        <button type="button" class="btn lesson-close" @click="closeLesson">Close</button>
      </footer>
    </div>
  </div>
</template>

<style scoped>
/* A help window, framed like the recipes window (design §4.8, §5): a full 2px
   turquoise border with the glow, no stripe, nothing orange. Groups are blue and
   OR between them is slate, because orange is the form's own OR. It sits under the
   recipes window (same layer, earlier in the DOM). */
.lesson-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1102;
  background: rgba(15, 23, 42, 0.38);
}
.lesson-window {
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
.lesson-window:focus { outline: none; }

.lesson-head {
  display: flex;
  align-items: center;
  gap: 20px;
  padding: 16px 24px;
  border-bottom: 1px solid #e2e8f0;
}
.lesson-head h2 { font-size: 21px; color: #0f172a; }
.lesson-sub { margin-top: 3px; font-size: 14px; }
.lesson-version { margin-left: auto; font-size: 12.5px; color: #64748b; white-space: nowrap; }
.lesson-x {
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
.lesson-x:hover { background: #f1f5f9; }
.lesson-x:focus-visible,
.lesson-recipes-btn:focus-visible { outline: 2px solid var(--help); outline-offset: 2px; }

.lesson-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 8px 24px 28px;
  background: #f8fafc;
}
.lesson-column {
  max-width: 900px;
  margin: 0 auto;
  counter-reset: part;
}
.lesson-part {
  margin-top: 18px;
  padding: 18px 22px 20px;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  background: #fff;
  font-size: 15px;
  line-height: 1.6;
}
.lesson-part p + p,
.lesson-part p + .lesson-lanes,
.lesson-lanes + p { margin-top: 10px; }
.lesson-heading {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
  font-size: 18px;
  color: #0f172a;
}
.lesson-heading::before {
  counter-increment: part;
  content: counter(part);
  flex: none;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: var(--help-dk);
  color: #fff;
  font-size: 14px;
  line-height: 26px;
  text-align: center;
}

.lesson-terms,
.lesson-pair {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}
.lesson-pair { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.lesson-term {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px 14px;
  border: 1px solid var(--help-line);
  border-radius: 10px;
  background: var(--help-tint);
  font-size: 14.5px;
  line-height: 1.5;
}
.lesson-term b { color: var(--help-dk); font-size: 15px; }
.lesson-lead { margin-top: 14px; }
.lesson-figure {
  margin-top: 8px;
  padding: 14px 16px;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  background: #f8fafc;
}
.lesson-read { margin-top: 12px; font-size: 14.5px; }
.lesson-read p + p { margin-top: 6px; }
.lesson-clause {
  padding: 7px 12px;
  border: 1.5px solid #93c5fd;
  border-radius: 8px;
  background: #fff;
  color: #0f172a;
}
.lesson-or-line { text-align: center; }
.lesson-or {
  display: inline-block;
  padding: 0 9px;
  border-radius: 10px;
  background: #475569;
  color: #fff;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.05em;
}

/* The Fail Count lanes */
.lesson-lanes {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 10px;
}
.lesson-lane {
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr) 150px;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  background: #f8fafc;
}
.lesson-lane-title { font-weight: 700; color: #0f172a; font-size: 14px; }
.lesson-steps {
  display: grid;
  grid-template-columns: repeat(10, minmax(0, 1fr));
  list-style: none;
}
.lesson-step { display: flex; flex-direction: column; align-items: center; gap: 4px; }
.lesson-mark {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  font-size: 13px;
  font-weight: 700;
  line-height: 24px;
  text-align: center;
}
.lesson-mark.is-ok { background: #dcfce7; color: #166534; }
.lesson-mark.is-fail { background: #fee2e2; color: #991b1b; }
.lesson-step-label {
  min-height: 16px;
  font-size: 11.5px;
  line-height: 16px;
  color: #64748b;
  white-space: nowrap;
}
.lesson-step-label.is-down { color: #991b1b; font-weight: 600; }
.lesson-lane-summary { margin: 0; font-size: 14px; font-weight: 600; color: var(--help-dk); }

.lesson-mistakes { padding-left: 22px; }
.lesson-mistakes li + li { margin-top: 4px; }
.lesson-mistakes li::marker { color: var(--help); }

.lesson-more {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-top: 18px;
  padding: 16px 22px;
  border: 1px solid var(--help-line);
  border-radius: 12px;
  background: var(--help-tint);
  font-size: 14.5px;
}
.lesson-recipes-btn {
  padding: 11px 22px;
  border: 0;
  border-radius: 10px;
  background: var(--help-dk);
  color: #fff;
  font: inherit;
  font-size: 15px;
  font-weight: 700;
  box-shadow: 0 8px 22px rgba(15, 118, 110, 0.35);
  cursor: pointer;
}
.lesson-recipes-btn:hover { background: #115e59; }

.lesson-foot {
  display: flex;
  justify-content: flex-end;
  padding: 14px 24px;
  border-top: 1px solid var(--help-line);
  background: var(--help-tint);
}
</style>
