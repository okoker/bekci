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
const showGuide = ref(false)
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
    category: '', preferred_check_type: '',
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
  // Return sorted array of { groupIdx, operator, conditions }
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
  // Find the operator used by existing conditions in this group
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
      category: data.category || 'Other',
      preferred_check_type: data.preferred_check_type || '',
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
    showForm.value = true
  } catch (e) {
    error.value = 'Failed to load target details'
  }
}

// Save
async function saveTarget() {
  error.value = ''
  success.value = ''
  try {
    const payload = {
      name: form.value.name,
      host: form.value.host,
      description: form.value.description,
      enabled: form.value.enabled,
      operator: 'AND', // kept for backward compat
      category: form.value.category,
      preferred_check_type: form.value.preferred_check_type,
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

// Delete confirmation modal
const showDeleteConfirm = ref(false)
const pendingDeleteId = ref(null)

function deleteTarget(id) {
  pendingDeleteId.value = id
  showDeleteConfirm.value = true
}

async function confirmDelete() {
  showDeleteConfirm.value = false
  error.value = ''
  success.value = ''
  const id = pendingDeleteId.value
  pendingDeleteId.value = null
  try {
    await api.delete(`/targets/${id}`)
    await loadTargets()
    success.value = 'Target deleted'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to delete target'
  }
}

function onFailCountChange(cond) {
  if (cond.fail_count > 1 && (!cond.fail_window || cond.fail_window < cond.interval_s)) {
    cond.fail_window = cond.interval_s
  }
  if (cond.fail_count <= 1) {
    cond.fail_window = 0
  }
}

function onConditionTypeChange(cond) {
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

async function saveRecipients(targetId) {
  await api.put(`/targets/${targetId}/recipients`, { user_ids: selectedRecipients.value })
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
  error.value = ''
  success.value = ''
  try {
    await api.post(`/checks/${checkId}/run`)
    success.value = 'Check queued for immediate run'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to run check'
  }
}

async function runAllChecks(targetId) {
  error.value = ''
  success.value = ''
  try {
    let checks = targetChecks.value[targetId]
    if (!checks) {
      const { data } = await api.get(`/targets/${targetId}/checks`)
      checks = data || []
      targetChecks.value[targetId] = checks
    }
    if (checks.length === 0) {
      error.value = 'No checks configured for this target'
      return
    }
    for (const c of checks) {
      await api.post(`/checks/${c.id}/run`)
    }
    success.value = `${checks.length} check(s) queued for immediate run`
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to run checks'
  }
}

// Pause / Unpause
const showPauseConfirm = ref(false)
const showUnpauseConfirm = ref(false)
const pendingPauseId = ref(null)

function pauseTarget(id) {
  pendingPauseId.value = id
  showPauseConfirm.value = true
}

async function confirmPause() {
  showPauseConfirm.value = false
  const id = pendingPauseId.value
  pendingPauseId.value = null
  error.value = ''
  success.value = ''
  try {
    await api.post(`/targets/${id}/pause`)
    await loadTargets()
    success.value = 'Target paused'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to pause target'
  }
}

function unpauseTarget(id) {
  pendingPauseId.value = id
  showUnpauseConfirm.value = true
}

async function confirmUnpause() {
  showUnpauseConfirm.value = false
  const id = pendingPauseId.value
  pendingPauseId.value = null
  error.value = ''
  success.value = ''
  try {
    await api.post(`/targets/${id}/unpause`)
    await loadTargets()
    success.value = 'Target unpaused — checks running now'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to unpause target'
  }
}

function isTargetPaused(t) {
  return t.paused_at != null
}

function formatInterval(s) {
  if (s >= 3600) return `${Math.floor(s / 3600)}h`
  if (s >= 60) return `${Math.floor(s / 60)}m`
  return `${s}s`
}

function stateClass(state) {
  if (state === 'healthy') return 'badge-active'
  if (state === 'unhealthy') return 'badge-suspended'
  if (state === 'paused') return 'badge-paused'
  return 'badge-unknown'
}

function categoryClass(cat) {
  if (cat === 'Security') return 'badge-cat-security'
  if (cat === 'Network') return 'badge-cat-network'
  if (cat === 'Physical Security') return 'badge-cat-physical'
  if (cat === 'Key Services') return 'badge-cat-server'
  return 'badge-cat-other'
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
            <tr :class="{ 'row-expanded': expandedTargetId === t.id, 'row-disabled': !t.enabled }" @click="toggleTarget(t.id)" style="cursor: pointer;">
              <td>
                <span class="expand-icon">{{ expandedTargetId === t.id ? '&#9660;' : '&#9654;' }}</span>
                {{ t.name }}
              </td>
              <td>{{ t.host }}</td>
              <td>
                <span v-if="isTargetPaused(t)" class="badge badge-paused">paused</span>
                <span v-else-if="t.state" :class="['badge', stateClass(t.state?.current_state)]">
                  {{ t.state?.current_state || '—' }}
                </span>
                <span v-else class="text-muted">—</span>
              </td>
              <td><span :class="['badge', categoryClass(t.category)]">{{ t.category }}</span></td>
              <td>{{ t.condition_count || 0 }}</td>
              <td>
                <span :class="['badge', t.enabled ? 'badge-active' : 'badge-suspended']">{{ t.enabled ? 'yes' : 'no' }}</span>
                <span v-if="!t.enabled" class="badge badge-disabled">DISABLED</span>
              </td>
              <td class="actions" @click.stop>
                <template v-if="auth.isOperator">
                  <button v-if="isTargetPaused(t)" class="btn btn-sm btn-unpause" @click="unpauseTarget(t.id)">Unpause</button>
                  <button v-else class="btn btn-sm btn-pause" @click="pauseTarget(t.id)">Pause</button>
                  <button class="btn btn-sm" @click="runAllChecks(t.id)">Run Now</button>
                  <button class="btn btn-sm" @click="openForm(t)">Edit</button>
                  <button class="btn btn-sm btn-danger" @click="deleteTarget(t.id)">Delete</button>
                </template>
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
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="c in targetChecks[t.id]" :key="c.id">
                      <td>{{ c.name }}</td>
                      <td>
                        <span class="badge badge-type">{{ c.type }}</span>
                        <span v-if="c.type === t.preferred_check_type" class="badge badge-primary-check" title="Primary check used for SLA &amp; dashboard status">PRIMARY</span>
                      </td>
                      <td>{{ formatInterval(c.interval_s) }}</td>
                      <td><span :class="['badge', c.enabled ? 'badge-active' : 'badge-suspended']">{{ c.enabled ? 'yes' : 'no' }}</span></td>
                    </tr>
                  </tbody>
                </table>
              </td>
            </tr>
          </template>
        </tbody>
      </table>

      <!-- Guide -->
      <div v-if="!loading" class="guide-bar">
        <button type="button" class="guide-btn" @click="showGuide = !showGuide">
          <span class="guide-btn-q">?</span>
          {{ showGuide ? 'Hide guide' : 'Target creation guide' }}
          <span class="guide-chevron" :class="{ open: showGuide }">&#9662;</span>
        </button>
      </div>

      <div v-if="showGuide" class="guide">

        <p class="guide-intro">
          A <strong>target</strong> is anything you monitor &mdash; a server, website, network device, or service.
          Each target has a <strong>host</strong> and one or more <strong>conditions</strong>.
          A condition = <em>check</em> (what to test) + <em>alert criteria</em> (when to trigger).
        </p>

        <div class="guide-two-col">
          <div class="guide-two-col-left">
            <h5 class="guide-h" style="margin-top:0;">Check Types</h5>
            <div class="guide-checks">
              <div class="guide-ck"><code>PING</code><div><strong>ICMP Ping</strong> &mdash; sends echo packets to test basic reachability<span class="guide-meta">Config: count, timeout_s &middot; Metrics: packet_loss, avg_rtt_ms</span></div></div>
              <div class="guide-ck"><code>HTTP</code><div><strong>HTTP/HTTPS</strong> &mdash; requests a URL and checks the response status code<span class="guide-meta">Config: scheme, port, endpoint, expect_status, skip_tls_verify &middot; Metrics: status_code</span></div></div>
              <div class="guide-ck"><code>TCP</code><div><strong>TCP Port</strong> &mdash; attempts a TCP connection to verify a port is open<span class="guide-meta">Config: port, timeout_s</span></div></div>
              <div class="guide-ck"><code>DNS</code><div><strong>DNS Lookup</strong> &mdash; resolves a record, optionally verifies the result<span class="guide-meta">Config: query, record_type, expect_value, nameserver &middot; Metrics: resolved</span></div></div>
              <div class="guide-ck"><code>HASH</code><div><strong>Page Hash</strong> &mdash; compares page SHA-256 against baseline to detect content changes<span class="guide-meta">Config: scheme, endpoint, baseline_hash (empty = auto-capture) &middot; Metrics: hash</span></div></div>
              <div class="guide-ck"><code>TLS</code><div><strong>TLS Certificate</strong> &mdash; inspects certificate expiry, alerts when expiring soon<span class="guide-meta">Config: port, warn_days &middot; Metrics: days_left, issuer, subject, not_after</span></div></div>
            </div>
          </div>
          <div class="guide-two-col-right">
            <h5 class="guide-h" style="margin-top:0;">Alert Criteria</h5>
            <div class="guide-alert-list">
              <div class="guide-al"><strong>Field</strong><span>What to evaluate from the check result: <code>status</code> (up/down) or <code>response_ms</code> (response time in milliseconds)</span></div>
              <div class="guide-al"><strong>Comparator</strong><span>How to compare: <code>equals</code>, <code>not equals</code>, <code>greater than</code>, <code>less than</code></span></div>
              <div class="guide-al"><strong>Value</strong><span>The expected value: <code>down</code>, <code>up</code>, or a number like <code>500</code>, <code>5000</code> (for response_ms)</span></div>
              <div class="guide-al"><strong>Fail Count</strong><span>How many consecutive failures before triggering the alert. Set to <code>1</code> for immediate, or higher (e.g. <code>3</code>) to avoid false positives</span></div>
              <div class="guide-al"><strong>Time Window</strong><span>Window (in seconds) to count consecutive failures. Only active when fail count &gt; 1. Example: fail count <code>3</code> + window <code>600</code> = 3 failures within 10 minutes</span></div>
            </div>
            <p class="guide-note">Most common: <code>status</code> <code>equals</code> <code>down</code> &mdash; alerts when the target is down.</p>
          </div>
        </div>

        <h5 class="guide-h">How It Comes Together</h5>
        <div class="guide-flow">
          <div class="guide-flow-step">
            <div class="guide-flow-num">1</div>
            <div class="guide-flow-content">
              <strong>Create a group</strong>
              <span>Click <code>+ Add Group or Condition</code>. This creates Group 1 with one empty check slot.</span>
            </div>
          </div>
          <div class="guide-flow-arrow">&#8595;</div>
          <div class="guide-flow-step">
            <div class="guide-flow-num">2</div>
            <div class="guide-flow-content">
              <strong>Add checks to the group</strong>
              <span>Pick a check type, configure it, set alert criteria. Click <code>+ Add condition to group</code> for more checks in the same group. Toggle AND/OR between them.</span>
            </div>
          </div>
          <div class="guide-flow-arrow">&#8595;</div>
          <div class="guide-flow-step">
            <div class="guide-flow-num">3</div>
            <div class="guide-flow-content">
              <strong>Add another group</strong>
              <span>Click <code>+ Add Group or Condition</code> again. A new group appears with an <em class="guide-or-accent">OR</em> divider between them. Groups are always OR&rsquo;d together.</span>
            </div>
          </div>
          <div class="guide-flow-arrow">&#8595;</div>
          <div class="guide-flow-step">
            <div class="guide-flow-num">4</div>
            <div class="guide-flow-content">
              <strong>Result</strong>
              <span>Target is unhealthy if <em>any</em> group triggers. Within each group, conditions combine with the AND/OR you chose.</span>
            </div>
          </div>
        </div>

        <h5 class="guide-h">Condition Grouping (AND / OR)</h5>
        <p>Within a group you choose <strong>AND</strong> or <strong>OR</strong>. Between groups it is always <strong>OR</strong>.</p>

        <div class="guide-diagram">
          <div class="guide-dg">
            <div class="guide-dg-head">Group 1 <span class="guide-dg-op">AND</span></div>
            <div class="guide-dg-body">
              <span class="guide-dg-chip">Ping</span>
              <span class="guide-dg-join">AND</span>
              <span class="guide-dg-chip">HTTP</span>
            </div>
            <div class="guide-dg-note">both must fail</div>
          </div>
          <div class="guide-dg-or">OR</div>
          <div class="guide-dg">
            <div class="guide-dg-head">Group 2</div>
            <div class="guide-dg-body">
              <span class="guide-dg-chip">TCP :443</span>
            </div>
            <div class="guide-dg-note">TCP alone fails</div>
          </div>
        </div>
        <p class="guide-formula">Unhealthy if <code>(Ping AND HTTP)</code> <strong>OR</strong> <code>(TCP)</code></p>

        <div class="guide-diagram" style="margin-top: 0.75rem;">
          <div class="guide-dg">
            <div class="guide-dg-head">Group 1 <span class="guide-dg-op guide-dg-op-or">OR</span></div>
            <div class="guide-dg-body">
              <span class="guide-dg-chip">HTTP</span>
              <span class="guide-dg-join guide-dg-join-or">OR</span>
              <span class="guide-dg-chip">TLS Cert</span>
            </div>
            <div class="guide-dg-note">either failing triggers the alert</div>
          </div>
        </div>
        <p class="guide-formula">Unhealthy if <code>HTTP fails</code> <strong>OR</strong> <code>TLS cert expiring</code></p>

        <h5 class="guide-h">Examples</h5>
        <div class="guide-examples">
          <div class="guide-ex">
            <strong>Web server + cert expiry</strong> &mdash; alert if service is down OR cert is about to expire
            <span class="guide-ex-logic">Group 1 (AND): Ping + HTTP &mdash; both must fail | Group 2: TLS cert (warn 14 days)</span>
            <span class="guide-ex-formula">(Ping AND HTTP) OR (TLS expiring)</span>
          </div>
          <div class="guide-ex">
            <strong>Security monitoring</strong> &mdash; detect defacement OR DNS hijacking
            <span class="guide-ex-logic">Group 1: Page Hash &mdash; detects content changes | Group 2: DNS &mdash; expects specific IP</span>
            <span class="guide-ex-formula">(Hash changed) OR (DNS mismatch)</span>
          </div>
          <div class="guide-ex">
            <strong>Redundant paths</strong> &mdash; alert only when all paths fail
            <span class="guide-ex-logic">Group 1 (AND): HTTP :443 + TCP :8080 + Ping &mdash; all three must fail to trigger</span>
            <span class="guide-ex-formula">(HTTP AND TCP AND Ping) &mdash; single AND group</span>
          </div>
          <div class="guide-ex">
            <strong>Mixed: connectivity OR slow response</strong> &mdash; two different failure modes
            <span class="guide-ex-logic">Group 1: Ping (status = down, fail_count: 3) | Group 2: HTTP (response_ms &gt; 5000)</span>
            <span class="guide-ex-formula">(Ping down 3x) OR (HTTP slow &gt; 5s)</span>
          </div>
        </div>

        <h5 class="guide-h">Quick Start</h5>
        <ol class="guide-steps">
          <li>Click <strong>+ Target</strong> &rarr; fill in Name, Host, Category</li>
          <li>Click <strong>+ Add Group or Condition</strong> &rarr; pick check type, configure</li>
          <li>Set alert criteria (usually: status equals down)</li>
          <li>Same group? <strong>+ Add condition to group</strong> &middot; New OR group? <strong>+ Add Group or Condition</strong></li>
          <li>Pick <strong>Alert Recipients</strong> &rarr; <strong>Save</strong></li>
        </ol>

      </div>
    </div>

    <!-- Unified target + conditions form modal -->
    <div v-if="showForm" class="modal-overlay">
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
          <div v-if="form.conditions.length > 1" class="form-group">
            <label>Preferred Check (for SLA &amp; dashboard)</label>
            <select v-model="form.preferred_check_type">
              <option v-for="t in availableCheckTypes" :key="t.value" :value="t.value">{{ t.label }}</option>
            </select>
            <span class="text-muted input-hint">Which check type drives the SLA badge and dashboard uptime display</span>
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
                          :min="cond.interval_s" max="1800"
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
            <button type="submit" class="btn btn-primary" :disabled="form.conditions.length === 0">Save</button>
            <button type="button" class="btn" @click="showForm = false">Cancel</button>
          </div>
        </form>
      </div>
    </div>

    <!-- Delete confirmation modal -->
    <div v-if="showDeleteConfirm" class="modal-overlay" @click.self="showDeleteConfirm = false">
      <div class="modal-card">
        <h3>Delete Target</h3>
        <p>Delete this target and all its checks? This cannot be undone.</p>
        <div class="form-actions">
          <button class="btn btn-danger" @click="confirmDelete">Delete</button>
          <button class="btn" @click="showDeleteConfirm = false">Cancel</button>
        </div>
      </div>
    </div>

    <!-- Pause confirmation modal -->
    <div v-if="showPauseConfirm" class="modal-overlay" @click.self="showPauseConfirm = false">
      <div class="modal-card">
        <h3>Pause Target</h3>
        <p>Testing will stop for this target. SLA will not be affected during pause. Are you sure?</p>
        <div class="form-actions">
          <button class="btn btn-pause" @click="confirmPause">Pause</button>
          <button class="btn" @click="showPauseConfirm = false">Cancel</button>
        </div>
      </div>
    </div>

    <!-- Unpause confirmation modal -->
    <div v-if="showUnpauseConfirm" class="modal-overlay" @click.self="showUnpauseConfirm = false">
      <div class="modal-card">
        <h3>Unpause Target</h3>
        <p>Testing will resume immediately for all checks. Continue?</p>
        <div class="form-actions">
          <button class="btn btn-primary" @click="confirmUnpause">Unpause</button>
          <button class="btn" @click="showUnpauseConfirm = false">Cancel</button>
        </div>
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

.row-expanded > td { background: #e8ecf1; }

.expand-icon {
  display: inline-block;
  width: 1rem;
  font-size: 0.7rem;
  color: #94a3b8;
}

.checks-panel {
  background: #eef2ff;
  padding: 0.75rem 1rem 0.75rem 4rem;
  border-left: 3px solid #6366f1;
}
.checks-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}
.checks-table {
  font-size: 0.8rem;
}
.checks-table th { font-size: 0.7rem; }

.badge-type {
  background: #e0e7ff;
  color: #3730a3;
  font-size: 0.7rem;
  text-transform: uppercase;
}
.badge-primary-check {
  background: #fef3c7;
  color: #92400e;
  font-size: 0.6rem;
  font-weight: 700;
  text-transform: uppercase;
}

.badge-unknown {
  background: #f1f5f9;
  color: #64748b;
}
.badge-paused {
  background: #e0e7ff;
  color: #4338ca;
}
.badge-disabled {
  background: #f1f5f9;
  color: #94a3b8;
  font-size: 0.65rem;
  margin-left: 0.25rem;
}
.row-disabled > td {
  opacity: 0.5;
}
.btn-pause {
  color: #4338ca;
  border-color: #c7d2fe;
}
.btn-pause:hover { background: #e0e7ff; }
.btn-unpause {
  color: #166534;
  border-color: #bbf7d0;
}
.btn-unpause:hover { background: #dcfce7; }

.badge-cat-security {
  background: #ede9fe;
  color: #6d28d9;
}
.badge-cat-network {
  background: #dbeafe;
  color: #1d4ed8;
}
.badge-cat-server {
  background: #fce7f3;
  color: #9d174d;
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
  color: #64748b;
  font-size: 0.8rem;
  background: #e0e7ff;
  color: #3730a3;
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

.form-row-4 {
  grid-template-columns: 1fr 1fr 1fr 1fr;
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

/* ── Guide ── */
.guide-bar {
  display: flex;
  justify-content: center;
  padding: 1.5rem 1rem 1rem;
}
.guide-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.35rem 1rem;
  border: 1px solid #cbd5e1;
  border-radius: 20px;
  background: #fff;
  color: #64748b;
  font-size: 0.8rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s;
}
.guide-btn:hover {
  border-color: #818cf8;
  color: #4338ca;
  background: #eef2ff;
}
.guide-btn-q {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.1rem;
  height: 1.1rem;
  border-radius: 50%;
  background: #e0e7ff;
  color: #4338ca;
  font-size: 0.7rem;
  font-weight: 700;
}
.guide-chevron {
  font-size: 0.6rem;
  transition: transform 0.2s;
}
.guide-chevron.open {
  transform: rotate(180deg);
}

.guide {
  padding: 0 1.25rem 1.25rem;
  font-size: 0.82rem;
  line-height: 1.55;
  color: #334155;
}
.guide p { margin: 0.3rem 0; }
.guide code {
  background: #f1f5f9;
  color: #475569;
  padding: 0.1rem 0.3rem;
  border-radius: 3px;
  font-size: 0.78rem;
}
.guide-intro {
  padding-bottom: 0.75rem;
  border-bottom: 1px solid #e2e8f0;
}
.guide-h {
  margin: 1rem 0 0.4rem;
  font-size: 0.72rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #475569;
}

/* Check types — boxed list */
.guide-checks {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}
.guide-ck {
  display: flex;
  gap: 0.6rem;
  align-items: baseline;
  padding: 0.35rem 0.5rem;
  border: 1px solid #e2e8f0;
  border-radius: 5px;
  background: #fff;
}
.guide-ck > code {
  font-weight: 700;
  font-size: 0.7rem;
  min-width: 3rem;
  color: #475569;
  background: #f1f5f9;
  padding: 0.15rem 0.35rem;
  border-radius: 3px;
  text-align: center;
  flex-shrink: 0;
}
.guide-ck > div { font-size: 0.8rem; }
.guide-meta {
  display: block;
  font-size: 0.72rem;
  color: #94a3b8;
  margin-top: 0.1rem;
}

/* Two-column: check types + alert criteria */
.guide-two-col {
  display: flex;
  gap: 1.5rem;
  align-items: flex-start;
}
.guide-two-col-left {
  flex: 1;
  min-width: 280px;
}
.guide-two-col-right {
  flex: 1;
  min-width: 260px;
}

/* Alert criteria — boxed list */
.guide-alert-list {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}
.guide-al {
  padding: 0.35rem 0.5rem;
  border: 1px solid #e2e8f0;
  border-radius: 5px;
  background: #fff;
  font-size: 0.8rem;
}
.guide-al strong {
  display: block;
  font-size: 0.75rem;
  color: #475569;
  margin-bottom: 0.05rem;
}
.guide-al span {
  font-size: 0.78rem;
  color: #64748b;
  line-height: 1.45;
}
.guide-note {
  font-size: 0.78rem;
  color: #64748b;
  margin-top: 0.35rem;
}

/* Flow diagram — how to build */
.guide-flow {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0;
  margin: 0.5rem 0;
}
.guide-flow-step {
  display: flex;
  gap: 0.5rem;
  align-items: flex-start;
  width: 100%;
  max-width: 480px;
  padding: 0.4rem 0.6rem;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  background: #fff;
}
.guide-flow-num {
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 1.3rem;
  height: 1.3rem;
  border-radius: 50%;
  background: #6366f1;
  color: #fff;
  font-size: 0.7rem;
  font-weight: 700;
  flex-shrink: 0;
  margin-top: 0.1rem;
}
.guide-flow-content {
  font-size: 0.8rem;
}
.guide-flow-content strong {
  display: block;
  font-size: 0.8rem;
  color: #1e293b;
}
.guide-flow-content > span {
  display: block;
  font-size: 0.76rem;
  color: #64748b;
  line-height: 1.4;
}
.guide-flow-arrow {
  font-size: 0.8rem;
  color: #94a3b8;
  line-height: 1;
  padding: 0.1rem 0;
}
.guide-or-accent {
  color: #ea580c;
  font-style: normal;
  font-weight: 700;
}

/* AND/OR diagram */
.guide-diagram {
  display: flex;
  align-items: center;
  gap: 0;
  margin: 0.6rem 0;
  justify-content: center;
}
.guide-dg {
  border: 1.5px solid #cbd5e1;
  border-radius: 6px;
  padding: 0.4rem 0.6rem;
  background: #fff;
  min-width: 110px;
}
.guide-dg-head {
  font-size: 0.72rem;
  font-weight: 700;
  color: #475569;
  margin-bottom: 0.2rem;
}
.guide-dg-op {
  display: inline-block;
  background: #3b82f6;
  color: #fff;
  padding: 0.05rem 0.3rem;
  border-radius: 3px;
  font-size: 0.65rem;
  margin-left: 0.2rem;
}
.guide-dg-op-or { background: #ea580c; }
.guide-dg-body {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}
.guide-dg-chip {
  padding: 0.15rem 0.4rem;
  background: #f1f5f9;
  border: 1px solid #e2e8f0;
  border-radius: 3px;
  font-size: 0.72rem;
  font-weight: 600;
  color: #475569;
}
.guide-dg-join {
  font-size: 0.65rem;
  font-weight: 700;
  color: #3b82f6;
}
.guide-dg-join-or { color: #ea580c; }
.guide-dg-note {
  font-size: 0.68rem;
  color: #94a3b8;
  margin-top: 0.15rem;
  font-style: italic;
}
.guide-dg-or {
  padding: 0.15rem 0.6rem;
  font-weight: 800;
  font-size: 0.75rem;
  color: #ea580c;
}
.guide-formula {
  text-align: center;
  font-size: 0.8rem;
  color: #475569;
  margin: 0.15rem 0;
}

/* Examples */
.guide-examples {
  display: flex;
  flex-direction: column;
}
.guide-ex {
  padding: 0.3rem 0;
  font-size: 0.8rem;
  border-bottom: 1px solid #f1f5f9;
}
.guide-ex:last-child { border-bottom: none; }
.guide-ex-logic {
  display: block;
  font-size: 0.72rem;
  color: #94a3b8;
  margin-top: 0.1rem;
}
.guide-ex-formula {
  display: block;
  font-size: 0.72rem;
  font-weight: 600;
  color: #64748b;
  font-family: monospace;
}

/* Quick start */
.guide-steps {
  padding-left: 1.1rem;
  margin: 0.25rem 0 0;
}
.guide-steps li {
  font-size: 0.8rem;
  margin-bottom: 0.2rem;
  padding-left: 0.15rem;
}
.guide-steps li::marker {
  color: #6366f1;
  font-weight: 700;
  font-size: 0.75rem;
}

@media (max-width: 840px) {
  .guide-two-col { flex-direction: column; gap: 0; }
  .guide-two-col-right .guide-h { margin-top: 1rem; }
}
@media (max-width: 640px) {
  .guide-diagram { flex-direction: column; }
  .guide-dg-or { padding: 0.1rem 0; }
}
</style>
