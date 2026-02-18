<script setup>
import { ref, computed, onMounted } from 'vue'
import { useAuthStore } from '../stores/auth'
import api from '../api'

const auth = useAuthStore()

// State
const targets = ref([])
const expandedTargetId = ref(null)
const loading = ref(false)
const error = ref('')
const success = ref('')
const activeCategory = ref('All')
const categories = ['All', 'Network', 'Security', 'Physical Security', 'Key Services', 'Other']

const filteredTargets = computed(() => {
  if (activeCategory.value === 'All') return targets.value
  return targets.value.filter(t => t.category === activeCategory.value)
})

function categoryCount(cat) {
  if (cat === 'All') return targets.value.length
  return targets.value.filter(t => t.category === cat).length
}

// Form state
const showForm = ref(false)
const editingTarget = ref(null) // null = creating
const form = ref(getEmptyForm())

const checkTypes = [
  { value: 'http', label: 'HTTP/HTTPS' },
  { value: 'tcp', label: 'TCP' },
  { value: 'ping', label: 'Ping (ICMP)' },
  { value: 'dns', label: 'DNS' },
  { value: 'page_hash', label: 'Page Hash' },
  { value: 'tls_cert', label: 'TLS Certificate' },
]

function getEmptyForm() {
  return {
    name: '', host: '', description: '', enabled: true,
    operator: 'AND', category: '',
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
    default: return {}
  }
}

function getDefaultCondition() {
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
  }
}

// Load
async function loadTargets() {
  loading.value = true
  try {
    const { data } = await api.get('/targets')
    targets.value = data
  } catch (e) {
    error.value = 'Failed to load targets'
  } finally {
    loading.value = false
  }
}

// Open form
function openForm(target = null) {
  editingTarget.value = target
  loadAllUsers()
  if (target) {
    // Load full detail for editing
    loadTargetDetail(target.id)
  } else {
    form.value = getEmptyForm()
    selectedRecipients.value = []
    showForm.value = true
  }
  error.value = ''
}

async function loadTargetDetail(id) {
  try {
    const { data } = await api.get(`/targets/${id}`)
    form.value = {
      name: data.name,
      host: data.host,
      description: data.description,
      enabled: data.enabled,
      operator: data.operator || 'AND',
      category: data.category || 'Other',
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
        }
      })
    }
    selectedRecipients.value = data.recipient_ids || []
    showForm.value = true
  } catch (e) {
    error.value = 'Failed to load target details'
  }
}

// Save
async function saveTarget() {
  error.value = ''
  try {
    const payload = {
      name: form.value.name,
      host: form.value.host,
      description: form.value.description,
      enabled: form.value.enabled,
      operator: form.value.operator,
      category: form.value.category,
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
      }))
    }

    let targetId
    if (editingTarget.value) {
      await api.put(`/targets/${editingTarget.value.id}`, payload)
      targetId = editingTarget.value.id
    } else {
      const { data } = await api.post('/targets', payload)
      targetId = data.id
    }
    // Save recipients
    if (targetId) {
      await saveRecipients(targetId)
    }
    showForm.value = false
    await loadTargets()
    success.value = editingTarget.value ? 'Target updated' : 'Target created'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to save target'
  }
}

async function deleteTarget(id) {
  if (!confirm('Delete this target and all its checks?')) return
  try {
    await api.delete(`/targets/${id}`)
    await loadTargets()
    success.value = 'Target deleted'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to delete target'
  }
}

// Conditions management
function addCondition() {
  form.value.conditions.push(getDefaultCondition())
}

function removeCondition(index) {
  form.value.conditions.splice(index, 1)
}

function onConditionTypeChange(index) {
  const cond = form.value.conditions[index]
  // Only reset config if it's a new condition (no check_id)
  if (!cond.check_id) {
    cond.config = getDefaultConfig(cond.check_type)
  }
}

// ── Alert recipients ──
const allUsers = ref([])
const selectedRecipients = ref([])

async function loadAllUsers() {
  try {
    const { data } = await api.get('/users')
    allUsers.value = data.filter(u => u.status === 'active')
  } catch { /* ignore */ }
}

async function loadRecipients(targetId) {
  try {
    const { data } = await api.get(`/targets/${targetId}`)
    selectedRecipients.value = data.recipient_ids || []
  } catch { /* ignore */ }
}

async function saveRecipients(targetId) {
  try {
    await api.put(`/targets/${targetId}/recipients`, { user_ids: selectedRecipients.value })
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to save recipients'
  }
}

function toggleRecipient(userId) {
  const idx = selectedRecipients.value.indexOf(userId)
  if (idx >= 0) {
    selectedRecipients.value.splice(idx, 1)
  } else {
    selectedRecipients.value.push(userId)
  }
}

// Expand for check details
const targetChecks = ref({})

async function toggleTarget(targetId) {
  if (expandedTargetId.value === targetId) {
    expandedTargetId.value = null
    return
  }
  expandedTargetId.value = targetId
  await reloadChecks(targetId)
}

async function reloadChecks(targetId) {
  try {
    const { data } = await api.get(`/targets/${targetId}/checks`)
    targetChecks.value[targetId] = data || []
  } catch (e) {
    error.value = 'Failed to load checks'
  }
}

async function runCheckNow(checkId) {
  try {
    await api.post(`/checks/${checkId}/run`)
    success.value = 'Check queued for immediate run'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to run check'
  }
}

function formatInterval(s) {
  if (s >= 3600) return `${Math.floor(s / 3600)}h`
  if (s >= 60) return `${Math.floor(s / 60)}m`
  return `${s}s`
}

function stateClass(state) {
  if (state === 'healthy') return 'badge-active'
  if (state === 'unhealthy') return 'badge-suspended'
  return 'badge-unknown'
}

function categoryClass(cat) {
  if (cat === 'Security') return 'badge-cat-security'
  if (cat === 'Network') return 'badge-cat-network'
  if (cat === 'Physical Security') return 'badge-cat-physical'
  if (cat === 'Key Services') return 'badge-cat-server'
  return 'badge-cat-other'
}

onMounted(() => loadTargets())
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h2>Targets</h2>
      <button v-if="auth.isOperator" class="btn btn-primary" @click="openForm()">+ Target</button>
    </div>

    <div v-if="error" class="error-msg">{{ error }}</div>
    <div v-if="success" class="success-msg" @click="success = ''">{{ success }}</div>

    <!-- Category filter bar -->
    <div v-if="!loading && targets.length > 0" class="filter-bar">
      <button v-for="cat in categories" :key="cat"
        :class="['filter-btn', { active: activeCategory === cat }]"
        @click="activeCategory = cat">
        {{ cat }} <span class="filter-count">({{ categoryCount(cat) }})</span>
      </button>
    </div>

    <!-- Targets table -->
    <div class="card">
      <div v-if="loading" class="text-muted" style="padding: 1rem;">Loading...</div>
      <div v-else-if="targets.length === 0" class="text-muted" style="padding: 1rem;">No targets yet. Create one to get started.</div>

      <table v-else>
        <thead>
          <tr>
            <th>Name</th>
            <th>Host</th>
            <th>State</th>
            <th>Category</th>
            <th>Conditions</th>
            <th>Enabled</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <template v-for="t in filteredTargets" :key="t.id">
            <tr :class="{ 'row-expanded': expandedTargetId === t.id }" @click="toggleTarget(t.id)" style="cursor: pointer;">
              <td>
                <span class="expand-icon">{{ expandedTargetId === t.id ? '&#9660;' : '&#9654;' }}</span>
                {{ t.name }}
              </td>
              <td>{{ t.host }}</td>
              <td>
                <span v-if="t.state" :class="['badge', stateClass(t.state?.current_state)]">
                  {{ t.state?.current_state || '—' }}
                </span>
                <span v-else class="text-muted">—</span>
              </td>
              <td><span :class="['badge', categoryClass(t.category)]">{{ t.category }}</span></td>
              <td>{{ t.condition_count || 0 }}</td>
              <td><span :class="['badge', t.enabled ? 'badge-active' : 'badge-suspended']">{{ t.enabled ? 'yes' : 'no' }}</span></td>
              <td class="actions" @click.stop>
                <button v-if="auth.isOperator" class="btn btn-sm" @click="openForm(t)">Edit</button>
                <button v-if="auth.isOperator" class="btn btn-sm btn-danger" @click="deleteTarget(t.id)">Delete</button>
              </td>
            </tr>
            <!-- Expanded: checks list -->
            <tr v-if="expandedTargetId === t.id">
              <td colspan="7" class="checks-panel">
                <div class="checks-header">
                  <strong>Checks</strong>
                </div>
                <div v-if="!targetChecks[t.id] || targetChecks[t.id].length === 0" class="text-muted" style="padding: 0.5rem 0;">
                  No checks configured.
                </div>
                <table v-else class="checks-table">
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Type</th>
                      <th>Interval</th>
                      <th>Enabled</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="c in targetChecks[t.id]" :key="c.id">
                      <td>{{ c.name }}</td>
                      <td><span class="badge badge-type">{{ c.type }}</span></td>
                      <td>{{ formatInterval(c.interval_s) }}</td>
                      <td><span :class="['badge', c.enabled ? 'badge-active' : 'badge-suspended']">{{ c.enabled ? 'yes' : 'no' }}</span></td>
                      <td class="actions">
                        <button v-if="auth.isOperator" class="btn btn-sm" @click="runCheckNow(c.id)">Run now</button>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>

    <!-- Unified target + conditions form modal -->
    <div v-if="showForm" class="modal-overlay" @click.self="showForm = false">
      <div class="modal-card modal-wide">
        <h3>{{ editingTarget ? 'Edit Target' : 'New Target' }}</h3>
        <form @submit.prevent="saveTarget">
          <!-- Target fields -->
          <div class="form-row">
            <div class="form-group">
              <label>Name</label>
              <input v-model="form.name" required placeholder="e.g. Web Server" />
            </div>
            <div class="form-group">
              <label>Host</label>
              <input v-model="form.host" required placeholder="e.g. google.com" />
            </div>
          </div>
          <div class="form-group">
            <label>Description</label>
            <input v-model="form.description" placeholder="Optional description" />
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>Operator</label>
              <select v-model="form.operator">
                <option value="AND">AND (all conditions must fail)</option>
                <option value="OR">OR (any condition fails)</option>
              </select>
            </div>
            <div class="form-group">
              <label>Category</label>
              <select v-model="form.category" required>
                <option value="" disabled>Select category</option>
                <option value="Network">Network</option>
                <option value="Security">Security</option>
                <option value="Physical Security">Physical Security</option>
                <option value="Key Services">Key Services</option>
                <option value="Other">Other</option>
              </select>
            </div>
          </div>
          <div class="form-group checkbox-group">
            <label class="checkbox-label"><input type="checkbox" v-model="form.enabled" /> Enabled</label>
          </div>

          <!-- Conditions section -->
          <div class="conditions-section">
            <div class="conditions-header">
              <strong>Conditions</strong>
              <button type="button" class="btn btn-sm btn-primary" @click="addCondition">+ Add Condition</button>
            </div>

            <div v-if="form.conditions.length === 0" class="text-muted" style="padding: 0.5rem 0; font-size: 0.85rem;">
              No conditions yet. Add one to enable monitoring.
            </div>

            <div v-for="(cond, idx) in form.conditions" :key="idx" class="condition-card">
              <div class="condition-card-header">
                <span class="condition-num">#{{ idx + 1 }}</span>
                <button type="button" class="btn btn-sm btn-danger" @click="removeCondition(idx)">Remove</button>
              </div>

              <!-- Check fields -->
              <div class="form-row">
                <div class="form-group">
                  <label>Check Type</label>
                  <select v-model="cond.check_type" @change="onConditionTypeChange(idx)" :disabled="!!cond.check_id">
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
                <input type="number" v-model.number="cond.interval_s" min="10" style="max-width: 120px;" />
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

              <!-- Alert criteria divider -->
              <div class="alert-divider">Alert when...</div>
              <div class="form-row form-row-4">
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
                  <input type="number" v-model.number="cond.fail_count" min="1" />
                </div>
              </div>
            </div>
          </div>

          <!-- Alert recipients -->
          <div class="recipients-section">
            <div class="conditions-header">
              <strong>Alert Recipients</strong>
            </div>
            <p v-if="allUsers.length === 0" class="text-muted" style="font-size: 0.85rem;">No users available.</p>
            <div v-else class="recipient-list">
              <label v-for="u in allUsers" :key="u.id" class="recipient-item">
                <input type="checkbox" :checked="selectedRecipients.includes(u.id)" @change="toggleRecipient(u.id)" />
                <span>{{ u.username }}</span>
                <span v-if="u.email" class="text-muted recipient-email">{{ u.email }}</span>
              </label>
            </div>
          </div>

          <div class="form-actions">
            <button type="submit" class="btn btn-primary">Save</button>
            <button type="button" class="btn" @click="showForm = false">Cancel</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Category filter */
.filter-bar {
  display: flex;
  justify-content: center;
  gap: 0.5rem;
  margin-bottom: 1rem;
  flex-wrap: wrap;
}
.filter-btn {
  background: #f1f5f9;
  border: 1px solid #e2e8f0;
  border-radius: 20px;
  padding: 0.35rem 0.85rem;
  font-size: 0.8rem;
  font-weight: 500;
  color: #64748b;
  cursor: pointer;
  transition: all 0.15s;
}
.filter-btn:hover {
  background: #e2e8f0;
  color: #334155;
}
.filter-btn.active {
  background: #ea580c;
  color: #fff;
  border-color: #ea580c;
}
.filter-count {
  font-weight: 400;
  opacity: 0.7;
}

.row-expanded > td { background: #f8fafc; }

.expand-icon {
  display: inline-block;
  width: 1rem;
  font-size: 0.7rem;
  color: #94a3b8;
}

.checks-panel {
  background: #f8fafc;
  padding: 0.75rem 1rem;
}
.checks-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}
.checks-table {
  font-size: 0.85rem;
}
.checks-table th { font-size: 0.75rem; }

.badge-type {
  background: #e0e7ff;
  color: #3730a3;
  font-size: 0.7rem;
  text-transform: uppercase;
}

.badge-unknown {
  background: #f1f5f9;
  color: #64748b;
}

.badge-cat-security {
  background: #ede9fe;
  color: #6d28d9;
}
.badge-cat-network {
  background: #dbeafe;
  color: #1d4ed8;
}
.badge-cat-server {
  background: #dcfce7;
  color: #166534;
}
.badge-cat-physical {
  background: #fef3c7;
  color: #92400e;
}
.badge-cat-other {
  background: #e5e7eb;
  color: #374151;
}

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

.form-actions {
  display: flex;
  gap: 0.5rem;
  margin-top: 1rem;
}

.btn-danger {
  color: #991b1b;
  border-color: #fca5a5;
}
.btn-danger:hover { background: #fee2e2; }

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

.condition-card {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  padding: 0.75rem;
  margin-bottom: 0.75rem;
}
.condition-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}
.condition-num {
  font-weight: 600;
  color: #64748b;
  font-size: 0.85rem;
}

.alert-divider {
  font-size: 0.8rem;
  font-weight: 600;
  color: #64748b;
  margin: 0.5rem 0;
  padding-top: 0.5rem;
  border-top: 1px dashed #e2e8f0;
}

.form-row-4 {
  grid-template-columns: 1fr 1fr 1fr 1fr;
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
</style>
