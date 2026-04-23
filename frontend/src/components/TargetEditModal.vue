<script setup>
import { ref, computed, watch } from 'vue'
import api from '../api'
import TagChipInput from './TagChipInput.vue'

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

const checkTypes = [
  { value: 'http', label: 'HTTP/HTTPS' },
  { value: 'tcp', label: 'TCP' },
  { value: 'ping', label: 'Ping (ICMP)' },
  { value: 'dns', label: 'DNS' },
  { value: 'page_hash', label: 'Page Hash' },
  { value: 'tls_cert', label: 'TLS Certificate' },
  { value: 'snmp_v2c', label: 'SNMP v2c' },
  { value: 'snmp_v3', label: 'SNMP v3' },
]

function getEmptyForm() {
  return {
    name: '', host: '', description: '', enabled: true,
    category: '', preferred_check_type: '',
    notes: '', contacts: '', project: '', location: '',
    tags: [],
    conditions: []
  }
}

function getDefaultConfig(type_) {
  switch (type_) {
    case 'http': return { scheme: 'https', port: 0, endpoint: '/', expect_status: 200, skip_tls_verify: false, timeout_s: 10 }
    case 'tcp': return { port: 80, timeout_s: 5 }
    case 'ping': return { count: 3, timeout_s: 5 }
    case 'dns': return { query: '', record_type: 'A', expect_value: '', nameserver: '' }
    case 'page_hash': return { scheme: 'https', endpoint: '/', baseline_hash: '', timeout_s: 10 }
    case 'tls_cert': return { port: 443, warn_days: 30, timeout_s: 10 }
    case 'snmp_v2c': return { port: 161, timeout_s: 5 }
    case 'snmp_v3': return { port: 161, timeout_s: 5 }
    default: return {}
  }
}

function getDefaultCondition(conditionGroup = 0, groupOperator = 'AND') {
  return {
    check_id: '',
    check_type: 'ping',
    check_name: '',
    config: getDefaultConfig('ping'),
    interval_s: 60,
    field: 'status',
    comparator: 'eq',
    value: 'down',
    fail_count: 1,
    fail_window: 0,
    condition_group: conditionGroup,
    group_operator: groupOperator,
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

function onFailCountChange(cond) {
  if (cond.fail_count > 1) {
    const minWindow = cond.fail_count * cond.interval_s
    if (!cond.fail_window || cond.fail_window < minWindow) {
      cond.fail_window = minWindow
    }
  } else {
    cond.fail_window = 0
  }
}

function onIntervalChange(cond) {
  if (cond.fail_count > 1) {
    const minWindow = cond.fail_count * cond.interval_s
    if (cond.fail_window < minWindow) {
      cond.fail_window = minWindow
    }
  }
}

function onConditionTypeChange(cond) {
  if (!cond.check_id) {
    cond.config = getDefaultConfig(cond.check_type)
  }
}

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
  <div v-if="show" class="modal-overlay">
    <div class="modal-card modal-wide">
      <h3>{{ targetId ? 'Edit Target' : cloneSourceId ? 'Clone Target' : 'New Target' }}</h3>
      <div v-if="formError" class="error-msg" style="margin-bottom: 0.75rem;">{{ formError }}</div>
      <form @submit.prevent="saveTarget">
        <!-- Target fields -->
        <div class="form-row">
          <div class="form-group">
            <label class="required">Name</label>
            <input v-model="form.name" required placeholder="e.g. Web Server" />
          </div>
          <div class="form-group">
            <label class="required">Host</label>
            <input v-model="form.host" required placeholder="e.g. google.com" />
          </div>
        </div>
        <div class="form-group">
          <label>Description</label>
          <input v-model="form.description" placeholder="Optional description" />
        </div>
        <div class="form-group">
          <label class="required">Category</label>
          <select v-model="form.category" required>
            <option value="" disabled>Select category</option>
            <option v-for="c in categoryOptions" :key="c.id" :value="c.value">{{ c.value }}</option>
          </select>
        </div>
        <div v-if="form.conditions.length > 1" class="form-group">
          <label>Preferred Check (for SLA &amp; dashboard)</label>
          <select v-model="form.preferred_check_type">
            <option v-for="t in availableCheckTypes" :key="t.value" :value="t.value">{{ t.label }}</option>
          </select>
          <span class="text-muted input-hint">Which check type drives the SLA badge and dashboard uptime display</span>
        </div>

        <!-- Notes & Contacts -->
        <div class="form-group form-group-wide">
          <label>Notes</label>
          <textarea v-model="form.notes" rows="4" placeholder="Optional notes about this target"></textarea>
        </div>
        <div class="form-group form-group-wide">
          <label>Contacts</label>
          <textarea v-model="form.contacts" rows="4" placeholder="Optional contact information"></textarea>
        </div>

        <!-- Project & Location tags -->
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

        <div class="form-group checkbox-group">
          <label class="checkbox-label"><input type="checkbox" v-model="form.enabled" /> Enabled</label>
        </div>

        <!-- Condition Groups section -->
        <div class="conditions-section">
          <div class="conditions-header">
            <strong>Condition Groups</strong>
            <button type="button" class="btn btn-sm btn-primary" @click="addGroup">+ Add Group or Condition</button>
          </div>

          <div v-if="form.conditions.length === 0" class="validation-warning">
            At least one condition is required. Add a group to get started.
          </div>

          <template v-for="(group, gIdx) in conditionGroups" :key="group.groupIdx">
            <!-- OR divider between groups -->
            <div v-if="gIdx > 0" class="or-divider"><span class="or-pill">OR</span></div>

            <div class="group-card">
              <div class="group-header">
                <span class="group-label">Group {{ gIdx + 1 }}</span>
                <div v-if="group.conditions.length > 1" class="operator-toggle">
                  <button type="button" :class="['op-btn', { active: group.operator === 'AND' }]" @click="setGroupOperator(group.groupIdx, 'AND')">AND</button>
                  <button type="button" :class="['op-btn', { active: group.operator === 'OR' }]" @click="setGroupOperator(group.groupIdx, 'OR')">OR</button>
                </div>
                <button type="button" class="btn btn-sm btn-danger" @click="removeGroup(group.groupIdx)">Remove Group</button>
              </div>

              <template v-for="(cond, cIdx) in group.conditions" :key="cond.check_id || cIdx">
                <!-- Within-group operator label -->
                <div v-if="cIdx > 0" class="intra-group-op">{{ group.operator }}</div>

                <div class="condition-card">
                  <div class="condition-card-header">
                    <span class="condition-num">{{ cond.check_type.toUpperCase() }}</span>
                    <button type="button" class="btn btn-sm btn-danger" @click="removeCondition(cond)">Remove</button>
                  </div>

                  <!-- Check fields -->
                  <div class="form-row">
                    <div class="form-group">
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
                  <div class="form-group">
                    <label>Interval (seconds)</label>
                    <input type="number" v-model.number="cond.interval_s" min="10" style="max-width: 120px;" @change="onIntervalChange(cond)" />
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
                        <input v-model="cond.config.endpoint" placeholder="/" />
                      </div>
                      <div class="form-group">
                        <label>Expected Status</label>
                        <input type="number" v-model.number="cond.config.expect_status" />
                      </div>
                    </div>
                    <div class="form-row">
                      <div class="form-group">
                        <label>Timeout (s)</label>
                        <input type="number" v-model.number="cond.config.timeout_s" min="1" max="60" />
                      </div>
                      <div class="form-group checkbox-group">
                        <label class="checkbox-label"><input type="checkbox" v-model="cond.config.skip_tls_verify" /> Skip TLS Verify</label>
                      </div>
                    </div>
                  </template>

                  <template v-if="cond.check_type === 'tcp'">
                    <div class="form-row">
                      <div class="form-group">
                        <label>Port</label>
                        <input type="number" v-model.number="cond.config.port" min="1" max="65535" required />
                      </div>
                      <div class="form-group">
                        <label>Timeout (s)</label>
                        <input type="number" v-model.number="cond.config.timeout_s" min="1" max="60" />
                      </div>
                    </div>
                  </template>

                  <template v-if="cond.check_type === 'ping'">
                    <div class="form-row">
                      <div class="form-group">
                        <label>Ping Count</label>
                        <input type="number" v-model.number="cond.config.count" min="1" max="10" />
                      </div>
                      <div class="form-group">
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
                      <div class="form-group">
                        <label>Expected Value (optional)</label>
                        <input v-model="cond.config.expect_value" placeholder="e.g. 1.2.3.4" />
                      </div>
                      <div class="form-group">
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
                    <div class="form-group">
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
                      <div class="form-group">
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
                      <div class="form-group">
                        <label>Timeout (s)</label>
                        <input type="number" v-model.number="cond.config.timeout_s" placeholder="5" />
                      </div>
                    </div>
                    <p class="text-muted input-hint">SNMP credentials are configured globally in Settings.</p>
                  </template>

                  <!-- Alert criteria divider -->
                  <div class="alert-divider">Alert when...</div>
                  <div class="form-row form-row-5">
                    <div class="form-group">
                      <label>Field</label>
                      <select v-model="cond.field">
                        <option value="status">Status</option>
                        <option value="response_ms">Response (ms)</option>
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
                      <input v-model="cond.value" placeholder="down" />
                    </div>
                    <div class="form-group">
                      <label>Fail Count</label>
                      <input type="number" v-model.number="cond.fail_count" min="1"
                        @change="onFailCountChange(cond)" />
                    </div>
                    <div class="form-group">
                      <label>Time Window</label>
                      <input type="number" v-model.number="cond.fail_window"
                        :min="cond.fail_count * cond.interval_s" max="1800"
                        :disabled="cond.fail_count <= 1"
                        :title="cond.fail_count <= 1 ? 'Set Fail Count > 1 to enable window' : 'Time window for consecutive failures'" />
                    </div>
                  </div>
                </div>
              </template>

              <button type="button" class="btn btn-sm btn-add-cond" @click="addConditionToGroup(group.groupIdx)">+ Add condition to group</button>
            </div>
          </template>
        </div>

        <!-- Alert recipients -->
        <div class="recipients-section">
          <div class="conditions-header">
            <strong>Alert Recipients</strong>
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

        <div class="form-actions">
          <button type="submit" class="btn btn-primary" :disabled="form.conditions.length === 0">Save</button>
          <button type="button" class="btn" @click="close">Cancel</button>
        </div>
      </form>
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
