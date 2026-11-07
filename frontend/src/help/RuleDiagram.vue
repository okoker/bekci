<script>
// An alert rule drawn as the engine reads it (design in_app_help.md §4.7): one
// blue box per group, a chip per condition (its check's name and how many
// results in a row it needs), the group's AND / OR between chips, and a slate
// OR pill between groups. A condition that must be passing (Status equals up,
// used to rule out an outage) is a green chip. Plain text only, rendered with {{ }}.
//
// Groups follow the engine; explainRule.js reads them the same way for the card.
import { ruleGroups as groupsOf } from './explainRule.js'
export function ruleGroups(conditions) {
  return groupsOf(conditions).map((g, i) => ({ ...g, label: `Group ${i + 1}${g.conds.length > 1 ? `, ${g.op}` : ''}` }))
}

// Must be passing for the rule to fire: Status equals up
export const isPassing = (c) => c.field === 'status' && c.comparator === 'eq' && c.value === 'up'
</script>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  conditions: { type: Array, default: () => [] },
})

const groups = computed(() => ruleGroups(props.conditions))
// How many results in a row the condition needs: Fail Count, or just the latest at 1
const inARow = (c) => (Number(c.fail_count) > 1 ? `${Number(c.fail_count)} in a row` : 'latest check')
</script>

<template>
  <div class="rule-diagram">
    <template v-for="(g, i) in groups" :key="g.id">
      <span v-if="i > 0" class="rd-or">OR</span>
      <div class="rd-group">
        <small class="rd-group-label">{{ g.label }}</small>
        <div class="rd-row">
          <template v-for="(c, j) in g.conds" :key="j">
            <span v-if="j > 0" class="rd-join">{{ g.op }}</span>
            <span class="rd-chip" :class="{ 'is-passing': isPassing(c) }">{{ c.check_name }}<em class="rd-chip-count">{{ inARow(c) }}</em></span>
          </template>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
/* Group boxes are blue, as in the form; OR between groups is a slate pill,
   because orange is the form's own OR (design §5). */
.rule-diagram {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
}
.rd-group {
  padding: 8px 10px;
  border: 1.5px solid #93c5fd;
  border-radius: 8px;
  background: #fff;
}
.rd-group-label {
  display: block;
  margin-bottom: 6px;
  font-size: 12px;
  font-weight: 600;
  color: #1e40af;
}
.rd-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.rd-chip {
  display: inline-block;
  max-width: 230px;
  padding: 5px 9px;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  background: #fff;
  color: #1e293b;
  font-size: 13px;
  line-height: 1.35;
}
.rd-chip-count {
  display: block;
  font-size: 12px;
  font-style: normal;
  color: #64748b;
}
.rd-chip.is-passing {
  border-color: #86efac;
  background: #f0fdf4;
  color: #166534;
}
.rd-chip.is-passing .rd-chip-count { color: #15803d; }
.rd-join {
  font-size: 12px;
  font-weight: 700;
  color: #94a3b8;
}
.rd-or {
  padding: 3px 10px;
  border-radius: 12px;
  background: #475569;
  color: #fff;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.05em;
}
</style>
