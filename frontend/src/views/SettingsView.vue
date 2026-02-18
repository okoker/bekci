<script setup>
import { ref, onMounted, onUnmounted, watch, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import api from '../api'

const auth = useAuthStore()
const router = useRouter()
const activeTab = ref('general')

// ── General tab state ──
const settings = ref({})
const error = ref('')
const success = ref('')
const loading = ref(false)

const labels = {
  session_timeout_hours: 'Session Timeout (hours)',
  history_days: 'History Retention (days)',
  default_check_interval: 'Default Check Interval (seconds)',
  audit_retention_days: 'Audit Log Retention (days)',
  soc_public: 'SOC View Public Access',
}

const slaKeys = [
  { key: 'sla_network', label: 'Network' },
  { key: 'sla_security', label: 'Security' },
  { key: 'sla_physical_security', label: 'Physical Security' },
  { key: 'sla_key_services', label: 'Key Services' },
  { key: 'sla_other', label: 'Other' },
]

const boolSettings = new Set(['soc_public'])

// Keys shown on the General tab (filter alerting keys out)
const generalKeys = new Set(Object.keys(labels))

async function loadSettings() {
  try {
    const { data } = await api.get('/settings')
    settings.value = data
  } catch (e) {
    error.value = 'Failed to load settings'
  }
}

async function saveSettings() {
  error.value = ''
  loading.value = true
  try {
    const payload = {}
    for (const [k, v] of Object.entries(settings.value)) payload[k] = String(v)
    await api.put('/settings', payload)
    success.value = 'Settings saved'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to save settings'
  } finally {
    loading.value = false
  }
}

// ── Backup & Restore state ──
const restoreFile = ref(null)
const restoring = ref(false)

async function downloadBackup() {
  error.value = ''
  try {
    const resp = await api.get('/backup', { responseType: 'blob' })
    const disposition = resp.headers['content-disposition'] || ''
    const match = disposition.match(/filename="(.+)"/)
    const filename = match ? match[1] : 'bekci-backup.json'
    const url = URL.createObjectURL(resp.data)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to download backup'
  }
}

function onFileSelected(e) {
  restoreFile.value = e.target.files[0] || null
}

async function restoreBackup() {
  if (!restoreFile.value) return
  if (!confirm('This will WIPE all current data and replace it with the backup. All users will be logged out. Continue?')) return

  error.value = ''
  restoring.value = true
  try {
    const text = await restoreFile.value.text()
    const data = JSON.parse(text)
    await api.post('/backup/restore', data)
    auth.clearAuth()
    router.push('/login')
  } catch (e) {
    if (e instanceof SyntaxError) {
      error.value = 'Invalid JSON file'
    } else {
      error.value = e.response?.data?.error || 'Restore failed'
    }
  } finally {
    restoring.value = false
  }
}

// ── Audit Log tab state ──
const auditEntries = ref([])
const auditTotal = ref(0)
const auditPage = ref(1)
const auditLimit = 50
const auditLoading = ref(false)
const auditError = ref('')

const auditTotalPages = computed(() => Math.ceil(auditTotal.value / auditLimit) || 1)

async function loadAuditLog() {
  auditLoading.value = true
  auditError.value = ''
  try {
    const { data } = await api.get('/audit-log', { params: { page: auditPage.value, limit: auditLimit } })
    auditEntries.value = data.entries
    auditTotal.value = data.total
  } catch (e) {
    auditError.value = 'Failed to load audit log'
  } finally {
    auditLoading.value = false
  }
}

function auditPrevPage() {
  if (auditPage.value > 1) { auditPage.value--; loadAuditLog() }
}
function auditNextPage() {
  if (auditPage.value < auditTotalPages.value) { auditPage.value++; loadAuditLog() }
}

function fmtDate(d) {
  if (!d) return '-'
  const dt = new Date(d)
  return dt.toLocaleDateString('en-GB') + ' ' + dt.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function actionClass(action) {
  if (action.includes('login')) return 'badge-action-auth'
  if (action.includes('create')) return 'badge-action-create'
  if (action.includes('delete')) return 'badge-action-delete'
  if (action.includes('suspend')) return 'badge-action-delete'
  if (action.includes('failed')) return 'badge-action-delete'
  return 'badge-action-default'
}

// ── Users tab state ──
const users = ref([])
const userShowCreate = ref(false)
const userShowResetPw = ref(null)
const userError = ref('')
const userSuccess = ref('')
const userForm = ref({ username: '', email: '', password: '', role: 'operator' })
const resetPwForm = ref({ password: '' })

async function loadUsers() {
  try {
    const { data } = await api.get('/users')
    users.value = data
  } catch (e) {
    userError.value = 'Failed to load users'
  }
}

async function createUser() {
  userError.value = ''
  try {
    await api.post('/users', userForm.value)
    userSuccess.value = 'User created'
    userShowCreate.value = false
    userForm.value = { username: '', email: '', password: '', role: 'operator' }
    await loadUsers()
  } catch (e) {
    userError.value = e.response?.data?.error || 'Failed to create user'
  }
}

async function toggleSuspend(user) {
  userError.value = ''
  try {
    await api.put(`/users/${user.id}/suspend`, { suspended: user.status === 'active' })
    userSuccess.value = `User ${user.status === 'active' ? 'suspended' : 'activated'}`
    await loadUsers()
  } catch (e) {
    userError.value = e.response?.data?.error || 'Failed to update user'
  }
}

async function resetPassword(userId) {
  userError.value = ''
  try {
    await api.put(`/users/${userId}/password`, { password: resetPwForm.value.password })
    userSuccess.value = 'Password reset'
    userShowResetPw.value = null
    resetPwForm.value = { password: '' }
  } catch (e) {
    userError.value = e.response?.data?.error || 'Failed to reset password'
  }
}

// ── Alerting tab state ──
const alertError = ref('')
const alertSuccess = ref('')
const alertSaving = ref(false)
const alertTesting = ref(false)

const alertForm = ref({
  alert_method: 'email',
  resend_api_key: '',
  alert_from_email: '',
  alert_cooldown_s: '1800',
  alert_realert_s: '3600',
})

function loadAlertSettings() {
  // Pull alerting keys from the shared settings ref
  const s = settings.value
  alertForm.value = {
    alert_method: s.alert_method || 'email',
    resend_api_key: s.resend_api_key || '',
    alert_from_email: s.alert_from_email || '',
    alert_cooldown_s: s.alert_cooldown_s || '1800',
    alert_realert_s: s.alert_realert_s || '3600',
  }
}

async function saveAlertSettings() {
  alertError.value = ''
  alertSuccess.value = ''
  alertSaving.value = true
  try {
    await api.put('/settings', {
      alert_method: alertForm.value.alert_method,
      resend_api_key: alertForm.value.resend_api_key,
      alert_from_email: alertForm.value.alert_from_email,
      alert_cooldown_s: String(alertForm.value.alert_cooldown_s),
      alert_realert_s: String(alertForm.value.alert_realert_s),
    })
    alertSuccess.value = 'Alert settings saved'
    // Reload to get masked API key
    await loadSettings()
    loadAlertSettings()
  } catch (e) {
    alertError.value = e.response?.data?.error || 'Failed to save alert settings'
  } finally {
    alertSaving.value = false
  }
}

async function sendTestEmail() {
  alertError.value = ''
  alertSuccess.value = ''
  alertTesting.value = true
  try {
    const { data } = await api.post('/settings/test-email')
    alertSuccess.value = data.message || 'Test email sent'
  } catch (e) {
    alertError.value = e.response?.data?.error || 'Failed to send test email'
  } finally {
    alertTesting.value = false
  }
}

// ── Fail2Ban tab state ──
const f2bJails = ref([])
const f2bError = ref('')
const f2bLoading = ref(false)
const f2bFetchedAt = ref(null)
const f2bExpandedJails = ref(new Set())
let f2bTimer = null

async function loadFail2Ban() {
  f2bLoading.value = true
  f2bError.value = ''
  try {
    const { data } = await api.get('/fail2ban/status')
    f2bJails.value = data.jails || []
    f2bFetchedAt.value = data.fetched_at
  } catch (e) {
    const status = e.response?.status
    if (status === 503) {
      f2bError.value = 'fail2ban is not installed or not running on this server.'
    } else if (status === 504) {
      f2bError.value = 'fail2ban-client timed out.'
    } else {
      f2bError.value = e.response?.data?.error || 'Failed to fetch fail2ban status'
    }
    f2bJails.value = []
  } finally {
    f2bLoading.value = false
  }
}

function toggleJailIPs(name) {
  const s = new Set(f2bExpandedJails.value)
  if (s.has(name)) {
    s.delete(name)
  } else {
    s.add(name)
  }
  f2bExpandedJails.value = s
}

function startF2BPolling() {
  stopF2BPolling()
  loadFail2Ban()
  f2bTimer = setInterval(loadFail2Ban, 30000)
}

function stopF2BPolling() {
  if (f2bTimer) {
    clearInterval(f2bTimer)
    f2bTimer = null
  }
}

// Start/stop polling and load data when tab changes
watch(activeTab, (tab) => {
  if (tab === 'fail2ban') {
    startF2BPolling()
  } else {
    stopF2BPolling()
  }
  if (tab === 'audit') {
    auditPage.value = 1
    loadAuditLog()
  }
  if (tab === 'users') {
    loadUsers()
  }
  if (tab === 'alerting') {
    loadAlertSettings()
  }
})

onMounted(() => {
  loadSettings()
})

onUnmounted(() => {
  stopF2BPolling()
})
</script>

<template>
  <div class="page">
    <h2>Settings</h2>

    <div class="tabs">
      <button
        class="tab-btn"
        :class="{ active: activeTab === 'general' }"
        @click="activeTab = 'general'"
      >General</button>
      <button
        class="tab-btn"
        :class="{ active: activeTab === 'sla' }"
        @click="activeTab = 'sla'"
      >SLA</button>
      <button
        v-if="auth.isAdmin"
        class="tab-btn"
        :class="{ active: activeTab === 'alerting' }"
        @click="activeTab = 'alerting'"
      >Alerting</button>
      <button
        v-if="auth.isAdmin"
        class="tab-btn"
        :class="{ active: activeTab === 'users' }"
        @click="activeTab = 'users'"
      >Users</button>
      <button
        v-if="auth.isAdmin"
        class="tab-btn"
        :class="{ active: activeTab === 'backup' }"
        @click="activeTab = 'backup'"
      >Backup &amp; Restore</button>
      <button
        v-if="auth.isOperator"
        class="tab-btn"
        :class="{ active: activeTab === 'audit' }"
        @click="activeTab = 'audit'"
      >Audit Log</button>
      <button
        v-if="auth.isAdmin"
        class="tab-btn"
        :class="{ active: activeTab === 'fail2ban' }"
        @click="activeTab = 'fail2ban'"
      >Fail2Ban</button>
    </div>

    <!-- ── General Tab ── -->
    <div v-if="activeTab === 'general'">
      <div v-if="error" class="error-msg">{{ error }}</div>
      <div v-if="success" class="success-msg" @click="success = ''">{{ success }}</div>

      <div class="card">
        <form @submit.prevent="saveSettings">
          <div v-for="(value, key) in settings" :key="key" v-show="generalKeys.has(key)" class="form-group">
            <label>{{ labels[key] || key }}</label>
            <select
              v-if="boolSettings.has(key)"
              v-model="settings[key]"
              :disabled="!auth.isAdmin"
            >
              <option value="true">Yes</option>
              <option value="false">No</option>
            </select>
            <input
              v-else
              v-model="settings[key]"
              type="number"
              min="1"
              :disabled="!auth.isAdmin"
            />
          </div>
          <button v-if="auth.isAdmin" type="submit" class="btn btn-primary" :disabled="loading">
            {{ loading ? 'Saving...' : 'Save' }}
          </button>
          <p v-else class="text-muted">Only admins can modify settings.</p>
        </form>
      </div>

    </div>

    <!-- ── SLA Tab ── -->
    <div v-if="activeTab === 'sla'">
      <div v-if="error" class="error-msg">{{ error }}</div>
      <div v-if="success" class="success-msg" @click="success = ''">{{ success }}</div>

      <div class="card sla-tab-card">
        <div class="sla-intro">
          <h3>SLA Compliance Thresholds</h3>
          <p class="text-muted">Define minimum uptime targets per category. Each target's preferred check 90-day uptime is compared against its category threshold. Targets below the threshold display an <span class="sla-badge-example sla-badge-unhealthy-ex">UNHEALTHY</span> badge on the Dashboard and SOC views.</p>
        </div>

        <form @submit.prevent="saveSettings">
          <div class="sla-cards-grid">
            <div v-for="s in slaKeys" :key="s.key" class="sla-item">
              <div class="sla-item-header">
                <span class="sla-item-label">{{ s.label }}</span>
                <span v-if="settings[s.key] == 0" class="sla-item-status sla-disabled">Disabled</span>
                <span v-else class="sla-item-status sla-active">Active</span>
              </div>
              <div class="sla-input-row">
                <input
                  v-model="settings[s.key]"
                  type="number"
                  min="0"
                  max="100"
                  step="0.1"
                  :disabled="!auth.isAdmin"
                  class="sla-input"
                />
                <span class="sla-unit">%</span>
              </div>
            </div>
          </div>

          <div class="sla-footer">
            <p class="text-muted sla-hint">Set to <strong>0</strong> to disable SLA tracking for a category.</p>
            <button v-if="auth.isAdmin" type="submit" class="btn btn-primary" :disabled="loading">
              {{ loading ? 'Saving...' : 'Save' }}
            </button>
            <p v-else class="text-muted">Only admins can modify SLA settings.</p>
          </div>
        </form>
      </div>
    </div>

    <!-- ── Audit Log Tab ── -->
    <div v-if="activeTab === 'audit'">
      <div class="audit-header">
        <span class="text-muted">{{ auditTotal }} entries</span>
      </div>

      <div v-if="auditError" class="error-msg">{{ auditError }}</div>

      <div class="card">
        <table>
          <thead>
            <tr>
              <th>Timestamp</th>
              <th>User</th>
              <th>Action</th>
              <th>Resource</th>
              <th>Detail</th>
              <th>Status</th>
              <th>IP</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="auditLoading">
              <td colspan="7" style="text-align:center; color:#94a3b8;">Loading...</td>
            </tr>
            <tr v-else-if="auditEntries.length === 0">
              <td colspan="7" style="text-align:center; color:#94a3b8;">No audit entries</td>
            </tr>
            <tr v-for="e in auditEntries" :key="e.id">
              <td class="nowrap">{{ fmtDate(e.created_at) }}</td>
              <td>{{ e.username }}</td>
              <td><span class="badge" :class="actionClass(e.action)">{{ e.action }}</span></td>
              <td>{{ e.resource_type }}<span v-if="e.resource_id" class="text-muted"> #{{ e.resource_id.slice(0, 8) }}</span></td>
              <td class="detail-cell">{{ e.detail || '-' }}</td>
              <td><span :class="e.status === 'success' ? 'status-ok' : 'status-fail'">{{ e.status }}</span></td>
              <td class="text-muted">{{ e.ip_address }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="pagination" v-if="auditTotalPages > 1">
        <button class="btn btn-sm" :disabled="auditPage <= 1" @click="auditPrevPage">Prev</button>
        <span>Page {{ auditPage }} of {{ auditTotalPages }}</span>
        <button class="btn btn-sm" :disabled="auditPage >= auditTotalPages" @click="auditNextPage">Next</button>
      </div>
    </div>

    <!-- ── Users Tab ── -->
    <div v-if="activeTab === 'users'">
      <div class="users-header">
        <button class="btn btn-primary" @click="userShowCreate = !userShowCreate">
          {{ userShowCreate ? 'Cancel' : 'Create User' }}
        </button>
      </div>

      <div v-if="userError" class="error-msg">{{ userError }}</div>
      <div v-if="userSuccess" class="success-msg" @click="userSuccess = ''">{{ userSuccess }}</div>

      <div v-if="userShowCreate" class="card">
        <h3>Create User</h3>
        <form @submit.prevent="createUser">
          <div class="form-row">
            <div class="form-group">
              <label>Username</label>
              <input v-model="userForm.username" required />
            </div>
            <div class="form-group">
              <label>Email</label>
              <input v-model="userForm.email" type="email" />
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>Password (min 8 chars)</label>
              <input v-model="userForm.password" type="password" required minlength="8" />
            </div>
            <div class="form-group">
              <label>Role</label>
              <select v-model="userForm.role">
                <option value="admin">Admin</option>
                <option value="operator">Operator</option>
                <option value="viewer">Viewer</option>
              </select>
            </div>
          </div>
          <button type="submit" class="btn btn-primary">Create</button>
        </form>
      </div>

      <div class="card">
        <table>
          <thead>
            <tr>
              <th>Username</th>
              <th>Email</th>
              <th>Phone</th>
              <th>Role</th>
              <th>Status</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="u in users" :key="u.id">
              <td>{{ u.username }}</td>
              <td>{{ u.email || '-' }}</td>
              <td>{{ u.phone || '-' }}</td>
              <td><span :class="'badge badge-' + u.role">{{ u.role }}</span></td>
              <td><span :class="'badge badge-' + u.status">{{ u.status }}</span></td>
              <td class="actions">
                <button class="btn btn-sm" @click="toggleSuspend(u)">
                  {{ u.status === 'active' ? 'Suspend' : 'Activate' }}
                </button>
                <button class="btn btn-sm" @click="userShowResetPw = (userShowResetPw === u.id ? null : u.id)">
                  Reset PW
                </button>
                <div v-if="userShowResetPw === u.id" class="inline-form">
                  <input v-model="resetPwForm.password" type="password" placeholder="New password" minlength="8" />
                  <button class="btn btn-sm btn-primary" @click="resetPassword(u.id)">Set</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="card privileges-card">
        <h3>Role Privileges</h3>
        <table class="privileges-table">
          <thead>
            <tr>
              <th>Action</th>
              <th>Viewer</th>
              <th>Operator</th>
              <th>Admin</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>View dashboard &amp; SOC</td>
              <td class="perm-yes">Yes</td>
              <td class="perm-yes">Yes</td>
              <td class="perm-yes">Yes</td>
            </tr>
            <tr>
              <td>View targets, checks &amp; results</td>
              <td class="perm-yes">Yes</td>
              <td class="perm-yes">Yes</td>
              <td class="perm-yes">Yes</td>
            </tr>
            <tr>
              <td>View settings</td>
              <td class="perm-yes">Yes</td>
              <td class="perm-yes">Yes</td>
              <td class="perm-yes">Yes</td>
            </tr>
            <tr>
              <td>Edit own profile &amp; password</td>
              <td class="perm-yes">Yes</td>
              <td class="perm-yes">Yes</td>
              <td class="perm-yes">Yes</td>
            </tr>
            <tr>
              <td>Create, edit &amp; delete targets</td>
              <td class="perm-no">No</td>
              <td class="perm-yes">Yes</td>
              <td class="perm-yes">Yes</td>
            </tr>
            <tr>
              <td>Run checks manually</td>
              <td class="perm-no">No</td>
              <td class="perm-yes">Yes</td>
              <td class="perm-yes">Yes</td>
            </tr>
            <tr>
              <td>View audit log</td>
              <td class="perm-no">No</td>
              <td class="perm-yes">Yes</td>
              <td class="perm-yes">Yes</td>
            </tr>
            <tr>
              <td>Modify settings</td>
              <td class="perm-no">No</td>
              <td class="perm-no">No</td>
              <td class="perm-yes">Yes</td>
            </tr>
            <tr>
              <td>Manage users</td>
              <td class="perm-no">No</td>
              <td class="perm-no">No</td>
              <td class="perm-yes">Yes</td>
            </tr>
            <tr>
              <td>Backup &amp; restore</td>
              <td class="perm-no">No</td>
              <td class="perm-no">No</td>
              <td class="perm-yes">Yes</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- ── Backup & Restore Tab ── -->
    <div v-if="activeTab === 'backup'">
      <div v-if="error" class="error-msg">{{ error }}</div>

      <div class="card">
        <h3>Backup & Restore</h3>
        <p class="text-muted">Export or import configuration data (users, targets, checks, rules, settings). Historical check results are not included.</p>

        <div class="backup-actions">
          <button class="btn btn-primary" @click="downloadBackup">Download Backup</button>
        </div>

        <hr class="divider" />

        <div class="restore-section">
          <label class="file-label">
            <input type="file" accept=".json" @change="onFileSelected" />
            <span>{{ restoreFile ? restoreFile.name : 'Choose backup file...' }}</span>
          </label>

          <div v-if="restoreFile" class="restore-warning">
            <strong>Warning:</strong> Restoring will delete ALL current data and replace it with the backup contents. All sessions will be invalidated.
          </div>

          <button
            v-if="restoreFile"
            class="btn btn-danger"
            :disabled="restoring"
            @click="restoreBackup"
          >
            {{ restoring ? 'Restoring...' : 'Restore Now' }}
          </button>
        </div>
      </div>
    </div>

    <!-- ── Alerting Tab ── -->
    <div v-if="activeTab === 'alerting'">
      <div v-if="alertError" class="error-msg">{{ alertError }}</div>
      <div v-if="alertSuccess" class="success-msg" @click="alertSuccess = ''">{{ alertSuccess }}</div>

      <div class="card">
        <h3>Email Alerting</h3>
        <p class="text-muted">Configure alerts sent when targets go down or recover. Uses the Resend API.</p>

        <form @submit.prevent="saveAlertSettings">
          <div class="form-group">
            <label>Alert Method</label>
            <select v-model="alertForm.alert_method">
              <option value="email">Email</option>
              <option value="signal">Signal (coming soon)</option>
              <option value="email+signal">Email + Signal</option>
            </select>
          </div>

          <div class="form-group">
            <label>Resend API Key</label>
            <input v-model="alertForm.resend_api_key" type="password" placeholder="re_..." autocomplete="off" />
          </div>

          <div class="form-group">
            <label>From Email Address</label>
            <input v-model="alertForm.alert_from_email" type="email" placeholder="alerts@yourdomain.com" />
          </div>

          <div class="form-row">
            <div class="form-group">
              <label>Cooldown (minutes)</label>
              <input type="number" :value="Math.round(alertForm.alert_cooldown_s / 60)" @input="alertForm.alert_cooldown_s = $event.target.value * 60" min="0" />
              <span class="text-muted input-hint">Min wait between alerts for same target</span>
            </div>
            <div class="form-group">
              <label>Re-alert Interval (minutes)</label>
              <input type="number" :value="Math.round(alertForm.alert_realert_s / 60)" @input="alertForm.alert_realert_s = $event.target.value * 60" min="0" />
              <span class="text-muted input-hint">Repeat alert if still down (0 = disabled)</span>
            </div>
          </div>

          <div class="form-actions">
            <button type="submit" class="btn btn-primary" :disabled="alertSaving">
              {{ alertSaving ? 'Saving...' : 'Save' }}
            </button>
            <button type="button" class="btn" :disabled="alertTesting" @click="sendTestEmail">
              {{ alertTesting ? 'Sending...' : 'Send Test Email' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- ── Fail2Ban Tab ── -->
    <div v-if="activeTab === 'fail2ban'">
      <div class="card">
        <div class="f2b-header">
          <h3>Fail2Ban Jail Status</h3>
          <div class="f2b-actions">
            <span v-if="f2bFetchedAt" class="f2b-updated">Updated: {{ fmtDate(f2bFetchedAt) }}</span>
            <button class="btn btn-sm" :disabled="f2bLoading" @click="loadFail2Ban">
              {{ f2bLoading ? 'Loading...' : 'Refresh' }}
            </button>
          </div>
        </div>

        <div v-if="f2bError" class="error-msg">{{ f2bError }}</div>

        <table v-if="f2bJails.length > 0">
          <thead>
            <tr>
              <th>Jail</th>
              <th>Active Bans</th>
              <th>Bans (total)</th>
              <th>Failed (window)</th>
              <th>Failed (total)</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            <template v-for="jail in f2bJails" :key="jail.name">
              <tr>
                <td><strong>{{ jail.name }}</strong></td>
                <td>
                  <span class="badge" :class="jail.currently_banned > 0 ? 'badge-banned' : 'badge-clear'">
                    {{ jail.currently_banned }}
                  </span>
                </td>
                <td>{{ jail.total_banned }}</td>
                <td>
                  <span :class="{ 'f2b-warn': jail.currently_failed > 0 }">
                    {{ jail.currently_failed }}
                  </span>
                </td>
                <td>{{ jail.total_failed }}</td>
                <td>
                  <button
                    v-if="jail.banned_ips && jail.banned_ips.length > 0"
                    class="btn btn-sm"
                    @click="toggleJailIPs(jail.name)"
                  >
                    {{ f2bExpandedJails.has(jail.name) ? 'Hide IPs' : 'Show IPs' }}
                  </button>
                  <span v-else class="text-muted">No bans</span>
                </td>
              </tr>
              <tr v-if="f2bExpandedJails.has(jail.name) && jail.banned_ips && jail.banned_ips.length > 0">
                <td colspan="6" class="f2b-ips-cell">
                  <div class="f2b-ips">
                    <span v-for="ip in jail.banned_ips" :key="ip" class="f2b-ip">{{ ip }}</span>
                  </div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>

        <p v-if="!f2bError && f2bJails.length === 0 && !f2bLoading" class="text-muted">
          No jails found.
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* ── Tabs ── */
.tabs {
  display: flex;
  gap: 0;
  border-bottom: 2px solid #e2e8f0;
  margin-bottom: 1rem;
}
.tab-btn {
  padding: 0.5rem 1.25rem;
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  margin-bottom: -2px;
  font-size: 0.9rem;
  font-weight: 500;
  color: #64748b;
  cursor: pointer;
  transition: color 0.15s, border-color 0.15s;
}
.tab-btn:hover {
  color: #ea580c;
}
.tab-btn.active {
  color: #ea580c;
  border-bottom-color: #ea580c;
}

/* ── General tab ── */
.backup-card h3 {
  margin: 0 0 0.5rem;
}
.backup-actions {
  margin: 1rem 0;
}
.divider {
  border: none;
  border-top: 1px solid #e2e8f0;
  margin: 1rem 0;
}
.restore-section {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.file-label {
  display: inline-block;
  cursor: pointer;
}
.file-label input[type="file"] {
  display: none;
}
.file-label span {
  display: inline-block;
  padding: 0.5rem 1rem;
  background: #f1f5f9;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  font-size: 0.875rem;
  color: #475569;
}
.file-label span:hover {
  background: #e2e8f0;
}
.restore-warning {
  background: #fef3c7;
  color: #92400e;
  padding: 0.5rem 0.75rem;
  border-radius: 6px;
  font-size: 0.875rem;
}
.btn-danger {
  background: #dc2626;
  color: #fff;
  border-color: #dc2626;
}
.btn-danger:hover {
  background: #b91c1c;
}

/* ── SLA tab ── */
.sla-tab-card {
  max-width: 720px;
}
.sla-intro {
  margin-bottom: 1.25rem;
}
.sla-intro h3 {
  margin: 0 0 0.35rem;
  font-size: 1rem;
}
.sla-intro p {
  font-size: 0.85rem;
  line-height: 1.5;
}
.sla-badge-example {
  display: inline-block;
  font-size: 0.65rem;
  font-weight: 700;
  padding: 0.05rem 0.4rem;
  border-radius: 10px;
  vertical-align: middle;
}
.sla-badge-unhealthy-ex {
  background: #fed7aa;
  color: #9a3412;
}
.sla-cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 0.75rem;
  margin-bottom: 1.25rem;
}
.sla-item {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 0.75rem;
}
.sla-item-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}
.sla-item-label {
  font-weight: 600;
  font-size: 0.85rem;
  color: #1e293b;
}
.sla-item-status {
  font-size: 0.65rem;
  font-weight: 600;
  padding: 0.1rem 0.4rem;
  border-radius: 10px;
  text-transform: uppercase;
}
.sla-active {
  background: #dcfce7;
  color: #166534;
}
.sla-disabled {
  background: #f1f5f9;
  color: #94a3b8;
}
.sla-input-row {
  display: flex;
  align-items: center;
  gap: 0.35rem;
}
.sla-input {
  flex: 1;
  font-size: 1.1rem;
  font-weight: 600;
  padding: 0.4rem 0.5rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: #fff;
  text-align: right;
  width: 100%;
}
.sla-input:focus {
  outline: none;
  border-color: #ea580c;
  box-shadow: 0 0 0 2px rgba(234, 88, 12, 0.15);
}
.sla-unit {
  font-size: 0.9rem;
  font-weight: 600;
  color: #64748b;
}
.sla-footer {
  display: flex;
  align-items: center;
  gap: 1rem;
}
.sla-hint {
  font-size: 0.8rem;
  margin: 0;
}

/* ── Audit Log tab ── */
.audit-header {
  margin-bottom: 0.75rem;
}
.nowrap { white-space: nowrap; }
.detail-cell { max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.status-ok { color: #16a34a; font-weight: 600; font-size: 0.8rem; }
.status-fail { color: #dc2626; font-weight: 600; font-size: 0.8rem; }
.pagination { display: flex; align-items: center; justify-content: center; gap: 1rem; margin-top: 1rem; }
.badge-action-auth { background: #dbeafe; color: #1d4ed8; }
.badge-action-create { background: #dcfce7; color: #166534; }
.badge-action-delete { background: #fee2e2; color: #991b1b; }
.badge-action-default { background: #f1f5f9; color: #475569; }

/* ── Users tab ── */
.users-header {
  margin-bottom: 0.75rem;
}
.privileges-card {
  margin-top: 1.5rem;
  opacity: 0.85;
}
.privileges-card h3 {
  margin-bottom: 0.75rem;
  font-size: 0.95rem;
}
.privileges-table {
  font-size: 0.85rem;
}
.privileges-table th:not(:first-child),
.privileges-table td:not(:first-child) {
  text-align: center;
  width: 100px;
}
.perm-yes {
  color: #16a34a;
  font-weight: 600;
}
.perm-no {
  color: #9ca3af;
}

/* ── Alerting tab ── */
.input-hint {
  display: block;
  font-size: 0.75rem;
  margin-top: 0.25rem;
}
.form-actions {
  display: flex;
  gap: 0.5rem;
  margin-top: 1rem;
}

/* ── Fail2Ban tab ── */
.f2b-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}
.f2b-header h3 {
  margin: 0;
}
.f2b-actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}
.f2b-updated {
  font-size: 0.8rem;
  color: #94a3b8;
}
.badge-banned {
  background: #fee2e2;
  color: #991b1b;
}
.badge-clear {
  background: #dcfce7;
  color: #166534;
}
.f2b-warn {
  color: #d97706;
  font-weight: 600;
}
.f2b-ips-cell {
  background: #fef2f2;
  padding: 0.75rem !important;
}
.f2b-ips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}
.f2b-ip {
  font-family: 'SF Mono', 'Consolas', 'Monaco', monospace;
  font-size: 0.8rem;
  background: #fee2e2;
  color: #991b1b;
  padding: 0.125rem 0.5rem;
  border-radius: 4px;
}
</style>
