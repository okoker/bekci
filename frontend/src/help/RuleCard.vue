<script setup>
// The plain-words card (design in_app_help.md §4.6): the target form's alert rule
// read back as sentences, and checked for settings that cause false alarms. The
// form shows it only while help is on, and it follows the form as it changes.
// explainRule returns plain strings; they are rendered with {{ }}, so a target
// name with HTML in it shows as typed.
import { computed } from 'vue'
import { explainRule } from './explainRule.js'

const props = defineProps({
  conditions: { type: Array, default: () => [] },
  targetName: { type: String, default: '' },
  // Ticked recipients the alerter would message; null while unknown (the user list didn't load)
  recipientCount: { type: Number, default: null },
})

const rule = computed(() => explainRule(props.conditions, props.targetName, props.recipientCount))

// ✓ met, ! a setting that can cause false alarms; a neutral note has no sign
const SIGN = { ok: '✓', warn: '!', note: '' }
</script>

<template>
  <aside class="rule-card" aria-labelledby="rule-card-title">
    <div class="rule-card-head">
      <h3 id="rule-card-title">Your rule in plain words</h3>
      <p>Rewritten from the form as you change it</p>
    </div>
    <div class="rule-card-body">
      <template v-if="rule.groups.length">
        <p>{{ rule.opening }}</p>
        <template v-for="(group, i) in rule.groups" :key="i">
          <div v-if="i > 0" class="rule-card-or"><span>OR</span></div>
          <div class="rule-card-group">
            {{ group.sentence }}
            <small v-if="group.delay">{{ group.delay }}</small>
          </div>
        </template>
        <p class="rule-card-closing">{{ rule.closing }}</p>
      </template>
      <p v-else class="rule-card-empty">No conditions yet.</p>
    </div>
    <div v-if="rule.checks.length" class="rule-card-checks">
      <h4>Checked for false alarms</h4>
      <p v-for="(check, i) in rule.checks" :key="i" :class="['rule-card-check', `is-${check.level}`]">
        <span class="rule-card-sign" aria-hidden="true">{{ SIGN[check.level] }}</span>
        <span>{{ check.text }}</span>
      </p>
    </div>
  </aside>
</template>

<style scoped>
/* A help box: full turquoise border and glow, no stripe, nothing orange
   (design §4.6, §5). Groups are blue, as in the form; OR between them is slate,
   because orange is the form's own OR. */
.rule-card {
  overflow: hidden;
  background: #fff;
  color: #334155;
  border: 2px solid var(--help);
  border-radius: 14px;
  box-shadow: var(--help-glow), 0 22px 60px rgba(15, 23, 42, 0.28);
  text-align: left;
}
.rule-card-head {
  padding: 14px 18px;
  background: var(--help-tint);
  border-bottom: 1px solid var(--help-line);
}
.rule-card-head h3 { font-size: 1.15rem; line-height: 1.3; color: #0f172a; }
.rule-card-head p { margin-top: 2px; font-size: 0.9rem; color: var(--help-dk); }
.rule-card-body {
  padding: 14px 18px 16px;
  font-size: 1rem;
  line-height: 1.55;
}
.rule-card-group {
  margin-top: 10px;
  padding: 9px 12px;
  border: 1.5px solid #93c5fd;
  border-radius: 8px;
  background: #f8fbff;
  color: #0f172a;
}
.rule-card-group small {
  display: block;
  margin-top: 2px;
  font-size: 0.85rem;
  color: #64748b;
}
.rule-card-or {
  display: flex;
  justify-content: center;
  margin-top: 10px;
}
.rule-card-or span {
  padding: 2px 12px;
  border-radius: 12px;
  background: #475569;
  color: #fff;
  font-size: 0.85rem;
  font-weight: 700;
  letter-spacing: 0.05em;
}
.rule-card-closing { margin-top: 10px; font-size: 0.95rem; }
.rule-card-empty { color: #64748b; }
.rule-card-checks {
  display: flex;
  flex-direction: column;
  gap: 7px;
  padding: 12px 18px 14px;
  border-top: 1px solid #e2e8f0;
}
.rule-card-checks h4 { font-size: 0.95rem; color: #0f172a; }
.rule-card-check {
  display: grid;
  grid-template-columns: 18px minmax(0, 1fr);
  gap: 8px;
  font-size: 0.925rem;
  line-height: 1.45;
}
.rule-card-sign {
  width: 18px;
  height: 18px;
  margin-top: 1px;
  border-radius: 50%;
  font-weight: 800;
  line-height: 18px;
  text-align: center;
}
.is-ok .rule-card-sign { color: #15803d; }
.is-warn .rule-card-sign { background: #fef3c7; color: #b45309; }
.is-note { color: #64748b; }
</style>
