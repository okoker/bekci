<script setup>
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import api from '../api'
import TagChipInput from './TagChipInput.vue'
import { help, toggleHelp, openRecipes, LazyHelpBar, LazyRuleCard } from '../help/useHelp.js'
import { applyConditions, needOf, needError } from '../help/applyConditions.js'
import {
  checkTypes, getDefaultConfig, getDefaultCondition, maxFailWindow, minFailWindow,
  failWindowError, valueError, packetLossHint,
} from '../conditionRules'

const props = defineProps({
  show: { type: Boolean, default: false },
  targetId: { type: String, default: null },
  cloneSourceId: { type: String, default: null }
})

const emit = defineEmits(['close', 'saved'])

// Form state
const form = ref(getEmptyForm())
const formError = ref('')
const projectOptions = ref([])
const locationOptions = ref([])
const categoryOptions = ref([])
const tagOptions = ref([])

function getEmptyForm() {
  return {
    name: '', host: '', description: '', enabled: true,
    category: '', preferred_check_type: '',
    notes: '', contacts: '', project: '', location: '',
    tags: [],
    conditions: []
  }
}

// Computed: group conditions by condition_group for display
const conditionGroups = computed(() => {
  const groups = {}
  for (const cond of form.value.conditions) {
    const g = cond.condition_group ?? 0
    if (!groups[g]) groups[g] = { operator: cond.group_operator || 'AND', conditions: [] }
    groups[g].conditions.push(cond)
  }
  return Object.keys(groups)
    .map(Number)
    .sort((a, b) => a - b)
    .map(idx => ({ groupIdx: idx, operator: groups[idx].operator, conditions: groups[idx].conditions }))
})

function addGroup() {
  const maxGroup = form.value.conditions.reduce((max, c) => Math.max(max, c.condition_group ?? 0), -1)
  form.value.conditions.push(getDefaultCondition(maxGroup + 1, 'AND'))
}

function addConditionToGroup(groupIdx) {
  const existing = form.value.conditions.find(c => c.condition_group === groupIdx)
  const op = existing?.group_operator || 'AND'
  form.value.conditions.push(getDefaultCondition(groupIdx, op))
}

function setGroupOperator(groupIdx, op) {
  for (const cond of form.value.conditions) {
    if (cond.condition_group === groupIdx) {
      cond.group_operator = op
    }
  }
}

function removeCondition(cond) {
  const idx = form.value.conditions.indexOf(cond)
  if (idx >= 0) form.value.conditions.splice(idx, 1)
}

function removeGroup(groupIdx) {
  form.value.conditions = form.value.conditions.filter(c => c.condition_group !== groupIdx)
}

const hasFailWindowError = computed(() => form.value.conditions.some(cond => failWindowError(cond)))

// Set on mount/update so native form validation shows this message on Save.
const vCustomValidity = (el, { value }) => el.setCustomValidity(value)

function onFailCountChange(cond) {
  if (cond.fail_count > 1) onIntervalChange(cond)
  else cond.fail_window = 0
}

function onIntervalChange(cond) {
  if (cond.fail_count > 1) {
    cond.fail_window = Math.min(maxFailWindow, Math.max(minFailWindow(cond), Number(cond.fail_window) || 0))
  }
}

function onConditionTypeChange(cond) {
  if (!cond.check_id) {
    cond.config = getDefaultConfig(cond.check_type)
    // A recipe's fields to fill in belong to the type it set, whose settings just went
    delete cond.needs
  }
  // Packet loss exists only for ping
  if (cond.field === 'metrics.packet_loss' && cond.check_type !== 'ping') {
    cond.field = 'status'
    onFieldChange(cond)
  }
}

// Changing Field sets the Comparator and Value that field needs, so a rule
// such as "packet loss equals down", which can never be true, is not left behind.
function onFieldChange(cond) {
  if (cond.field === 'status') {
    cond.comparator = 'eq'
    cond.value = 'down'
  } else {
    cond.comparator = 'gt'
    cond.value = ''
  }
}

const valuePlaceholder = { status: 'down', response_ms: '500', 'metrics.packet_loss': '30' }

const hasValueError = computed(() => form.value.conditions.some(cond => valueError(cond)))

// A field a recipe left for the person (a port, an address, a path) still empty,
// or not in the form it needs: Save waits for it, with the recipe's message
const hasNeedError = computed(() => form.value.conditions.some(cond =>
  (cond.needs || []).some(need => needError(cond, need.field))))

// Unique check types from current conditions — for preferred check dropdown
const availableCheckTypes = computed(() => {
  const seen = new Set()
  const types = []
  for (const c of form.value.conditions) {
    if (c.check_type && !seen.has(c.check_type)) {
      seen.add(c.check_type)
      const label = checkTypes.find(t => t.value === c.check_type)?.label || c.check_type
      types.push({ value: c.check_type, label })
    }
  }
  return types
})

// ── Alert recipients ──
const allUsers = ref([])
const allUsersError = ref(false)
const selectedRecipients = ref([])

async function loadAllUsers() {
  allUsersError.value = false
  try {
    const { data } = await api.get('/users')
    allUsers.value = data.filter(u => u.status === 'active')
  } catch {
    allUsersError.value = true
  }
}

async function loadTagOptions() {
  try {
    const [p, l, c, t] = await Promise.all([
      api.get('/tags?group=project'),
      api.get('/tags?group=location'),
      api.get('/tags?group=category'),
      api.get('/tags?group=tag')
    ])
    projectOptions.value = p.data
    locationOptions.value = l.data
    categoryOptions.value = c.data
    tagOptions.value = t.data
  } catch { /* ignore */ }
}

function toggleRecipient(userId) {
  const idx = selectedRecipients.value.indexOf(userId)
  if (idx >= 0) {
    selectedRecipients.value.splice(idx, 1)
  } else {
    selectedRecipients.value.push(userId)
  }
}

async function saveRecipients(targetId) {
  await api.put(`/targets/${targetId}/recipients`, { user_ids: selectedRecipients.value })
}

// ── In-app help (design in_app_help.md §4.5) ──
// The form is help page "targetForm" and its own help scope. At 1280px and
// wider the plain-words card sits left of the form and notes open in the right
// margin (the form card is marked data-help-margin for the help layer); narrower, the
// card sits above Condition Groups and notes open below their field.
const WIDE = '(min-width: 1280px)'
const wide = ref(false)
let wideQuery = null
const onWideChange = () => { wide.value = wideQuery.matches }
onMounted(() => {
  wideQuery = window.matchMedia(WIDE)
  onWideChange()
  wideQuery.addEventListener('change', onWideChange)
})
onUnmounted(() => wideQuery?.removeEventListener('change', onWideChange))

// Who the card counts as ticked: the ticked boxes, i.e. active users (the only
// ones the alerter messages); unknown while the user list can't be loaded.
const recipientCount = computed(() => (allUsersError.value
  ? null
  : allUsers.value.filter(u => selectedRecipients.value.includes(u.id)).length))

// ── Rule recipes (design in_app_help.md §4.7) ──
// "Start from a recipe" opens the recipes window, with help on or off. While
// the form is open it registers useRecipe, which the window's "Use this recipe"
// button calls. A form that holds conditions asks first, because on a saved
// target Save deletes the checks no longer in the form, with their results
// (store/targets.go UpdateTargetWithConditions; ON DELETE CASCADE). Nothing is
// sent until Save, and Undo puts back what the form had.
const conditionsSection = ref(null)
const recipeCancel = ref(null)
const pendingRecipe = ref(null) // the recipe waiting for Replace or Cancel
const recipeBanner = ref(null)  // after a recipe went in: { name, fill, previous } (previous is for Undo)

function useRecipe(recipe) {
  if (form.value.conditions.length) {
    pendingRecipe.value = recipe
    nextTick(() => recipeCancel.value?.focus())
  } else {
    putRecipe(recipe)
  }
}

// applyConditions replaces the conditions (never merges) with copies of the
// recipe's, sets Preferred Check, and returns what the form had
function putRecipe(recipe) {
  pendingRecipe.value = null
  const previous = applyConditions(form.value, recipe.conditions, recipe.preferred, recipe.needs)
  recipeBanner.value = { name: recipe.name, fill: recipe.needs.length > 0, previous }
  nextTick(() => conditionsSection.value?.scrollIntoView({ block: 'start' }))
}

// The conditions and Preferred Check from before the recipe, check ids
// included, so Save keeps those checks
function undoRecipe() {
  const { previous } = recipeBanner.value
  form.value.conditions = previous.conditions
  form.value.preferred_check_type = previous.preferred_check_type
  recipeBanner.value = null
}

watch(() => props.show, (open) => {
  if (open) help.formApply = useRecipe
  else if (help.formApply === useRecipe) help.formApply = null
})
onUnmounted(() => {
  if (help.formApply === useRecipe) help.formApply = null
})

// Load target detail for editing
async function loadTargetDetail(id) {
  try {
    const { data } = await api.get(`/targets/${id}`)
    form.value = {
      name: data.name,
      host: data.host,
      description: data.description,
      enabled: data.enabled,
      category: data.category || 'Other',
      preferred_check_type: data.preferred_check_type || '',
      notes: data.notes || '',
      contacts: data.contacts || '',
      project: data.project || '',
      location: data.location || '',
      tags: Array.isArray(data.tags) ? [...data.tags] : [],
      conditions: (data.conditions || []).map(c => {
        let cfg = {}
        try { cfg = JSON.parse(c.config) } catch { cfg = {} }
        return {
          check_id: c.check_id || '',
          check_type: c.check_type,
          check_name: c.check_name,
          config: cfg,
          interval_s: c.interval_s,
          field: c.field || 'status',
          comparator: c.comparator || 'eq',
          value: c.value || 'down',
          fail_count: c.fail_count || 1,
          fail_window: c.fail_window || 0,
          condition_group: c.condition_group ?? 0,
          group_operator: c.group_operator || 'AND',
        }
      })
    }
    selectedRecipients.value = data.recipient_ids || []
  } catch (e) {
    formError.value = 'Failed to load target details'
  }
}

// Save
async function saveTarget() {
  formError.value = ''
  if (hasFailWindowError.value || hasValueError.value || hasNeedError.value) return
  try {
    const payload = {
      name: form.value.name,
      host: form.value.host,
      description: form.value.description,
      enabled: form.value.enabled,
      operator: 'AND',
      category: form.value.category,
      preferred_check_type: form.value.preferred_check_type,
      notes: form.value.notes || null,
      contacts: form.value.contacts || null,
      project: form.value.project || null,
      location: form.value.location || null,
      tags: form.value.tags || [],
      conditions: form.value.conditions.map(c => ({
        check_id: c.check_id || undefined,
        check_type: c.check_type,
        check_name: c.check_name,
        config: JSON.stringify(c.config),
        interval_s: Number(c.interval_s) || 60,
        field: c.field || 'status',
        comparator: c.comparator || 'eq',
        value: c.value || 'down',
        fail_count: Number(c.fail_count) || 1,
        fail_window: Number(c.fail_window) || 0,
        condition_group: c.condition_group ?? 0,
        group_operator: c.group_operator || 'AND',
      }))
    }

    let targetId
    if (props.targetId) {
      await api.put(`/targets/${props.targetId}`, payload)
      targetId = props.targetId
    } else {
      const { data } = await api.post('/targets', payload)
      targetId = data.id
    }
    if (targetId) {
      await saveRecipients(targetId)
    }
    emit('saved')
  } catch (e) {
    formError.value = e.response?.data?.error || 'Failed to save target'
  }
}

function close() {
  emit('close')
}

// When modal opens, load data
watch(() => props.show, async (val) => {
  if (!val) return
  formError.value = ''
  pendingRecipe.value = null
  recipeBanner.value = null
  loadAllUsers()
  loadTagOptions()
  if (props.targetId) {
    await loadTargetDetail(props.targetId)
  } else if (props.cloneSourceId) {
    await loadTargetDetail(props.cloneSourceId)
    // Clear IDs so save creates new target + new checks
    form.value.name = 'Clone of ' + form.value.name
    form.value.host = ''
    form.value.conditions = form.value.conditions.map(c => ({ ...c, check_id: '' }))
    selectedRecipients.value = []
  } else {
    form.value = getEmptyForm()
    selectedRecipients.value = []
  }
})
</script>

<template>
  <div v-if="show" class="modal-overlay" :class="{ 'help-on': help.on, 'help-wide': help.on && wide }"
    data-help-scope="form" data-help-page="targetForm">
    <LazyHelpBar v-if="help.on" />
    <LazyRuleCard v-if="help.on && wide" class="form-rule-card" :conditions="form.conditions"
      :target-name="form.name" :recipient-count="recipientCount" />
    <div class="modal-card modal-wide" :data-help-margin="help.on && wide ? '' : null">
      <div class="modal-title-row">
        <h3>{{ targetId ? 'Edit Target' : cloneSourceId ? 'Clone Target' : 'New Target' }}</h3>
        <button type="button" class="btn btn-sm btn-form-help" :class="{ 'is-on': help.on }"
          :aria-pressed="help.on ? 'true' : 'false'" @click="toggleHelp">
          <span class="form-help-icon" aria-hidden="true"></span>Help
        </button>
      </div>
      <div v-if="formError" class="error-msg" style="margin-bottom: 0.75rem;">{{ formError }}</div>
      <form @submit.prevent="saveTarget">
        <!-- Target fields -->
        <div class="form-row">
          <div class="form-group" data-help="targetForm.name">
            <label class="required">Name</label>
            <input v-model="form.name" required placeholder="e.g. Web Server" />
          </div>
          <div class="form-group" data-help="targetForm.host">
            <label class="required">Host</label>
            <input v-model="form.host" required placeholder="e.g. google.com" />
          </div>
        </div>
        <div class="form-group">
          <label>Description</label>
          <input v-model="form.description" placeholder="Optional description" />
        </div>
        <div class="form-group" data-help="targetForm.category">
          <label class="required">Category</label>
          <select v-model="form.category" required>
            <option value="" disabled>Select category</option>
            <option v-for="c in categoryOptions" :key="c.id" :value="c.value">{{ c.value }}</option>
          </select>
        </div>
        <div v-if="form.conditions.length > 1" class="form-group" data-help="targetForm.preferred">
          <label>Preferred Check (for SLA &amp; dashboard)</label>
          <select v-model="form.preferred_check_type">
            <option v-for="t in availableCheckTypes" :key="t.value" :value="t.value">{{ t.label }}</option>
          </select>
          <span class="text-muted input-hint">Which check type drives the SLA badge and dashboard uptime display</span>
        </div>

        <!-- Notes & Contacts -->
        <div data-help="targetForm.notes">
          <div class="form-group form-group-wide">
            <label>Notes</label>
            <textarea v-model="form.notes" rows="4" placeholder="Optional notes about this target"></textarea>
          </div>
          <div class="form-group form-group-wide">
            <label>Contacts</label>
            <textarea v-model="form.contacts" rows="4" placeholder="Optional contact information"></textarea>
          </div>
        </div>

        <!-- Project & Location tags -->
        <div data-help="targetForm.filters">
          <div class="form-row">
            <div class="form-group">
              <label>Project</label>
              <select v-model="form.project">
                <option value="">— none —</option>
                <option v-for="t in projectOptions" :key="t.id" :value="t.value">{{ t.value }}</option>
              </select>
            </div>
            <div class="form-group">
              <label>Location</label>
              <select v-model="form.location">
                <option value="">— none —</option>
                <option v-for="t in locationOptions" :key="t.id" :value="t.value">{{ t.value }}</option>
              </select>
            </div>
          </div>

          <div class="form-group form-group-wide">
            <label>Tags</label>
            <TagChipInput v-model="form.tags" :options="tagOptions" placeholder="Type to add (e.g. P1)" />
          </div>
        </div>

        <div class="form-group checkbox-group" data-help="targetForm.enabled">
          <label class="checkbox-label"><input type="checkbox" v-model="form.enabled" /> Enabled</label>
        </div>

        <!-- The plain-words card, in help mode below 1280px -->
        <LazyRuleCard v-if="help.on && !wide" class="form-rule-card-inline" :conditions="form.conditions"
          :target-name="form.name" :recipient-count="recipientCount" />

        <!-- Condition Groups section -->
        <div ref="conditionsSection" class="conditions-section">
          <div class="conditions-header">
            <strong data-help="targetForm.groups">Condition Groups</strong>
            <span class="conditions-buttons">
              <button type="button" class="btn btn-sm btn-recipe" @click="openRecipes">Start from a recipe</button>
              <button type="button" class="btn btn-sm btn-primary" data-help="targetForm.addgroup" @click="addGroup">+ Add Group or Condition</button>
            </span>
          </div>

          <div v-if="recipeBanner" class="recipe-banner" role="status">
            <span class="recipe-banner-text">Filled in from the recipe <b>{{ recipeBanner.name }}</b>. {{ recipeBanner.fill ? 'Fill in the fields marked in turquoise, then Save.' : 'Check the settings, then Save.' }}</span>
            <button type="button" class="recipe-undo" @click="undoRecipe">Undo</button>
          </div>

          <div v-if="form.conditions.length === 0" class="validation-warning">
            At least one condition is required. Add a group to get started.
          </div>

          <template v-for="(group, gIdx) in conditionGroups" :key="group.groupIdx">
            <!-- OR divider between groups -->
            <div v-if="gIdx > 0" class="or-divider"><span class="or-pill" data-help="targetForm.or">OR</span></div>

            <div class="group-card">
              <div class="group-header">
                <span class="group-label" data-help="targetForm.group">Group {{ gIdx + 1 }}</span>
                <div v-if="group.conditions.length > 1" class="operator-toggle" data-help="targetForm.andor">
                  <button type="button" :class="['op-btn', { active: group.operator === 'AND' }]" @click="setGroupOperator(group.groupIdx, 'AND')">AND</button>
                  <button type="button" :class="['op-btn', { active: group.operator === 'OR' }]" @click="setGroupOperator(group.groupIdx, 'OR')">OR</button>
                </div>
                <button type="button" class="btn btn-sm btn-danger" data-help="targetForm.remove" @click="removeGroup(group.groupIdx)">Remove Group</button>
              </div>

              <template v-for="(cond, cIdx) in group.conditions" :key="cond.check_id || cIdx">
                <!-- Within-group operator label -->
                <div v-if="cIdx > 0" class="intra-group-op">{{ group.operator }}</div>

                <div class="condition-card">
                  <div class="condition-card-header">
                    <span class="condition-num">{{ cond.check_type.toUpperCase() }}</span>
                    <button type="button" class="btn btn-sm btn-danger" data-help="targetForm.remove" @click="removeCondition(cond)">Remove</button>
                  </div>

                  <!-- Check fields -->
                  <div class="form-row">
                    <div class="form-group" data-help="targetForm.type">
                      <label>Check Type</label>
                      <select v-model="cond.check_type" @change="onConditionTypeChange(cond)" :disabled="!!cond.check_id">
                        <option v-for="t in checkTypes" :key="t.value" :value="t.value">{{ t.label }}</option>
                      </select>
                    </div>
                    <div class="form-group">
                      <label>Check Name</label>
                      <input v-model="cond.check_name" required placeholder="e.g. HTTPS Check" />
                    </div>
                  </div>
                  <div class="form-group" data-help="targetForm.int">
                    <label>Interval (seconds)</label>
                    <input type="number" v-model.number="cond.interval_s" min="10" max="86400" style="max-width: 120px;"
                      @change="onIntervalChange(cond)" />
                  </div>

                  <!-- Type-specific config -->
                  <template v-if="cond.check_type === 'http'">
                    <div class="form-row">
                      <div class="form-group">
                        <label>Scheme</label>
                        <select v-model="cond.config.scheme">
                          <option value="http">HTTP</option>
                          <option value="https">HTTPS</option>
                        </select>
                      </div>
                      <div class="form-group">
                        <label>Port (0 = default)</label>
                        <input type="number" v-model.number="cond.config.port" min="0" max="65535" />
                      </div>
                    </div>
                    <div class="form-row">
                      <div class="form-group">
                        <label>Endpoint</label>
                        <input v-model="cond.config.endpoint" :placeholder="needOf(cond, 'config.endpoint')?.placeholder ?? '/'"
                          :class="{ 'need-field': needOf(cond, 'config.endpoint') }"
                          v-custom-validity="needError(cond, 'config.endpoint')" />
                      </div>
                      <div class="form-group" data-help="targetForm.expect">
                        <label>Expected Status</label>
                        <input type="number" v-model.number="cond.config.expect_status" />
                      </div>
                    </div>
                    <div class="form-row">
                      <div class="form-group" data-help="targetForm.timeout">
                        <label>Timeout (s)</label>
                        <input type="number" v-model.number="cond.config.timeout_s" min="1" max="60" />
                      </div>
                      <div class="form-group checkbox-group" data-help="targetForm.skiptls">
                        <label class="checkbox-label"><input type="checkbox" v-model="cond.config.skip_tls_verify" /> Skip TLS Verify</label>
                      </div>
                    </div>
                  </template>

                  <template v-if="cond.check_type === 'tcp'">
                    <div class="form-row">
                      <div class="form-group">
                        <label>Port</label>
                        <input type="number" v-model.number="cond.config.port" min="1" max="65535" required
                          :placeholder="needOf(cond, 'config.port')?.placeholder"
                          :class="{ 'need-field': needOf(cond, 'config.port') }"
                          v-custom-validity="needError(cond, 'config.port')" />
                      </div>
                      <div class="form-group" data-help="targetForm.timeout">
                        <label>Timeout (s)</label>
                        <input type="number" v-model.number="cond.config.timeout_s" min="1" max="60" />
                      </div>
                    </div>
                    <div class="form-group checkbox-group" data-help="targetForm.closed">
                      <label class="checkbox-label"><input type="checkbox" v-model="cond.config.expect_closed" /> Port must stay closed</label>
                    </div>
                    <p v-if="cond.config.expect_closed" class="text-muted input-hint">Up while the port is closed or does not answer, down when it accepts a connection. The usual Status equals down then alerts when the port opens.</p>
                  </template>

                  <template v-if="cond.check_type === 'ping'">
                    <div class="form-row">
                      <div class="form-group" data-help="targetForm.pingcount">
                        <label>Ping Count</label>
                        <input type="number" v-model.number="cond.config.count" min="1" max="10" />
                      </div>
                      <div class="form-group" data-help="targetForm.timeout">
                        <label>Timeout (s)</label>
                        <input type="number" v-model.number="cond.config.timeout_s" min="1" max="30" />
                      </div>
                    </div>
                  </template>

                  <template v-if="cond.check_type === 'dns'">
                    <div class="form-row">
                      <div class="form-group">
                        <label>Query (empty = target host)</label>
                        <input v-model="cond.config.query" placeholder="Leave empty for target host" />
                      </div>
                      <div class="form-group">
                        <label>Record Type</label>
                        <select v-model="cond.config.record_type">
                          <option value="A">A</option>
                          <option value="AAAA">AAAA</option>
                          <option value="MX">MX</option>
                          <option value="CNAME">CNAME</option>
                        </select>
                      </div>
                    </div>
                    <div class="form-row">
                      <div class="form-group" data-help="targetForm.dnsexpect">
                        <label>Expected Value (optional)</label>
                        <input v-model="cond.config.expect_value" :placeholder="needOf(cond, 'config.expect_value')?.placeholder ?? 'e.g. 1.2.3.4'"
                          :class="{ 'need-field': needOf(cond, 'config.expect_value') }"
                          v-custom-validity="needError(cond, 'config.expect_value')" />
                      </div>
                      <div class="form-group" data-help="targetForm.dnsns">
                        <label>Nameserver (optional)</label>
                        <input v-model="cond.config.nameserver" placeholder="e.g. 8.8.8.8" />
                      </div>
                    </div>
                  </template>

                  <template v-if="cond.check_type === 'page_hash'">
                    <div class="form-row">
                      <div class="form-group">
                        <label>Scheme</label>
                        <select v-model="cond.config.scheme">
                          <option value="http">HTTP</option>
                          <option value="https">HTTPS</option>
                        </select>
                      </div>
                      <div class="form-group">
                        <label>Endpoint</label>
                        <input v-model="cond.config.endpoint" placeholder="/" />
                      </div>
                    </div>
                    <div class="form-group" data-help="targetForm.baseline">
                      <label>Baseline Hash (empty = auto-capture)</label>
                      <input v-model="cond.config.baseline_hash" placeholder="Leave empty to auto-capture" />
                    </div>
                  </template>

                  <template v-if="cond.check_type === 'tls_cert'">
                    <div class="form-row">
                      <div class="form-group">
                        <label>Port</label>
                        <input type="number" v-model.number="cond.config.port" min="1" max="65535" />
                      </div>
                      <div class="form-group" data-help="targetForm.warn">
                        <label>Warning Days</label>
                        <input type="number" v-model.number="cond.config.warn_days" min="1" />
                      </div>
                    </div>
                  </template>

                  <template v-if="cond.check_type === 'snmp_v2c' || cond.check_type === 'snmp_v3'">
                    <div class="form-row">
                      <div class="form-group">
                        <label>Port</label>
                        <input type="number" v-model.number="cond.config.port" placeholder="161" />
                      </div>
                      <div class="form-group" data-help="targetForm.timeout">
                        <label>Timeout (s)</label>
                        <input type="number" v-model.number="cond.config.timeout_s" placeholder="5" />
                      </div>
                    </div>
                    <p class="text-muted input-hint" data-help="targetForm.snmp">SNMP credentials are configured globally in Settings.</p>
                  </template>

                  <!-- Alert criteria divider -->
                  <div class="alert-divider" data-help="targetForm.when">Alert when...</div>
                  <div class="form-row form-row-5">
                    <div class="form-group">
                      <label>Field</label>
                      <select v-model="cond.field" @change="onFieldChange(cond)">
                        <option value="status">Status</option>
                        <option value="response_ms">Response (ms)</option>
                        <option v-if="cond.check_type === 'ping' || cond.field === 'metrics.packet_loss'"
                          value="metrics.packet_loss">Packet loss (%)</option>
                      </select>
                    </div>
                    <div class="form-group">
                      <label>Comparator</label>
                      <select v-model="cond.comparator">
                        <option value="eq">equals</option>
                        <option value="neq">not equals</option>
                        <option value="gt">greater than</option>
                        <option value="lt">less than</option>
                      </select>
                    </div>
                    <div class="form-group">
                      <label>Value</label>
                      <input v-model="cond.value" :placeholder="valuePlaceholder[cond.field] || 'down'"
                        v-custom-validity="valueError(cond)" />
                    </div>
                    <div class="form-group" data-help="targetForm.fc"
                      :data-help-args="JSON.stringify({ fail_count: cond.fail_count, interval_s: cond.interval_s })">
                      <label>Fail Count</label>
                      <input type="number" v-model.number="cond.fail_count" min="1" max="100"
                        @change="onFailCountChange(cond)" />
                    </div>
                    <div class="form-group" data-help="targetForm.tw"
                      :data-help-args="JSON.stringify({ fail_count: cond.fail_count, interval_s: cond.interval_s })">
                      <label>Time Window</label>
                      <input type="number" v-model.number="cond.fail_window"
                        v-custom-validity="failWindowError(cond)"
                        :min="minFailWindow(cond)" :max="maxFailWindow"
                        :disabled="cond.fail_count <= 1"
                        :title="cond.fail_count <= 1 ? 'Set Fail Count > 1 to enable window' : 'Time window for consecutive failures'" />
                    </div>
                  </div>
                  <p v-if="cond.field === 'metrics.packet_loss'" class="text-muted input-hint">{{ packetLossHint(cond) }}</p>
                </div>
              </template>

              <button type="button" class="btn btn-sm btn-add-cond" data-help="targetForm.addcond" @click="addConditionToGroup(group.groupIdx)">+ Add condition to group</button>
            </div>
          </template>
        </div>

        <!-- Alert recipients -->
        <div class="recipients-section">
          <div class="conditions-header">
            <strong data-help="targetForm.rcp">Alert Recipients</strong>
          </div>
          <p v-if="allUsersError" class="text-muted" style="font-size: 0.85rem; color: #dc2626;">Could not load users.</p>
          <p v-else-if="allUsers.length === 0" class="text-muted" style="font-size: 0.85rem;">No users available.</p>
          <div v-else class="recipient-list">
            <label v-for="u in allUsers" :key="u.id" class="recipient-item">
              <input type="checkbox" :checked="selectedRecipients.includes(u.id)" @change="toggleRecipient(u.id)" />
              <span>{{ u.username }}</span>
              <span v-if="u.email" class="text-muted recipient-email">{{ u.email }}</span>
            </label>
          </div>
        </div>

        <div class="form-actions" data-help="targetForm.save">
          <button type="submit" class="btn btn-primary" :disabled="form.conditions.length === 0">Save</button>
          <button type="button" class="btn" @click="close">Cancel</button>
        </div>
      </form>
    </div>

    <!-- Asked before a recipe replaces the form's conditions (design §4.7). A
         dialog scope, so help's marks for the form under it are not drawn. -->
    <div v-if="pendingRecipe" class="recipe-confirm" data-help-scope="dialog" @click.self="pendingRecipe = null">
      <div class="recipe-confirm-card" role="alertdialog" aria-modal="true" aria-labelledby="recipe-confirm-text">
        <p id="recipe-confirm-text" class="recipe-confirm-text">Replace the {{ form.conditions.length }} condition{{ form.conditions.length === 1 ? '' : 's' }} in this form with this recipe? On a saved target, Save then deletes the replaced checks and their history, SLA included.</p>
        <div class="recipe-confirm-actions">
          <button type="button" class="btn btn-danger" @click="putRecipe(pendingRecipe)">Replace</button>
          <button ref="recipeCancel" type="button" class="btn" @click="pendingRecipe = null">Cancel</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Modal */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.3);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  z-index: 100;
  overflow-y: auto;
  padding: 2rem 1rem;
}
.modal-card {
  background: #fff;
  border-radius: 8px;
  padding: 1.5rem;
  width: 100%;
  max-width: 420px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.15);
}
.modal-card h3 { margin-bottom: 1rem; }
.modal-wide { max-width: 640px; }
.modal-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  margin-bottom: 1rem;
}
.modal-card .modal-title-row h3 { margin-bottom: 0; }

/* In-app help (design in_app_help.md §4.5). "? Help" is help's turquoise outline
   button, never the app's orange. While help is on, its bar runs across the top
   of the overlay and the form sits under it; the plain-words card goes above
   Condition Groups, or at 1280px and wider (help-wide) left of the form, with the
   right margin kept for notes. */
.btn-form-help {
  gap: 6px;
  border-color: var(--help);
  color: var(--help-dk);
  font-weight: 600;
}
.btn-form-help:hover,
.btn-form-help.is-on { background: var(--help-tint); }
.form-help-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--help-dk);
  color: #fff;
  font-size: 10.5px;
  font-weight: 800;
  line-height: 1;
}
.form-help-icon::before { content: '?'; }
.modal-overlay.help-on {
  display: block;
  padding-top: 0;
}
.modal-overlay.help-on > .help-bar { margin: 0 -1rem 2rem; }
.modal-overlay.help-on > .modal-card { margin: 0 auto; }
.modal-overlay.help-wide {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 640px) minmax(0, 1fr);
  column-gap: 24px;
  align-items: start;
}
.modal-overlay.help-wide > .help-bar { grid-column: 1 / -1; }
.modal-overlay.help-wide > .form-rule-card {
  grid-column: 1;
  grid-row: 2;
  justify-self: end;
  width: 100%;
  max-width: 340px;
  max-height: calc(100vh - 2rem);
  overflow-y: auto;
  position: sticky;
  top: 1rem;
}
.modal-overlay.help-wide > .modal-card {
  grid-column: 2;
  grid-row: 2;
  margin: 0;
}
.form-rule-card-inline { margin-top: 1rem; }

.form-group-wide {
  width: 75%;
}
.form-group-wide textarea {
  width: 100%;
  resize: vertical;
}

.form-actions {
  display: flex;
  gap: 0.5rem;
  margin-top: 1rem;
}

/* Checkbox alignment */
.checkbox-group {
  display: flex;
  align-items: center;
}
.checkbox-label {
  display: inline-flex !important;
  align-items: center;
  gap: 0.375rem;
  cursor: pointer;
}
.checkbox-label input[type="checkbox"] {
  width: auto;
  margin: 0;
}

/* Conditions */
.conditions-section {
  margin-top: 1rem;
  border-top: 1px solid #e2e8f0;
  padding-top: 0.75rem;
}
.conditions-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}
.conditions-buttons {
  display: flex;
  gap: 0.5rem;
}
/* Room for the ? mark help draws left of "+ Add Group or Condition" */
.modal-overlay.help-on .conditions-buttons { gap: 32px; }

/* Rule recipes (design §4.7): styled as help, because the recipes are the
   assistant, and never Bekci's orange. A field a recipe left for the person is
   dashed turquoise with an italic placeholder until it has a value. */
.btn-recipe {
  border-color: var(--help-line);
  background: var(--help-tint);
  color: var(--help-dk);
  font-weight: 600;
}
.btn-recipe:hover {
  border-color: var(--help);
  background: var(--help-tint);
}
.recipe-banner {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 0.5rem;
  padding: 9px 12px;
  border: 1px solid var(--help-line);
  border-radius: 8px;
  background: var(--help-tint);
  color: var(--help-dk);
  font-size: 0.85rem;
  line-height: 1.45;
}
.recipe-banner b { color: #0f172a; }
.recipe-undo {
  margin-left: auto;
  padding: 0;
  border: 0;
  background: none;
  color: var(--help-dk);
  font: inherit;
  font-weight: 600;
  white-space: nowrap;
  cursor: pointer;
}
.recipe-undo:hover { text-decoration: underline; }
.form-group input.need-field:placeholder-shown {
  border: 1px dashed var(--help);
  background: var(--help-tint);
}
.form-group input.need-field:placeholder-shown:focus {
  border-color: var(--help);
  box-shadow: 0 0 0 2px rgba(13, 148, 136, 0.18);
}
.form-group input.need-field::placeholder {
  color: var(--help-dk);
  font-style: italic;
  opacity: 1;
}
.recipe-confirm {
  position: fixed;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
  background: rgba(15, 23, 42, 0.3);
}
.recipe-confirm-card {
  width: 100%;
  max-width: 460px;
  padding: 1.25rem 1.5rem;
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.2);
}
.recipe-confirm-text { font-size: 0.95rem; line-height: 1.5; color: #1e293b; }
.recipe-confirm-actions {
  display: flex;
  gap: 0.5rem;
  margin-top: 1rem;
}

/* Group card */
.group-card {
  border: 1px solid #cbd5e1;
  border-left: 4px solid #3b82f6;
  border-radius: 6px;
  padding: 0.5rem 0.75rem;
  background: #f8fafc;
}
.group-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
}
.group-label {
  font-weight: 600;
  font-size: 0.85rem;
  color: #1e40af;
}
.group-header .btn-danger {
  margin-left: auto;
}

/* AND/OR toggle */
.operator-toggle {
  display: inline-flex;
  border: 1px solid #cbd5e1;
  border-radius: 4px;
  overflow: hidden;
}
.op-btn {
  padding: 0.2rem 0.6rem;
  font-size: 0.75rem;
  font-weight: 600;
  border: none;
  background: #fff;
  color: #64748b;
  cursor: pointer;
}
.op-btn.active {
  background: #3b82f6;
  color: #fff;
}
.op-btn:not(:last-child) {
  border-right: 1px solid #cbd5e1;
}

/* OR divider between groups */
.or-divider {
  text-align: center;
  margin: 0.75rem 0;
}
.or-pill {
  display: inline-block;
  background: #ea580c;
  color: #fff;
  font-weight: 700;
  font-size: 0.75rem;
  padding: 0.2rem 0.75rem;
  border-radius: 12px;
  letter-spacing: 0.05em;
}

/* Intra-group operator label */
.intra-group-op {
  text-align: center;
  font-size: 0.75rem;
  font-weight: 600;
  color: #94a3b8;
  margin: 0.25rem 0;
}

/* Add condition to group button */
.btn-add-cond {
  margin-top: 0.5rem;
  color: #3b82f6;
  border-color: #bfdbfe;
}
.btn-add-cond:hover {
  background: #eff6ff;
}

.condition-card {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  padding: 0.5rem;
  margin-left: 0.75rem;
  font-size: 0.85rem;
}
.condition-card .form-row {
  gap: 0.5rem;
}
.condition-card .form-group label {
  font-size: 0.78rem;
}
.condition-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.35rem;
}
.condition-num {
  font-weight: 600;
  color: #3730a3;
  font-size: 0.8rem;
  background: #e0e7ff;
  padding: 0.1rem 0.4rem;
  border-radius: 3px;
}

.alert-divider {
  font-size: 0.8rem;
  font-weight: 600;
  color: #64748b;
  margin: 0.5rem 0;
  padding-top: 0.5rem;
  border-top: 1px dashed #e2e8f0;
}

.form-row-5 {
  grid-template-columns: 1fr 1fr 0.7fr 0.7fr 0.85fr;
}
.form-row-5 > .form-group > label {
  padding-left: 0.25rem;
}

/* Recipients */
.recipients-section {
  margin-top: 1rem;
  border-top: 1px solid #e2e8f0;
  padding-top: 0.75rem;
}
.recipient-list {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}
.recipient-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
  cursor: pointer;
}
.recipient-item input[type="checkbox"] {
  width: auto;
  margin: 0;
}
.recipient-email {
  font-size: 0.75rem;
}

.input-hint {
  display: block;
  font-size: 0.75rem;
  margin-top: 0.25rem;
}

.validation-warning {
  padding: 0.5rem 0.75rem;
  font-size: 0.85rem;
  color: #b45309;
  background: #fef3c7;
  border: 1px solid #fde68a;
  border-radius: 4px;
}

.btn-danger {
  color: #991b1b;
  border-color: #fca5a5;
}
.btn-danger:hover { background: #fee2e2; }
</style>

<style>
/* While the target form shows help, its own help bar is the one in use: the
   page's bar under the header would show through the overlay's dim backdrop as
   a second bar, so it is hidden (keeping its place, so nothing under it moves). */
body:has(.modal-overlay.help-on) .layout-v > .help-bar { visibility: hidden; }
</style>
