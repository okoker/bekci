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
  soc_public: 'SOC View Public Access',
}

const boolSettings = new Set(['soc_public'])

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
        v-if="auth.isOperator"
        class="tab-btn"
        :class="{ active: activeTab === 'audit' }"
        @click="activeTab = 'audit'"
      >Audit Log</button>
      <button
        v-if="auth.isAdmin"
        class="tab-btn"
        :class="{ active: activeTab === 'backup' }"
        @click="activeTab = 'backup'"
      >Backup &amp; Restore</button>
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
          <div v-for="(value, key) in settings" :key="key" class="form-group">
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
