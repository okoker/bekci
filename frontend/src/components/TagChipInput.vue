<script setup>
import { ref, computed, nextTick } from 'vue'

const props = defineProps({
  modelValue: { type: Array, default: () => [] },
  options: { type: Array, default: () => [] },
  placeholder: { type: String, default: 'Add tag…' },
  allowPin: { type: Boolean, default: false },
  pinned: { type: Array, default: () => [] },
})
const emit = defineEmits(['update:modelValue', 'update:pinned'])

const input = ref('')
const inputEl = ref(null)
const focused = ref(false)

const optionValues = computed(() => props.options.map(o => o.value))

const selected = computed(() => props.modelValue || [])

const suggestions = computed(() => {
  const q = input.value.trim().toUpperCase()
  const selSet = new Set(selected.value)
  return optionValues.value
    .filter(v => !selSet.has(v))
    .filter(v => q === '' || v.toUpperCase().includes(q))
    .slice(0, 10)
})

function addChip(raw) {
  const v = String(raw || '').trim().toUpperCase()
  if (!v) return
  // Only accept values that exist in the catalog — admin-managed only.
  const known = optionValues.value.find(o => o.toUpperCase() === v)
  if (!known) {
    // Surface a transient hint without throwing.
    hint.value = `No tag "${v}" — ask an admin to add it in Settings > Tags.`
    setTimeout(() => { if (hint.value.startsWith('No tag')) hint.value = '' }, 2500)
    return
  }
  if (selected.value.includes(known)) return
  emit('update:modelValue', [...selected.value, known])
  input.value = ''
  nextTick(() => inputEl.value?.focus())
}

function removeChip(v) {
  emit('update:modelValue', selected.value.filter(x => x !== v))
  if (props.allowPin) {
    emit('update:pinned', props.pinned.filter(x => x !== v))
  }
}

function togglePin(v) {
  if (!props.allowPin) return
  const set = new Set(props.pinned)
  if (set.has(v)) set.delete(v)
  else set.add(v)
  emit('update:pinned', Array.from(set))
}

function onKeydown(e) {
  if (e.key === 'Enter') {
    e.preventDefault()
    if (suggestions.value.length > 0) {
      addChip(suggestions.value[0])
    } else if (input.value.trim()) {
      addChip(input.value)
    }
  } else if (e.key === 'Backspace' && input.value === '' && selected.value.length > 0) {
    removeChip(selected.value[selected.value.length - 1])
  }
}

const hint = ref('')

function onBlur() {
  // Delay so mousedown on a suggestion can fire first.
  setTimeout(() => { focused.value = false }, 150)
}
</script>

<template>
  <div class="tag-chip-input" :class="{ focused }">
    <div class="chip-row">
      <span v-for="v in selected" :key="v" class="chip" :class="{ pinned: pinned.includes(v) }">
        <button v-if="allowPin" class="chip-pin" @click="togglePin(v)" :title="pinned.includes(v) ? 'Unpin' : 'Pin to top'" type="button">📌</button>
        <span class="chip-label">{{ v }}</span>
        <button class="chip-x" @click="removeChip(v)" type="button" title="Remove">&times;</button>
      </span>
      <input
        ref="inputEl"
        v-model="input"
        :placeholder="selected.length === 0 ? placeholder : ''"
        class="chip-text"
        @keydown="onKeydown"
        @focus="focused = true"
        @blur="onBlur"
      />
    </div>
    <div v-if="focused && suggestions.length > 0" class="chip-suggest">
      <div
        v-for="s in suggestions"
        :key="s"
        class="chip-suggest-item"
        @mousedown.prevent="addChip(s)"
      >{{ s }}</div>
    </div>
    <div v-if="hint" class="chip-hint">{{ hint }}</div>
  </div>
</template>

<style scoped>
.tag-chip-input {
  position: relative;
  display: inline-block;
  min-width: 200px;
}
.chip-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.25rem;
  padding: 0.25rem 0.4rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: #fff;
  min-height: 2rem;
}
.tag-chip-input.focused .chip-row {
  border-color: #4f46e5;
  box-shadow: 0 0 0 2px rgba(79, 70, 229, 0.15);
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  background: #e0e7ff;
  color: #3730a3;
  border-radius: 10px;
  padding: 0.1rem 0.5rem;
  font-size: 0.8rem;
  font-weight: 500;
}
.chip.pinned {
  background: #fef3c7;
  color: #92400e;
}
.chip-label { user-select: none; }
.chip-x, .chip-pin {
  background: transparent;
  border: none;
  cursor: pointer;
  padding: 0 0.1rem;
  color: inherit;
  font-size: 0.9rem;
  line-height: 1;
}
.chip-x:hover { color: #dc2626; }
.chip-pin { font-size: 0.7rem; opacity: 0.6; }
.chip-pin:hover { opacity: 1; }
.chip-text {
  border: none;
  outline: none;
  padding: 0.2rem 0.25rem;
  font-size: 0.85rem;
  min-width: 100px;
  flex: 1;
  background: transparent;
}
.chip-suggest {
  position: absolute;
  top: 100%;
  left: 0;
  right: 0;
  margin-top: 2px;
  background: #fff;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
  z-index: 50;
  max-height: 240px;
  overflow-y: auto;
}
.chip-suggest-item {
  padding: 0.35rem 0.6rem;
  cursor: pointer;
  font-size: 0.85rem;
}
.chip-suggest-item:hover { background: #f1f5f9; }
.chip-hint {
  margin-top: 0.25rem;
  font-size: 0.75rem;
  color: #dc2626;
}
</style>
