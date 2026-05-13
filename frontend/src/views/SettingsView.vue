<script setup>
import { ref, nextTick, onMounted, onUnmounted, watch, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import api from '../api'

const props = defineProps({
  initialTab: { type: String, default: '' }
})

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const activeTab = ref(props.initialTab || 'general')

// ── General tab state ──
const settings = ref({})
const error = ref('')
const success = ref('')
const loading = ref(false)

const labels = {
  session_timeout_hours: 'Session Timeout (hours)',
  history_days: 'History Retention (days)',
  audit_retention_days: 'Audit Log Retention (days)',
  backup_max_copies: 'Backup Copies to Keep',
  soc_public: 'SOC View Public Access',
  api_rate_limit_per_min: 'API Rate Limit (req/min per token)',
}

const slaKeys = ref([])

function deriveSLAKey(name) {
  return 'sla_' + name.toLowerCase().replace(/ /g, '_')
}

async function loadSLAKeys() {
  try {
    const { data } = await api.get('/tags?group=category')
    data.sort((a, b) => {
      if (a.value === 'Other') return 1
      if (b.value === 'Other') return -1
      return a.value.localeCompare(b.value)
    })
    slaKeys.value = data.map(c => ({ key: deriveSLAKey(c.value), label: c.value }))
  } catch { /* ignore */ }
}

const boolSettings = new Set(['soc_public'])
const showSlaInfo = ref(false)

// Keys shown on the General tab (filter alerting keys out)
const generalKeys = new Set(Object.keys(labels))

async function loadSettings() {
  try {
    const { data } = await api.get('/settings')
    // Default SLA keys to "0" if missing so inputs aren't blank
    for (const s of slaKeys.value) {
      if (!(s.key in data)) data[s.key] = '99.5'
    }
    // Default backup_max_copies if not yet saved
    if (!('backup_max_copies' in data)) data['backup_max_copies'] = '5'
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
    const knownKeys = new Set([...generalKeys, ...slaKeys.value.map(s => s.key)])
    for (const [k, v] of Object.entries(settings.value)) {
      if (knownKeys.has(k)) payload[k] = String(v)
    }
    await api.put('/settings', payload)
    success.value = 'Settings saved'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to save settings'
  } finally {
    loading.value = false
  }
}

// ── Auto Backup state ──
const autoBackupForm = ref({ schedule: 'off', time: '03:00' })
const autoBackupSaving = ref(false)
const autoBackupSuccess = ref('')
const autoBackupError = ref('')

function loadAutoBackupSettings() {
  const s = settings.value
  autoBackupForm.value = {
    schedule: s.auto_backup_schedule || 'off',
    time: s.auto_backup_time || '03:00',
  }
}

async function saveAutoBackupSettings() {
  autoBackupError.value = ''
  autoBackupSuccess.value = ''
  autoBackupSaving.value = true
  try {
    await api.put('/settings', {
      auto_backup_schedule: autoBackupForm.value.schedule,
      auto_backup_time: autoBackupForm.value.time,
    })
    autoBackupSuccess.value = 'Auto backup settings saved'
    await loadSettings()
    loadAutoBackupSettings()
  } catch (e) {
    autoBackupError.value = e.response?.data?.error || 'Failed to save'
  } finally {
    autoBackupSaving.value = false
  }
}

// ── Backup & Restore state ──
const restoreFile = ref(null)
const restoring = ref(false)
const showRestoreConfirm = ref(false)

// ── Full Backup state ──
const fullBackupExpanded = ref(false)
const fullBackupEncrypt = ref(false)
const fullBackupPassphrase = ref('')
const fullBackupLoading = ref(false)
const fullBackupError = ref('')
const fullBackupDest = ref('download')
const fullBackupSuccess = ref('')
const savedBackups = ref([])
const savedBackupsLoading = ref(false)

async function fetchPassphrase() {
  try {
    const { data } = await api.get('/backup/generate-passphrase')
    fullBackupPassphrase.value = data.passphrase
  } catch (e) {
    fullBackupError.value = 'Failed to generate passphrase'
  }
}

async function downloadFullBackup() {
  fullBackupError.value = ''
  fullBackupLoading.value = true
  try {
    const body = {}
    if (fullBackupEncrypt.value) {
      body.encrypt = true
      body.passphrase = fullBackupPassphrase.value
    }
    const resp = await api.post('/backup/full', body, { responseType: 'blob', timeout: 300000 })
    const disposition = resp.headers['content-disposition'] || ''
    const match = disposition.match(/filename="(.+)"/)
    const filename = match ? match[1] : 'bekci-full-backup'
    const url = URL.createObjectURL(resp.data)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  } catch (e) {
    fullBackupError.value = e.response?.data?.error || 'Full backup failed'
  } finally {
    fullBackupLoading.value = false
  }
}

function copyPassphrase() {
  navigator.clipboard.writeText(fullBackupPassphrase.value)
}

async function fetchSavedBackups() {
  savedBackupsLoading.value = true
  try {
    const { data } = await api.get('/backup/full/list')
    savedBackups.value = data || []
  } catch { savedBackups.value = [] }
  finally { savedBackupsLoading.value = false }
}

async function saveFullBackup() {
  fullBackupError.value = ''
  fullBackupSuccess.value = ''
  fullBackupLoading.value = true
  try {
    const body = {}
    if (fullBackupEncrypt.value) {
      body.encrypt = true
      body.passphrase = fullBackupPassphrase.value
    }
    const { data } = await api.post('/backup/full/save', body, { timeout: 300000 })
    fullBackupSuccess.value = `Saved: ${data.filename}`
    try {
      await fetchSavedBackups()
    } catch {
      fullBackupError.value = 'Backup saved but failed to refresh backup list'
    }
  } catch (e) {
    fullBackupError.value = e.response?.data?.error || 'Save failed'
  } finally {
    fullBackupLoading.value = false
  }
}

async function downloadSavedBackup(filename) {
  try {
    const resp = await api.get(`/backup/full/saved/${filename}`, { responseType: 'blob', timeout: 300000 })
    const url = URL.createObjectURL(resp.data)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  } catch (e) {
    fullBackupError.value = e.response?.data?.error || 'Download failed'
  }
}

async function deleteSavedBackup(filename) {
  if (!confirm(`Delete ${filename}?`)) return
  try {
    await api.delete(`/backup/full/saved/${filename}`)
    fetchSavedBackups()
  } catch (e) {
    fullBackupError.value = e.response?.data?.error || 'Delete failed'
  }
}

function copyHash(hash) {
  navigator.clipboard.writeText(hash)
}

function formatBackupDate(isoStr) {
  const d = new Date(isoStr)
  return d.toLocaleDateString('en-GB') + ' ' + d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })
}

function formatSize(bytes) {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / 1048576).toFixed(1) + ' MB'
}

async function downloadBackup() {
  error.value = ''
  try {
    const resp = await api.get('/backup', { responseType: 'blob' })
    const disposition = resp.headers['content-disposition'] || ''
    const match = disposition.match(/filename="(.+)"/)
    const filename = match ? match[1] : 'bekci-config.json'
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

function confirmRestore() {
  if (!restoreFile.value) return
  showRestoreConfirm.value = true
}

async function executeRestore() {
  showRestoreConfirm.value = false
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
const auditSearch = ref('')
const auditSearchInput = ref(null)
let auditSearchTimer = null

const auditTotalPages = computed(() => Math.ceil(auditTotal.value / auditLimit) || 1)

async function loadAuditLog() {
  auditLoading.value = true
  auditError.value = ''
  try {
    const params = { page: auditPage.value, limit: auditLimit }
    if (auditSearch.value.trim()) params.q = auditSearch.value.trim()
    const { data } = await api.get('/audit-log', { params })
    auditEntries.value = data.entries
    auditTotal.value = data.total
  } catch (e) {
    auditError.value = 'Failed to load audit log'
  } finally {
    auditLoading.value = false
  }
}

function onAuditSearch() {
  clearTimeout(auditSearchTimer)
  auditSearchTimer = setTimeout(() => {
    auditPage.value = 1
    loadAuditLog()
  }, 300)
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
const userShowEdit = ref(null)
const userError = ref('')
const userSuccess = ref('')
const showPrivileges = ref(false)
const userForm = ref({ username: '', email: '', password: '', role: 'operator' })
const resetPwForm = ref({ password: '' })
const editUserForm = ref({ email: '', phone: '', role: '' })

// ── Users > sub-tabs (humans / api-access) ──
const usersSubTab = ref('humans')

// ── API tokens state ──
const apiTokens = ref([])
const apiTokenError = ref('')
const apiTokenSuccess = ref('')
const apiTokenShowCreate = ref(false)
const apiTokenNewName = ref('')
// Plaintext surfaces here ONCE right after creation — cleared on dismiss.
const apiTokenJustCreated = ref(null)

async function loadAPITokens() {
  apiTokenError.value = ''
  try {
    const { data } = await api.get('/api-tokens')
    apiTokens.value = data
  } catch (e) {
    apiTokenError.value = e.response?.data?.error || 'Failed to load API tokens'
  }
}

async function createAPIToken() {
  apiTokenError.value = ''
  apiTokenSuccess.value = ''
  const name = apiTokenNewName.value.trim()
  if (!name) return
  try {
    const { data } = await api.post('/api-tokens', { name })
    apiTokenJustCreated.value = { name, plaintext: data.plaintext, prefix: data.token?.prefix }
    apiTokenNewName.value = ''
    apiTokenShowCreate.value = false
    await loadAPITokens()
  } catch (e) {
    apiTokenError.value = e.response?.data?.error || 'Failed to create token'
  }
}

async function revokeAPIToken(id, name) {
  if (!confirm(`Revoke token "${name}"? This cannot be undone and any system using it will stop working immediately.`)) return
  try {
    await api.delete(`/api-tokens/${id}`)
    apiTokenSuccess.value = `Token "${name}" revoked`
    await loadAPITokens()
  } catch (e) {
    apiTokenError.value = e.response?.data?.error || 'Failed to revoke token'
  }
}

async function copyTokenToClipboard(text) {
  try {
    await navigator.clipboard.writeText(text)
    apiTokenSuccess.value = 'Token copied to clipboard'
  } catch {
    apiTokenSuccess.value = 'Copy failed — select and copy manually'
  }
}

function dismissJustCreatedToken() {
  apiTokenJustCreated.value = null
}

function fmtDateTime(s) {
  if (!s) return '—'
  const d = new Date(s)
  if (isNaN(d.getTime())) return s
  return d.toLocaleDateString('en-GB') + ' ' + d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })
}

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

const suspendingUser = ref(null) // user object for suspend confirmation modal

function confirmSuspend(user) {
  suspendingUser.value = user
}

async function toggleSuspend() {
  const user = suspendingUser.value
  if (!user) return
  userError.value = ''
  try {
    await api.put(`/users/${user.id}/suspend`, { suspended: user.status === 'active' })
    userSuccess.value = `User ${user.status === 'active' ? 'suspended' : 'activated'}`
    suspendingUser.value = null
    await loadUsers()
  } catch (e) {
    userError.value = e.response?.data?.error || 'Failed to update user'
    suspendingUser.value = null
  }
}

const editingUser = ref(null) // full user object for modal

function openEditUser(u) {
  editingUser.value = u
  editUserForm.value = { email: u.email || '', phone: u.phone || '', role: u.role }
  resetPwForm.value = { password: '' }
  userShowEdit.value = u.id
}

function closeEditModal() {
  editingUser.value = null
  userShowEdit.value = null
  resetPwForm.value = { password: '' }
}

async function saveEditUser(userId) {
  userError.value = ''
  try {
    await api.put(`/users/${userId}`, editUserForm.value)
    userSuccess.value = 'User updated'
    closeEditModal()
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

const signalTesting = ref(false)
const signalTestPhone = ref('')

const webhookTesting = ref(false)
const webhookTestResult = ref('')
const webhookTestError = ref(false)
const webhookLastError = ref('')
const webhookLastSuccess = ref('')

const emailTestRecipient = ref('')
const emailTestResult = ref('')
const emailTestError = ref(false)

const alertForm = ref({
  alert_method: 'email',
  email_provider: 'resend',
  resend_api_key: '',
  alert_from_email: '',
  smtp_host: 'smtp.office365.com',
  smtp_port: '587',
  smtp_username: '',
  smtp_password: '',
  alert_cooldown_s: '1800',
  alert_realert_s: '3600',
  signal_api_url: '',
  signal_number: '',
  signal_username: '',
  signal_password: '',
  signal_skip_tls: 'false',
  webhook_enabled: 'false',
  webhook_url: '',
  webhook_auth_type: '',
  webhook_bearer_token: '',
  webhook_basic_username: '',
  webhook_basic_password: '',
  webhook_skip_tls: 'false',
  webhook_timeout_s: '10',
  system_alert_admins: 'true',
  system_alert_users: '',
})

// Track which alerting sections are expanded (first open by default)
const alertSections = ref({ general: true, email: false, signal: false, webhook: false, system: false })
const tagSections = ref({ tag: true, project: false, location: false, category: false })

// SNMP credentials (separate from alerting)
const snmpForm = ref({
  snmp_v2c_community: 'public',
  snmp_v3_username: '',
  snmp_v3_security_level: 'authPriv',
  snmp_v3_auth_protocol: 'SHA',
  snmp_v3_auth_passphrase: '',
  snmp_v3_privacy_protocol: 'AES',
  snmp_v3_privacy_passphrase: '',
})
const snmpExpanded = ref(false)
const snmpSaving = ref(false)
const snmpError = ref('')
const snmpSuccess = ref('')

function loadAlertSettings() {
  // Pull alerting keys from the shared settings ref
  const s = settings.value
  alertForm.value = {
    alert_method: s.alert_method || 'email',
    email_provider: s.email_provider || 'resend',
    resend_api_key: s.resend_api_key || '',
    alert_from_email: s.alert_from_email || '',
    smtp_host: s.smtp_host || 'smtp.office365.com',
    smtp_port: s.smtp_port || '587',
    smtp_username: s.smtp_username || '',
    smtp_password: s.smtp_password || '',
    alert_cooldown_s: s.alert_cooldown_s || '1800',
    alert_realert_s: s.alert_realert_s || '3600',
    signal_api_url: s.signal_api_url || '',
    signal_number: s.signal_number || '',
    signal_username: s.signal_username || '',
    signal_password: s.signal_password || '',
    signal_skip_tls: s.signal_skip_tls || 'false',
    webhook_enabled: s.webhook_enabled || 'false',
    webhook_url: s.webhook_url || '',
    webhook_auth_type: s.webhook_auth_type || '',
    webhook_bearer_token: s.webhook_bearer_token || '',
    webhook_basic_username: s.webhook_basic_username || '',
    webhook_basic_password: s.webhook_basic_password || '',
    webhook_skip_tls: s.webhook_skip_tls || 'false',
    webhook_timeout_s: s.webhook_timeout_s || '10',
    system_alert_admins: s.system_alert_admins || 'true',
    system_alert_users: s.system_alert_users || '',
  }
  // Pre-populate test phone from logged-in user's profile
  if (!signalTestPhone.value) {
    signalTestPhone.value = auth.user?.phone || ''
  }
  if (!emailTestRecipient.value) {
    emailTestRecipient.value = auth.user?.email || ''
  }
  // Load webhook status
  api.get('/settings/webhook-status').then(res => {
    webhookLastError.value = res.data.last_error || ''
    webhookLastSuccess.value = res.data.last_success || ''
  }).catch(() => {})
}

async function saveAlertGeneral() {
  alertError.value = ''
  alertSuccess.value = ''
  alertSaving.value = true
  try {
    await api.put('/settings', {
      alert_method: alertForm.value.alert_method,
      alert_cooldown_s: String(alertForm.value.alert_cooldown_s),
      alert_realert_s: String(alertForm.value.alert_realert_s),
    })
    alertSuccess.value = 'Alert settings saved'
    await loadSettings()
    loadAlertSettings()
  } catch (e) {
    alertError.value = e.response?.data?.error || 'Failed to save alert settings'
  } finally {
    alertSaving.value = false
  }
}

async function saveEmailSettings() {
  alertError.value = ''
  alertSuccess.value = ''
  alertSaving.value = true
  try {
    await api.put('/settings', {
      email_provider: alertForm.value.email_provider,
      resend_api_key: alertForm.value.resend_api_key,
      alert_from_email: alertForm.value.alert_from_email,
      smtp_host: alertForm.value.smtp_host,
      smtp_port: alertForm.value.smtp_port,
      smtp_username: alertForm.value.smtp_username,
      smtp_password: alertForm.value.smtp_password,
    })
    alertSuccess.value = 'Email settings saved'
    await loadSettings()
    loadAlertSettings()
  } catch (e) {
    alertError.value = e.response?.data?.error || 'Failed to save email settings'
  } finally {
    alertSaving.value = false
  }
}

async function saveSignalSettings() {
  alertError.value = ''
  alertSuccess.value = ''
  alertSaving.value = true
  try {
    await api.put('/settings', {
      signal_api_url: alertForm.value.signal_api_url,
      signal_number: alertForm.value.signal_number,
      signal_username: alertForm.value.signal_username,
      signal_password: alertForm.value.signal_password,
      signal_skip_tls: alertForm.value.signal_skip_tls,
    })
    alertSuccess.value = 'Signal settings saved'
    await loadSettings()
    loadAlertSettings()
  } catch (e) {
    alertError.value = e.response?.data?.error || 'Failed to save signal settings'
  } finally {
    alertSaving.value = false
  }
}

async function saveWebhookSettings() {
  alertError.value = ''
  alertSuccess.value = ''
  alertSaving.value = true
  try {
    await api.put('/settings', {
      webhook_enabled: alertForm.value.webhook_enabled,
      webhook_url: alertForm.value.webhook_url,
      webhook_auth_type: alertForm.value.webhook_auth_type,
      webhook_bearer_token: alertForm.value.webhook_bearer_token,
      webhook_basic_username: alertForm.value.webhook_basic_username,
      webhook_basic_password: alertForm.value.webhook_basic_password,
      webhook_skip_tls: alertForm.value.webhook_skip_tls,
      webhook_timeout_s: String(alertForm.value.webhook_timeout_s),
    })
    alertSuccess.value = 'Webhook settings saved'
    await loadSettings()
    loadAlertSettings()
  } catch (e) {
    alertError.value = e.response?.data?.error || 'Failed to save webhook settings'
  } finally {
    alertSaving.value = false
  }
}

async function saveSystemAlertSettings() {
  alertError.value = ''
  alertSuccess.value = ''
  alertSaving.value = true
  try {
    await api.put('/settings', {
      system_alert_admins: alertForm.value.system_alert_admins,
      system_alert_users: alertForm.value.system_alert_users,
    })
    alertSuccess.value = 'System alert settings saved'
    await loadSettings()
    loadAlertSettings()
  } catch (e) {
    alertError.value = e.response?.data?.error || 'Failed to save system alert settings'
  } finally {
    alertSaving.value = false
  }
}

// Computed: selected user IDs for multi-select
const systemAlertUserIds = computed({
  get() {
    const val = alertForm.value.system_alert_users
    return val ? val.split(',').filter(Boolean) : []
  },
  set(ids) {
    alertForm.value.system_alert_users = ids.join(',')
  }
})

function toggleSystemAlertUser(userId) {
  const ids = new Set(systemAlertUserIds.value)
  if (ids.has(userId)) {
    ids.delete(userId)
  } else {
    ids.add(userId)
  }
  systemAlertUserIds.value = [...ids]
}

function loadSnmpSettings() {
  const s = settings.value
  snmpForm.value = {
    snmp_v2c_community: s.snmp_v2c_community || 'public',
    snmp_v3_username: s.snmp_v3_username || '',
    snmp_v3_security_level: s.snmp_v3_security_level || 'authPriv',
    snmp_v3_auth_protocol: s.snmp_v3_auth_protocol || 'SHA',
    snmp_v3_auth_passphrase: s.snmp_v3_auth_passphrase || '',
    snmp_v3_privacy_protocol: s.snmp_v3_privacy_protocol || 'AES',
    snmp_v3_privacy_passphrase: s.snmp_v3_privacy_passphrase || '',
  }
}

async function saveSnmpSettings() {
  snmpError.value = ''
  snmpSuccess.value = ''
  snmpSaving.value = true
  try {
    await api.put('/settings', {
      snmp_v2c_community: snmpForm.value.snmp_v2c_community,
      snmp_v3_username: snmpForm.value.snmp_v3_username,
      snmp_v3_security_level: snmpForm.value.snmp_v3_security_level,
      snmp_v3_auth_protocol: snmpForm.value.snmp_v3_auth_protocol,
      snmp_v3_auth_passphrase: snmpForm.value.snmp_v3_auth_passphrase,
      snmp_v3_privacy_protocol: snmpForm.value.snmp_v3_privacy_protocol,
      snmp_v3_privacy_passphrase: snmpForm.value.snmp_v3_privacy_passphrase,
    })
    snmpSuccess.value = 'SNMP settings saved'
    await loadSettings()
    loadSnmpSettings()
  } catch (e) {
    snmpError.value = e.response?.data?.error || 'Failed to save SNMP settings'
  } finally {
    snmpSaving.value = false
  }
}

async function sendTestEmail() {
  alertError.value = ''
  alertSuccess.value = ''
  emailTestResult.value = ''
  emailTestError.value = false
  alertTesting.value = true
  try {
    const { data } = await api.post('/settings/test-email', { email: emailTestRecipient.value })
    const msg = data.message || 'Test email sent'
    alertSuccess.value = msg
    emailTestResult.value = msg
    emailTestError.value = false
  } catch (e) {
    const msg = e.response?.data?.error || 'Failed to send test email'
    alertError.value = msg
    emailTestResult.value = msg
    emailTestError.value = true
  } finally {
    alertTesting.value = false
  }
}

async function sendTestSignal() {
  alertError.value = ''
  alertSuccess.value = ''
  if (!signalTestPhone.value) {
    alertError.value = 'Enter a phone number to send the test to'
    return
  }
  signalTesting.value = true
  try {
    const { data } = await api.post('/settings/test-signal', { phone: signalTestPhone.value })
    alertSuccess.value = data.message || 'Test signal sent'
  } catch (e) {
    alertError.value = e.response?.data?.error || 'Failed to send test signal'
  } finally {
    signalTesting.value = false
  }
}

async function sendTestWebhook() {
  webhookTesting.value = true
  webhookTestResult.value = ''
  webhookTestError.value = false
  alertError.value = ''
  alertSuccess.value = ''
  try {
    const res = await api.post('/settings/test-webhook')
    const msg = res.data.message || 'Test webhook sent successfully'
    webhookTestResult.value = msg
    webhookTestError.value = false
    alertSuccess.value = msg
    const statusRes = await api.get('/settings/webhook-status')
    webhookLastError.value = statusRes.data.last_error || ''
    webhookLastSuccess.value = statusRes.data.last_success || ''
  } catch (err) {
    const msg = err.response?.data?.error || 'Failed to send test webhook'
    webhookTestResult.value = msg
    webhookTestError.value = true
    alertError.value = msg
  } finally {
    webhookTesting.value = false
  }
}

// ── Tags tab state ──
const projectTags = ref([])
const locationTags = ref([])
const newProjectTag = ref('')
const newLocationTag = ref('')
const categoryTags = ref([])
const newCategoryTag = ref('')
const freeTags = ref([])
const newFreeTag = ref('')
const editingCategoryId = ref(null)
const editingCategoryValue = ref('')
const categoryDeleteError = ref(null)

async function loadTags() {
  try {
    const [p, l, c, t] = await Promise.all([
      api.get('/tags?group=project'),
      api.get('/tags?group=location'),
      api.get('/tags?group=category'),
      api.get('/tags?group=tag')
    ])
    projectTags.value = p.data
    locationTags.value = l.data
    categoryTags.value = c.data
    freeTags.value = t.data
  } catch (e) {
    error.value = 'Failed to load tags'
  }
}

async function addTag(group) {
  const value = group === 'project' ? newProjectTag.value.trim()
    : group === 'location' ? newLocationTag.value.trim()
    : group === 'tag' ? newFreeTag.value.trim()
    : newCategoryTag.value.trim()
  if (!value) return
  try {
    await api.post('/tags', { group, value })
    if (group === 'project') newProjectTag.value = ''
    else if (group === 'location') newLocationTag.value = ''
    else if (group === 'tag') newFreeTag.value = ''
    else newCategoryTag.value = ''
    await loadTags()
    success.value = 'Tag added'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to add tag'
  }
}

async function deleteTag(id) {
  categoryDeleteError.value = null
  if (!confirm('Are you sure you want to delete this category?')) return
  try {
    await api.delete(`/tags/${id}`)
    await loadTags()
    success.value = 'Tag deleted'
  } catch (e) {
    if (e.response?.status === 409 && e.response?.data?.targets) {
      categoryDeleteError.value = {
        id,
        targets: e.response.data.targets
      }
      return
    }
    error.value = e.response?.data?.error || 'Failed to delete tag'
  }
}

function startRenameCategory(tag) {
  editingCategoryId.value = tag.id
  editingCategoryValue.value = tag.value
}

function cancelRenameCategory() {
  editingCategoryId.value = null
  editingCategoryValue.value = ''
}

async function saveRenameCategory(tag) {
  const newValue = editingCategoryValue.value.trim()
  if (!newValue || newValue === tag.value) {
    cancelRenameCategory()
    return
  }
  try {
    await api.put(`/tags/${tag.id}`, { value: newValue })
    cancelRenameCategory()
    await loadTags()
    success.value = 'Category renamed'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to rename category'
  }
}

// ── Fail2Ban tab state ──
const f2bJails = ref([])
const f2bError = ref('')
const f2bLoading = ref(false)
const f2bFetchedAt = ref(null)
const f2bDetailView = ref(null)   // null | {type: 'active'|'history', jail: string|null}
const f2bDetailData = ref([])
const f2bDetailLoading = ref(false)
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

async function toggleF2BDetail(type, jail) {
  // Toggle off if clicking same thing
  if (f2bDetailView.value &&
      f2bDetailView.value.type === type &&
      f2bDetailView.value.jail === jail) {
    f2bDetailView.value = null
    f2bDetailData.value = []
    return
  }

  f2bDetailView.value = { type, jail }
  f2bDetailLoading.value = true
  f2bDetailData.value = []

  try {
    if (type === 'active') {
      // For active bans, get IPs from existing status data, enrich with DB timestamps
      const activeIPs = []
      const jails = jail ? f2bJails.value.filter(j => j.name === jail) : f2bJails.value
      for (const j of jails) {
        for (const ip of (j.banned_ips || [])) {
          activeIPs.push({ jail: j.name, ip })
        }
      }
      // Fetch DB data to get timestamps for these IPs
      const url = jail ? `/fail2ban/bans?jail=${encodeURIComponent(jail)}` : '/fail2ban/bans'
      const { data } = await api.get(url)
      const dbBans = data.bans || []
      // Match active IPs with latest DB record for timestamps
      const enriched = activeIPs.map(a => {
        const match = dbBans.find(b => b.ip === a.ip && b.jail === a.jail)
        return match || { jail: a.jail, ip: a.ip, banned_at: null, expires_at: null, ban_count: 0 }
      })
      f2bDetailData.value = enriched
    } else {
      // Historical: straight from DB
      const url = jail ? `/fail2ban/bans?jail=${encodeURIComponent(jail)}` : '/fail2ban/bans'
      const { data } = await api.get(url)
      f2bDetailData.value = data.bans || []
    }
  } catch {
    f2bDetailData.value = []
  } finally {
    f2bDetailLoading.value = false
  }
}

function f2bDetailLabel() {
  if (!f2bDetailView.value) return ''
  const { type, jail } = f2bDetailView.value
  if (type === 'active') return jail ? `Active Bans — ${jail}` : 'All Active Bans'
  return jail ? `Ban History — ${jail}` : 'All Ban History'
}

function fmtBanDate(iso) {
  if (!iso) return '—'
  const d = new Date(iso)
  return d.toLocaleDateString('en-GB') + ' ' + d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })
}

function showJailColumn() {
  return !f2bDetailView.value?.jail
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
watch(fullBackupEncrypt, (val) => {
  if (val && !fullBackupPassphrase.value) {
    fetchPassphrase()
  }
})

watch(fullBackupExpanded, (val) => {
  if (val) fetchSavedBackups()
})

watch(activeTab, (tab) => {
  // Clear messages from previous tab
  success.value = ''
  error.value = ''
  alertSuccess.value = ''
  alertError.value = ''
  if (tab === 'fail2ban') {
    startF2BPolling()
  } else {
    stopF2BPolling()
  }
  if (tab === 'audit') {
    auditPage.value = 1
    loadAuditLog()
    nextTick(() => auditSearchInput.value?.focus())
  }
  if (tab === 'users') {
    loadUsers()
    if (usersSubTab.value === 'api-access') loadAPITokens()
  }
  if (tab === 'alerting') {
    loadAlertSettings()
    loadUsers()
  }
  if (tab === 'tags') {
    loadTags()
  }
})

// Sync activeTab when route changes (e.g. /users, /audit-log, /settings)
watch(() => route.path, (path) => {
  if (path === '/users') activeTab.value = 'users'
  else if (path === '/audit-log') activeTab.value = 'audit'
  else if (path === '/settings') activeTab.value = props.initialTab || 'general'
})

onMounted(async () => {
  await loadSLAKeys()
  await loadSettings()
  loadSnmpSettings()
  loadAutoBackupSettings()
  // Trigger initial data load for routed tabs
  if (activeTab.value === 'users') loadUsers()
  if (activeTab.value === 'audit') { auditPage.value = 1; loadAuditLog(); nextTick(() => auditSearchInput.value?.focus()) }
})

// Load token list when switching to the API Access sub-tab.
watch(usersSubTab, (v) => {
  if (v === 'api-access') loadAPITokens()
})

onUnmounted(() => {
  stopF2BPolling()
})
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h2>{{ activeTab === 'users' ? 'Users' : activeTab === 'audit' ? 'Audit Log' : 'Settings' }}</h2>
      <button
        v-if="activeTab === 'users' && auth.isAdmin && usersSubTab === 'humans'"
        class="btn btn-primary"
        @click="userShowCreate = !userShowCreate"
      >
        {{ userShowCreate ? 'Cancel' : 'Create User' }}
      </button>
      <button
        v-else-if="activeTab === 'users' && auth.isAdmin && usersSubTab === 'api-access' && !apiTokenJustCreated"
        class="btn btn-primary"
        @click="apiTokenShowCreate = !apiTokenShowCreate"
      >
        {{ apiTokenShowCreate ? 'Cancel' : 'Create Token' }}
      </button>
    </div>

    <div v-if="activeTab !== 'users' && activeTab !== 'audit'" class="tabs">
      <button
        class="tab-btn"
        :class="{ active: activeTab === 'general' }"
        @click="activeTab = 'general'"
      >General</button>
      <button
        v-if="auth.isAdmin"
        class="tab-btn"
        :class="{ active: activeTab === 'tags' }"
        @click="activeTab = 'tags'"
      >Tags</button>
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
          <template v-for="(value, key) in settings" :key="key">
            <div v-if="generalKeys.has(key)" class="form-group">
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
                :max="key === 'backup_max_copies' ? 50 : key === 'api_rate_limit_per_min' ? 10000 : undefined"
                :disabled="!auth.isAdmin"
              />
            </div>
          </template>
          <button v-if="auth.isAdmin" type="submit" class="btn btn-primary" :disabled="loading">
            {{ loading ? 'Saving...' : 'Save' }}
          </button>
          <p v-else class="text-muted">Only admins can modify settings.</p>
        </form>
      </div>

      <!-- SNMP Credentials -->
      <div v-if="auth.isAdmin" class="card collapsible-card" :class="{ expanded: snmpExpanded }" style="margin-top: 1rem;">
        <div class="collapsible-header" @click="snmpExpanded = !snmpExpanded">
          <div class="collapsible-title-row">
            <span class="collapse-arrow" :class="{ open: snmpExpanded }">&#9654;</span>
            <h3 style="margin: 0;">SNMP Credentials</h3>
          </div>
          <span class="collapsible-hint">{{ snmpExpanded ? 'collapse' : 'expand' }}</span>
        </div>
        <p class="text-muted">Credentials used by all SNMP v2c and v3 checks.</p>
        <div class="collapsible-body" :class="{ open: snmpExpanded }">
          <div class="collapsible-inner">
        <div v-if="snmpError" class="error-msg">{{ snmpError }}</div>
        <div v-if="snmpSuccess" class="success-msg" @click="snmpSuccess = ''">{{ snmpSuccess }}</div>

        <h4>SNMP v2c</h4>
        <div class="form-group">
          <label>Community String</label>
          <input v-model="snmpForm.snmp_v2c_community" placeholder="public" />
        </div>

        <h4>SNMP v3</h4>
        <div class="form-group">
          <label>Username</label>
          <input v-model="snmpForm.snmp_v3_username" placeholder="SNMPv3 username" />
        </div>
        <div class="form-group">
          <label>Security Level</label>
          <select v-model="snmpForm.snmp_v3_security_level">
            <option value="noAuthNoPriv">No Auth, No Privacy</option>
            <option value="authNoPriv">Auth, No Privacy</option>
            <option value="authPriv">Auth + Privacy</option>
          </select>
        </div>
        <template v-if="snmpForm.snmp_v3_security_level !== 'noAuthNoPriv'">
          <div class="form-group">
            <label>Auth Protocol</label>
            <select v-model="snmpForm.snmp_v3_auth_protocol">
              <option value="MD5">MD5</option>
              <option value="SHA">SHA</option>
            </select>
          </div>
          <div class="form-group">
            <label>Auth Passphrase</label>
            <input type="password" v-model="snmpForm.snmp_v3_auth_passphrase" placeholder="Auth passphrase" />
          </div>
        </template>
        <template v-if="snmpForm.snmp_v3_security_level === 'authPriv'">
          <div class="form-group">
            <label>Privacy Protocol</label>
            <select v-model="snmpForm.snmp_v3_privacy_protocol">
              <option value="DES">DES</option>
              <option value="AES">AES</option>
            </select>
          </div>
          <div class="form-group">
            <label>Privacy Passphrase</label>
            <input type="password" v-model="snmpForm.snmp_v3_privacy_passphrase" placeholder="Privacy passphrase" />
          </div>
        </template>

        <div class="form-actions">
          <button type="button" class="btn btn-primary" :disabled="snmpSaving" @click="saveSnmpSettings">
            {{ snmpSaving ? 'Saving...' : 'Save' }}
          </button>
        </div>
          </div>
        </div>
      </div>

    </div>

    <!-- ── SLA Tab ── -->
    <div v-if="activeTab === 'sla'">
      <div v-if="error" class="error-msg">{{ error }}</div>
      <div v-if="success" class="success-msg" @click="success = ''">{{ success }}</div>

      <div class="card sla-tab-card">
        <div class="sla-intro">
          <h3>SLA Compliance Thresholds</h3>
          <p class="text-muted">Define minimum uptime targets per category. Each target's preferred check 90-day uptime is compared against its category threshold.</p>
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

        <div class="privileges-bar">
          <button type="button" class="privileges-btn" @click="showSlaInfo = !showSlaInfo">
            <span class="privileges-btn-icon">?</span>
            {{ showSlaInfo ? 'Hide badge guide' : 'Badge guide' }}
            <span class="privileges-chevron" :class="{ open: showSlaInfo }">&#9662;</span>
          </button>
        </div>

        <div v-if="showSlaInfo" class="card privileges-card sla-info-card">
          <table class="privileges-table sla-info-table">
            <thead>
              <tr>
                <th>Badge</th>
                <th>Meaning</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><span class="sla-badge-example sla-badge-up-ex">UP</span></td>
                <td>The target's most recent check succeeded. The target is currently reachable and responding normally.</td>
              </tr>
              <tr>
                <td><span class="sla-badge-example sla-badge-down-ex">DOWN</span></td>
                <td>The target's most recent check failed. The target is currently unreachable or not responding as expected.</td>
              </tr>
              <tr>
                <td><span class="sla-badge-example sla-badge-healthy-ex">HEALTHY</span></td>
                <td>The target's 90-day uptime percentage meets or exceeds its category's SLA threshold. Long-term reliability is on track.</td>
              </tr>
              <tr>
                <td><span class="sla-badge-example sla-badge-unhealthy-ex">UNHEALTHY</span></td>
                <td>The target's 90-day uptime percentage has dropped below its category's SLA threshold. Long-term reliability is degraded.</td>
              </tr>
            </tbody>
          </table>
          <p class="text-muted sla-info-note">UP/DOWN reflects the <strong>current</strong> state of a single check. HEALTHY/UNHEALTHY reflects <strong>long-term</strong> reliability over 90 days. A target can be <span class="sla-badge-example sla-badge-up-ex">UP</span> right now but still <span class="sla-badge-example sla-badge-unhealthy-ex">UNHEALTHY</span> if past outages have pulled its overall uptime below the SLA threshold.</p>
        </div>
      </div>
    </div>

    <!-- ── Audit Log Tab ── -->
    <div v-if="activeTab === 'audit' && auth.isOperator">
      <div class="audit-header">
        <span class="text-muted">{{ auditTotal }} events</span>
        <input
          ref="auditSearchInput"
          v-model="auditSearch"
          @input="onAuditSearch"
          type="text"
          class="audit-search"
          placeholder="Search audit log..."
        />
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
              <td colspan="7" style="text-align:center; color:#94a3b8;">No audit events</td>
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
    <div v-if="activeTab === 'users' && auth.isAdmin">
      <div class="tabs users-subtabs">
        <button
          class="tab-btn"
          :class="{ active: usersSubTab === 'humans' }"
          @click="usersSubTab = 'humans'"
        >Humans</button>
        <button
          class="tab-btn"
          :class="{ active: usersSubTab === 'api-access' }"
          @click="usersSubTab = 'api-access'"
        >API Access</button>
      </div>

      <div v-if="usersSubTab === 'humans'">

      <div v-if="userError" class="error-msg">{{ userError }}</div>
      <div v-if="userSuccess" class="success-msg" @click="userSuccess = ''">{{ userSuccess }}</div>

      <div v-if="userShowCreate" class="card">
        <h3>Create User</h3>
        <form @submit.prevent="createUser">
          <div class="form-row">
            <div class="form-group">
              <label class="required">Username</label>
              <input v-model="userForm.username" required />
            </div>
            <div class="form-group">
              <label>Email</label>
              <input v-model="userForm.email" type="email" />
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label class="required">Password (min 15 chars)</label>
              <input v-model="userForm.password" type="password" required minlength="15" />
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
                <button class="btn btn-sm" @click="openEditUser(u)">Edit</button>
                <button class="btn btn-sm" @click="confirmSuspend(u)">
                  {{ u.status === 'active' ? 'Suspend' : 'Activate' }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Suspend/Activate Confirmation Modal -->
      <div v-if="suspendingUser" class="modal-overlay" @click.self="suspendingUser = null">
        <div class="modal-card">
          <h3>{{ suspendingUser.status === 'active' ? 'Suspend' : 'Activate' }} {{ suspendingUser.username }}?</h3>
          <p v-if="suspendingUser.status === 'active'">This user will be <strong>locked out</strong> and unable to log in until reactivated.</p>
          <p v-else>This user will regain access and be able to log in again.</p>
          <div class="form-actions">
            <button class="btn btn-primary" @click="toggleSuspend">{{ suspendingUser.status === 'active' ? 'Suspend' : 'Activate' }}</button>
            <button class="btn" @click="suspendingUser = null">Cancel</button>
          </div>
        </div>
      </div>

      <!-- Edit User Modal -->
      <div v-if="editingUser" class="modal-overlay" @click.self="closeEditModal">
        <div class="modal-card" style="max-width: 480px;">
          <h3>Edit {{ editingUser.username }}</h3>
          <div class="form-group">
            <label>Email</label>
            <input v-model="editUserForm.email" type="email" placeholder="user@example.com" />
          </div>
          <div class="form-group">
            <label>Phone</label>
            <input v-model="editUserForm.phone" type="tel" placeholder="+1234567890" />
          </div>
          <div class="form-group">
            <label>Role</label>
            <select v-model="editUserForm.role">
              <option value="admin">Admin</option>
              <option value="operator">Operator</option>
              <option value="viewer">Viewer</option>
            </select>
          </div>
          <div class="form-actions" style="margin-bottom: 1rem;">
            <button class="btn btn-primary" @click="saveEditUser(editingUser.id)">Save</button>
            <button class="btn" @click="closeEditModal">Cancel</button>
          </div>
          <hr style="border: none; border-top: 1px solid #e2e8f0; margin: 0 0 1rem;" />
          <div class="form-group">
            <label>Reset Password</label>
            <div style="display: flex; gap: 0.5rem;">
              <input v-model="resetPwForm.password" type="password" placeholder="New password (min 15 chars)" minlength="15" style="flex: 1;" />
              <button class="btn btn-sm" @click="resetPassword(editingUser.id)" :disabled="!resetPwForm.password || resetPwForm.password.length < 15">Set</button>
            </div>
          </div>
        </div>
      </div>

      <div class="privileges-bar">
        <button type="button" class="privileges-btn" @click="showPrivileges = !showPrivileges">
          <span class="privileges-btn-icon">?</span>
          {{ showPrivileges ? 'Hide privileges' : 'Role privileges' }}
          <span class="privileges-chevron" :class="{ open: showPrivileges }">&#9662;</span>
        </button>
      </div>

      <div v-if="showPrivileges" class="card privileges-card">
        <table class="privileges-table">
          <thead>
            <tr>
              <th>Action</th>
              <th><span class="badge badge-viewer">viewer</span></th>
              <th><span class="badge badge-operator">operator</span></th>
              <th><span class="badge badge-admin">admin</span></th>
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
              <td class="perm-no">No</td>
              <td class="perm-no">No</td>
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
            <tr>
              <td>Manage API tokens (create/revoke)</td>
              <td class="perm-no">No</td>
              <td class="perm-no">No</td>
              <td class="perm-yes">Yes</td>
            </tr>
          </tbody>
        </table>
      </div>
      </div> <!-- /humans sub-tab -->

      <!-- ── API Access sub-tab ── -->
      <div v-if="usersSubTab === 'api-access'">
        <p class="text-muted" style="margin: 0 0 0.75rem;">
          Bearer tokens for remote machine consumers of <code>/api/v1/*</code>. Created tokens are shown once — copy immediately, then store securely. Revocation is instant.
        </p>

        <div v-if="apiTokenError" class="error-msg">{{ apiTokenError }}</div>
        <div v-if="apiTokenSuccess" class="success-msg" @click="apiTokenSuccess = ''">{{ apiTokenSuccess }}</div>

        <!-- Plaintext display for the just-created token.
             While this card is visible, the Create flow is hidden below so
             the admin has one obvious next step: copy + dismiss. -->
        <div v-if="apiTokenJustCreated" class="card token-reveal">
          <h3 style="margin: 0 0 0.5rem;">New token — copy now</h3>
          <p class="text-muted" style="margin: 0 0 0.5rem;">
            This is the only time the plaintext value for <strong>{{ apiTokenJustCreated.name }}</strong> will be shown. Store it immediately in your consumer's config (Grafana, Zabbix, script, etc.).
          </p>
          <div class="token-plaintext-row">
            <code class="token-plaintext">{{ apiTokenJustCreated.plaintext }}</code>
            <button class="btn btn-sm btn-primary" @click="copyTokenToClipboard(apiTokenJustCreated.plaintext)">Copy</button>
          </div>
          <div style="margin-top: 0.5rem;">
            <button class="btn btn-sm btn-primary" @click="dismissJustCreatedToken">I've saved it — dismiss</button>
          </div>
        </div>

        <!-- Create flow hidden while a just-created token is being revealed. -->
        <template v-if="!apiTokenJustCreated">
          <div v-if="apiTokenShowCreate" class="card">
            <form @submit.prevent="createAPIToken">
              <div class="form-group">
                <label class="required">Token name</label>
                <input v-model="apiTokenNewName" placeholder="e.g. grafana-prod, zabbix-dr" maxlength="80" required />
                <p class="text-muted" style="margin-top: 0.25rem; font-size: 0.85rem;">
                  Used only to identify the token in this list — does not affect the token value itself.
                </p>
              </div>
              <button type="submit" class="btn btn-primary">Create</button>
            </form>
          </div>
        </template>

        <div class="card">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Prefix</th>
                <th>Created</th>
                <th>Last used</th>
                <th>Status</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="t in apiTokens" :key="t.id" :class="{ 'row-disabled': t.revoked_at }">
                <td>{{ t.name }}</td>
                <td><code>{{ t.prefix }}…</code></td>
                <td>{{ fmtDateTime(t.created_at) }}</td>
                <td>{{ fmtDateTime(t.last_used_at) }}</td>
                <td>
                  <span v-if="t.revoked_at" class="badge badge-suspended">Revoked {{ fmtDateTime(t.revoked_at) }}</span>
                  <span v-else class="badge badge-active">Active</span>
                </td>
                <td class="actions">
                  <button v-if="!t.revoked_at" class="btn btn-sm btn-danger" @click="revokeAPIToken(t.id, t.name)">Revoke</button>
                </td>
              </tr>
              <tr v-if="apiTokens.length === 0">
                <td colspan="6" class="text-muted" style="padding: 1rem; text-align: center;">No API tokens yet. Create one to give a remote consumer read access to <code>/api/v1/*</code>.</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- ── Backup & Restore Tab ── -->
    <div v-if="activeTab === 'backup' && auth.isAdmin">
      <div v-if="error" class="error-msg">{{ error }}</div>

      <div class="card backup-card" style="margin-bottom: 1rem;">
        <h3>Automated Backups</h3>
        <p class="text-muted" style="margin: 0 0 1rem;">Schedule unencrypted full backups. Respects the max backup copies setting. Backup files are stored with owner-only permissions (0600).</p>
        <div v-if="autoBackupError" class="error-msg">{{ autoBackupError }}</div>
        <div v-if="autoBackupSuccess" class="success-msg" @click="autoBackupSuccess = ''">{{ autoBackupSuccess }}</div>
        <div style="display: flex; gap: 1rem; align-items: flex-end;">
          <div class="form-group" style="width: 180px;">
            <label>Schedule</label>
            <select v-model="autoBackupForm.schedule">
              <option value="off">Off</option>
              <option value="weekly">Weekly</option>
              <option value="10days">Every 10 days</option>
              <option value="monthly">Monthly</option>
            </select>
          </div>
          <div class="form-group" style="width: 80px;">
            <label>Time (24h)</label>
            <input type="text" v-model="autoBackupForm.time" placeholder="03:00" />
          </div>
        </div>
        <div class="form-actions">
          <button class="btn btn-primary" :disabled="autoBackupSaving" @click="saveAutoBackupSettings">
            {{ autoBackupSaving ? 'Saving...' : 'Save' }}
          </button>
        </div>
      </div>

      <div class="card backup-card" style="margin-bottom: 1rem;">
        <h3>Config Backup</h3>
        <p class="text-muted" style="margin: 0 0 1rem;">Download a snapshot of all configuration data (users, targets, checks, rules, settings). Historical check results are not included.</p>
        <button class="btn btn-primary" @click="downloadBackup">Download Config Backup</button>
      </div>

      <div class="card backup-card collapsible-card" :class="{ expanded: fullBackupExpanded }" style="margin-bottom: 1rem;">
        <div class="collapsible-header" @click="fullBackupExpanded = !fullBackupExpanded">
          <div class="collapsible-title-row">
            <span class="collapse-arrow" :class="{ open: fullBackupExpanded }">&#9654;</span>
            <h3 style="margin: 0;">Full Database Backup</h3>
          </div>
          <span class="collapsible-hint">{{ fullBackupExpanded ? 'collapse' : 'expand' }}</span>
        </div>
        <p class="text-muted" style="margin: 0;">Complete database including historical data, audit logs, alert history.<br>Restore via CLI: <code>bekci restore-full &lt;file&gt;</code></p>

        <div class="collapsible-body" :class="{ open: fullBackupExpanded }">
          <div class="collapsible-inner">
            <hr class="divider" />
            <div v-if="fullBackupError" class="error-msg" style="margin-bottom: 0.75rem;">{{ fullBackupError }}</div>
            <div v-if="fullBackupSuccess" class="success-msg" style="margin-bottom: 0.75rem;">{{ fullBackupSuccess }}</div>

            <div class="full-backup-options">
              <label class="toggle-label">
                <input type="checkbox" v-model="fullBackupEncrypt" />
                <span>Encrypt backup</span>
              </label>

              <div v-if="fullBackupEncrypt" class="passphrase-section">
                <div class="passphrase-display">
                  <code class="passphrase-text">{{ fullBackupPassphrase || 'Generating...' }}</code>
                  <button class="btn btn-small" @click="copyPassphrase" title="Copy">Copy</button>
                  <button class="btn btn-small" @click="fetchPassphrase" title="Regenerate">New</button>
                </div>
                <div class="restore-warning" style="margin-top: 0.5rem;">
                  <strong>Save this passphrase</strong> &mdash; it cannot be recovered. You will need it to restore from this backup.
                </div>
              </div>

              <div class="backup-action-row">
                <label class="backup-dest-label">Backup to:</label>
                <select v-model="fullBackupDest" class="backup-dest-select">
                  <option value="download">Download</option>
                  <option value="server">Save to server</option>
                </select>
                <button
                  class="btn btn-primary"
                  :disabled="fullBackupLoading || (fullBackupEncrypt && !fullBackupPassphrase)"
                  @click="fullBackupDest === 'download' ? downloadFullBackup() : saveFullBackup()"
                >
                  {{ fullBackupLoading ? 'Preparing...' : (fullBackupDest === 'download' ? 'Download Backup' : 'Save Backup') }}
                </button>
              </div>
            </div>

            <div class="saved-backups-section">
              <h4 style="margin: 0 0 0.5rem;">Saved Backups</h4>
              <div v-if="savedBackupsLoading" class="text-muted">Loading...</div>
              <div v-else-if="savedBackups.length" class="saved-backups-list">
                <table class="saved-backups-table">
                  <thead>
                    <tr>
                      <th>Filename</th>
                      <th>Date</th>
                      <th>Size</th>
                      <th>Hash <span class="copy-hint" title="Click hash to copy">&#128203;</span></th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="b in savedBackups" :key="b.filename">
                      <td class="saved-backup-filename">{{ b.filename }}</td>
                      <td style="white-space: nowrap;">{{ formatBackupDate(b.created_at) }}</td>
                      <td style="white-space: nowrap;">{{ formatSize(b.size) }}</td>
                      <td><code class="hash-text" @click="copyHash(b.sha256)" title="Click to copy">{{ b.sha256 }}</code></td>
                      <td class="saved-backup-actions">
                        <button class="btn btn-small" @click="downloadSavedBackup(b.filename)" title="Download">Download</button>
                        <button class="btn btn-small btn-danger" @click="deleteSavedBackup(b.filename)" title="Delete">Delete</button>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <p v-else class="text-muted" style="margin: 0;">No saved backups</p>
            </div>
          </div>
        </div>
      </div>

      <div class="card backup-card">
        <h3>Config Restore</h3>
        <p class="text-muted" style="margin: 0 0 1rem;">Upload a previously exported backup file to replace all current configuration. This is a destructive operation.</p>

        <div class="restore-section">
          <input ref="restoreInput" type="file" accept=".json" style="display:none" @change="onFileSelected" />
          <button class="btn btn-restore" @click="$refs.restoreInput.click()">
            {{ restoreFile ? restoreFile.name : 'Choose backup file...' }}
          </button>

          <div v-if="restoreFile" class="restore-warning">
            <strong>Warning:</strong> Restoring will delete ALL current data and replace it with the backup contents. All sessions will be invalidated.
          </div>

          <button
            v-if="restoreFile"
            class="btn btn-restore"
            :disabled="restoring"
            @click="confirmRestore"
          >
            {{ restoring ? 'Restoring...' : 'Restore from backup' }}
          </button>
        </div>
      </div>

      <!-- Restore confirmation modal -->
      <div v-if="showRestoreConfirm" class="modal-overlay" @click.self="showRestoreConfirm = false">
        <div class="modal-card">
          <h3>Restore Backup</h3>
          <p>This will <strong>WIPE all current data</strong> and replace it with the backup. All users will be logged out. This cannot be undone.</p>
          <div class="form-actions">
            <button class="btn btn-restore" @click="executeRestore">Restore from backup</button>
            <button class="btn" @click="showRestoreConfirm = false">Cancel</button>
          </div>
        </div>
      </div>
    </div>

    <!-- ── Alerting Tab ── -->
    <div v-if="activeTab === 'alerting' && auth.isAdmin">
      <div v-if="alertError" class="error-msg">{{ alertError }}</div>
      <div v-if="alertSuccess" class="success-msg" @click="alertSuccess = ''">{{ alertSuccess }}</div>

        <!-- General alerting settings -->
        <div class="card collapsible-card" :class="{ expanded: alertSections.general }" style="margin-bottom: 1rem;">
          <div class="collapsible-header" @click="alertSections.general = !alertSections.general">
            <div class="collapsible-title-row">
              <span class="collapse-arrow" :class="{ open: alertSections.general }">&#9654;</span>
              <h3 style="margin: 0;">Alert Method &amp; Timing</h3>
            </div>
            <span class="collapsible-hint">{{ alertSections.general ? 'collapse' : 'expand' }}</span>
          </div>
          <div class="collapsible-body" :class="{ open: alertSections.general }">
            <div class="collapsible-inner">
          <div class="form-group">
            <label>Alert Method</label>
            <select v-model="alertForm.alert_method">
              <option value="">Disabled</option>
              <option value="email">Email</option>
              <option value="signal">Signal</option>
              <option value="email+signal">Email + Signal</option>
            </select>
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
            <button type="button" class="btn btn-primary" :disabled="alertSaving" @click="saveAlertGeneral">
              {{ alertSaving ? 'Saving...' : 'Save' }}
            </button>
          </div>
            </div>
          </div>
        </div>

        <!-- Email settings -->
        <div class="card collapsible-card" :class="{ expanded: alertSections.email }" style="margin-bottom: 1rem;">
          <div class="collapsible-header" @click="alertSections.email = !alertSections.email">
            <div class="collapsible-title-row">
              <span class="collapse-arrow" :class="{ open: alertSections.email }">&#9654;</span>
              <h3 style="margin: 0;">Email Alerting</h3>
            </div>
            <span class="collapsible-hint">{{ alertSections.email ? 'collapse' : 'expand' }}</span>
          </div>
          <div class="collapsible-body" :class="{ open: alertSections.email }">
            <div class="collapsible-inner">
          <div class="form-group">
            <label>Email Provider</label>
            <select v-model="alertForm.email_provider">
              <option value="resend">Resend</option>
              <option value="ms365">Microsoft 365 (SMTP)</option>
            </select>
          </div>

          <!-- Resend fields -->
          <template v-if="alertForm.email_provider === 'resend'">
            <p class="text-muted">Uses the Resend API. Requires a valid API key and sender address.</p>
            <div class="form-group">
              <label>Resend API Key</label>
              <input v-model="alertForm.resend_api_key" type="text" placeholder="re_..." autocomplete="off" />
            </div>
          </template>

          <!-- MS365 SMTP fields -->
          <template v-if="alertForm.email_provider === 'ms365'">
            <p class="text-muted">Uses SMTP AUTH to send via Microsoft 365 or any SMTP server.</p>
            <div class="form-row">
              <div class="form-group">
                <label>SMTP Host</label>
                <input v-model="alertForm.smtp_host" type="text" placeholder="smtp.office365.com" />
              </div>
              <div class="form-group" style="max-width: 120px;">
                <label>Port</label>
                <input v-model="alertForm.smtp_port" type="text" placeholder="587" />
              </div>
            </div>
            <div class="form-row">
              <div class="form-group">
                <label>Username</label>
                <input v-model="alertForm.smtp_username" type="text" placeholder="alerts@company.com" autocomplete="off" />
              </div>
              <div class="form-group">
                <label>Password</label>
                <input v-model="alertForm.smtp_password" type="password" autocomplete="off" />
              </div>
            </div>
          </template>

          <div class="form-group">
            <label>From Email Address</label>
            <input v-model="alertForm.alert_from_email" type="email" placeholder="alerts@yourdomain.com" />
          </div>

          <div class="form-actions">
            <button type="button" class="btn btn-primary" :disabled="alertSaving" @click="saveEmailSettings">
              {{ alertSaving ? 'Saving...' : 'Save' }}
            </button>
          </div>
          <hr class="section-divider" />
          <div class="form-actions">
            <input v-model="emailTestRecipient" type="email" placeholder="recipient@example.com" class="test-phone-input" />
            <button type="button" class="btn" :disabled="alertTesting" @click="sendTestEmail">
              {{ alertTesting ? 'Sending...' : 'Send Test Email' }}
            </button>
            <span v-if="emailTestResult" :class="emailTestError ? 'inline-error' : 'inline-success'">{{ emailTestResult }}</span>
          </div>
            </div>
          </div>
        </div>

        <!-- Signal settings -->
        <div class="card collapsible-card" :class="{ expanded: alertSections.signal }" style="margin-bottom: 1rem;">
          <div class="collapsible-header" @click="alertSections.signal = !alertSections.signal">
            <div class="collapsible-title-row">
              <span class="collapse-arrow" :class="{ open: alertSections.signal }">&#9654;</span>
              <h3 style="margin: 0;">Signal Alerting</h3>
            </div>
            <span class="collapsible-hint">{{ alertSections.signal ? 'collapse' : 'expand' }}</span>
          </div>
          <div class="collapsible-body" :class="{ open: alertSections.signal }">
            <div class="collapsible-inner">
          <p class="text-muted">Uses a Signal REST API gateway. Requires gateway URL and credentials.</p>

          <div class="form-group">
            <label>Send Endpoint URL</label>
            <input v-model="alertForm.signal_api_url" type="text" placeholder="https://signal-api.example.com/v2/send" />
          </div>

          <div class="form-group">
            <label>Sender Number</label>
            <input v-model="alertForm.signal_number" type="text" placeholder="+1234567890" />
          </div>

          <div class="form-row">
            <div class="form-group">
              <label>Username</label>
              <input v-model="alertForm.signal_username" type="text" autocomplete="off" />
            </div>
            <div class="form-group">
              <label>Password</label>
              <input v-model="alertForm.signal_password" type="password" autocomplete="off" />
            </div>
          </div>

          <div class="form-group">
            <label class="checkbox-label">
              <input type="checkbox"
                :checked="alertForm.signal_skip_tls === 'true'"
                @change="alertForm.signal_skip_tls = $event.target.checked ? 'true' : 'false'" />
              Skip TLS Verification
            </label>
            <span class="text-muted input-hint">Allow self-signed certificates</span>
          </div>

          <div class="form-actions">
            <button type="button" class="btn btn-primary" :disabled="alertSaving" @click="saveSignalSettings">
              {{ alertSaving ? 'Saving...' : 'Save' }}
            </button>
          </div>
          <hr class="section-divider" />
          <div class="form-actions">
            <input v-model="signalTestPhone" type="tel" placeholder="+1234567890" class="test-phone-input" />
            <button type="button" class="btn" :disabled="signalTesting || !signalTestPhone" @click="sendTestSignal">
              {{ signalTesting ? 'Sending...' : 'Send Test Signal' }}
            </button>
          </div>
            </div>
          </div>
        </div>

        <!-- Webhook settings -->
        <div class="card collapsible-card" :class="{ expanded: alertSections.webhook }" style="margin-bottom: 1rem;">
          <div class="collapsible-header" @click="alertSections.webhook = !alertSections.webhook">
            <div class="collapsible-title-row">
              <span class="collapse-arrow" :class="{ open: alertSections.webhook }">&#9654;</span>
              <h3 style="margin: 0;">Webhook Alerting</h3>
            </div>
            <span class="collapsible-hint">{{ alertSections.webhook ? 'collapse' : 'expand' }}</span>
          </div>
          <div class="collapsible-body" :class="{ open: alertSections.webhook }">
            <div class="collapsible-inner">
          <p class="text-muted">Send JSON alerts to any HTTP endpoint (SOAR, Slack, etc.)</p>

          <div v-if="webhookLastError" class="webhook-warning">
            <span class="warning-icon">!</span>
            Last webhook delivery failed: {{ webhookLastError }}
          </div>

          <div class="form-group">
            <label class="checkbox-label">
              <input type="checkbox"
                :checked="alertForm.webhook_enabled === 'true'"
                @change="alertForm.webhook_enabled = $event.target.checked ? 'true' : 'false'" />
              Enabled
            </label>
          </div>

          <div class="form-group">
            <label>Type</label>
            <select v-model="alertForm.webhook_auth_type">
              <option value="">No Auth</option>
              <option value="bearer">Bearer Token</option>
              <option value="basic">Basic Auth</option>
            </select>
          </div>

          <div class="form-group">
            <label>Webhook URL</label>
            <input type="text" v-model="alertForm.webhook_url"
              placeholder="https://soar.example.com/webhook" />
          </div>

          <template v-if="alertForm.webhook_auth_type === 'bearer'">
            <div class="form-group">
              <label>Bearer Token</label>
              <input type="password" v-model="alertForm.webhook_bearer_token"
                placeholder="Authentication token" autocomplete="off" />
            </div>
          </template>

          <template v-if="alertForm.webhook_auth_type === 'basic'">
            <div class="form-group">
              <label>Username</label>
              <input type="text" v-model="alertForm.webhook_basic_username"
                placeholder="Username" autocomplete="off" />
            </div>
            <div class="form-group">
              <label>Password</label>
              <input type="password" v-model="alertForm.webhook_basic_password"
                placeholder="Password" autocomplete="off" />
            </div>
          </template>

          <div class="form-group">
            <label class="checkbox-label">
              <input type="checkbox"
                :checked="alertForm.webhook_skip_tls === 'true'"
                @change="alertForm.webhook_skip_tls = $event.target.checked ? 'true' : 'false'" />
              Skip TLS Verification
            </label>
            <span class="text-muted input-hint">Allow self-signed certificates</span>
          </div>

          <div class="form-group">
            <label>Timeout (seconds)</label>
            <input type="number" v-model="alertForm.webhook_timeout_s" min="1" max="120" style="width: 100px;" />
            <span class="text-muted input-hint">How long to wait for webhook response (default 10)</span>
          </div>

          <div class="form-actions">
            <button type="button" class="btn btn-primary" :disabled="alertSaving" @click="saveWebhookSettings">
              {{ alertSaving ? 'Saving...' : 'Save' }}
            </button>
          </div>
          <hr class="section-divider" />
          <div class="form-actions">
            <button type="button" class="btn"
              @click="sendTestWebhook"
              :disabled="webhookTesting || alertForm.webhook_enabled !== 'true' || !alertForm.webhook_url">
              {{ webhookTesting ? 'Sending...' : 'Send Test Webhook' }}
            </button>
            <span v-if="webhookTestResult" :class="webhookTestError ? 'inline-error' : 'inline-success'">{{ webhookTestResult }}</span>
          </div>
            </div>
          </div>
        </div>

        <!-- System Alerts -->
        <div class="card collapsible-card" :class="{ expanded: alertSections.system }" style="margin-bottom: 1rem;">
          <div class="collapsible-header" @click="alertSections.system = !alertSections.system">
            <div class="collapsible-title-row">
              <span class="collapse-arrow" :class="{ open: alertSections.system }">&#9654;</span>
              <h3 style="margin: 0;">System Alerts</h3>
            </div>
            <span class="collapsible-hint">{{ alertSections.system ? 'collapse' : 'expand' }}</span>
          </div>
          <div class="collapsible-body" :class="{ open: alertSections.system }">
            <div class="collapsible-inner">
          <p class="text-muted" style="margin: 0 0 1rem;">Recipients for system alerts (unclean restarts, scheduler failures). Alerts are sent via the configured alert method above.</p>

          <div class="form-group">
            <label>
              <input type="checkbox"
                :checked="alertForm.system_alert_admins === 'true'"
                @change="alertForm.system_alert_admins = $event.target.checked ? 'true' : 'false'" />
              All admin users
            </label>
            <span class="text-muted input-hint">Send system alerts to every user with admin role who has email/phone configured</span>
          </div>

          <div class="form-group">
            <label>Additional recipients</label>
            <div class="user-select-list">
              <label v-for="u in users.filter(u => u.status === 'active')" :key="u.id" class="user-select-item">
                <input type="checkbox"
                  :checked="systemAlertUserIds.includes(u.id)"
                  @change="toggleSystemAlertUser(u.id)" />
                {{ u.username }} <span class="text-muted">({{ u.role }})</span>
              </label>
            </div>
            <span class="text-muted input-hint">Select specific users to receive system alerts (in addition to admins if enabled above)</span>
          </div>

          <div class="form-actions">
            <button type="button" class="btn btn-primary" :disabled="alertSaving" @click="saveSystemAlertSettings">
              {{ alertSaving ? 'Saving...' : 'Save' }}
            </button>
          </div>
            </div>
          </div>
        </div>
    </div>

    <!-- ── Fail2Ban Tab ── -->
    <div v-if="activeTab === 'fail2ban' && auth.isAdmin">
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

        <table v-if="f2bJails.length > 0" class="f2b-summary-table">
          <thead>
            <tr>
              <th>Jail</th>
              <th class="f2b-clickable" @click="toggleF2BDetail('active', null)"
                  :class="{ 'f2b-col-active': f2bDetailView?.type === 'active' && f2bDetailView?.jail === null }">
                Active Bans
              </th>
              <th class="f2b-clickable" @click="toggleF2BDetail('history', null)"
                  :class="{ 'f2b-col-active': f2bDetailView?.type === 'history' && f2bDetailView?.jail === null }">
                Bans (total)
              </th>
              <th>Failed (window)</th>
              <th>Failed (total)</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="jail in f2bJails" :key="jail.name">
              <td><strong>{{ jail.name }}</strong></td>
              <td>
                <span v-if="jail.currently_banned > 0"
                      class="badge badge-banned f2b-clickable"
                      :class="{ 'f2b-cell-active': f2bDetailView?.type === 'active' && f2bDetailView?.jail === jail.name }"
                      @click="toggleF2BDetail('active', jail.name)">
                  {{ jail.currently_banned }}
                </span>
                <span v-else class="badge badge-clear">0</span>
              </td>
              <td>
                <span v-if="jail.total_banned > 0"
                      class="f2b-clickable f2b-clickable-text"
                      :class="{ 'f2b-cell-active': f2bDetailView?.type === 'history' && f2bDetailView?.jail === jail.name }"
                      @click="toggleF2BDetail('history', jail.name)">
                  {{ jail.total_banned }}
                </span>
                <span v-else>0</span>
              </td>
              <td>
                <span :class="{ 'f2b-warn': jail.currently_failed > 0 }">
                  {{ jail.currently_failed }}
                </span>
              </td>
              <td>{{ jail.total_failed }}</td>
            </tr>
          </tbody>
        </table>

        <!-- Detail table -->
        <div v-if="f2bDetailView" class="f2b-detail">
          <div class="f2b-detail-header">
            <strong>{{ f2bDetailLabel() }}</strong>
            <button class="btn btn-sm" @click="f2bDetailView = null; f2bDetailData = []">Close</button>
          </div>
          <div v-if="f2bDetailLoading" class="text-muted" style="padding: 0.75rem;">Loading...</div>
          <div v-else-if="f2bDetailData.length === 0" class="text-muted" style="padding: 0.75rem;">No records found. <span style="font-size:0.8em;opacity:0.7">(History limited by fail2ban retention policy)</span></div>
          <table v-else class="f2b-detail-table">
            <thead>
              <tr>
                <th>Source IP</th>
                <th v-if="showJailColumn()">Jail</th>
                <th>Banned At</th>
                <th>Expires At</th>
                <th>Ban Count</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(ban, idx) in f2bDetailData" :key="idx">
                <td class="f2b-ip-cell">{{ ban.ip }}</td>
                <td v-if="showJailColumn()">{{ ban.jail }}</td>
                <td>{{ fmtBanDate(ban.banned_at) }}</td>
                <td>{{ fmtBanDate(ban.expires_at) }}</td>
                <td>{{ ban.ban_count }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <p v-if="!f2bError && f2bJails.length === 0 && !f2bLoading" class="text-muted">
          No jails found.
        </p>
      </div>
    </div>

    <!-- ── Tags Tab ── -->
    <div v-if="activeTab === 'tags' && auth.isAdmin">
      <div v-if="error" class="error-msg">{{ error }}</div>
      <div v-if="success" class="success-msg" @click="success = ''">{{ success }}</div>

      <div class="card collapsible-card" :class="{ expanded: tagSections.tag }" style="margin-bottom: 1rem;">
        <div class="collapsible-header" @click="tagSections.tag = !tagSections.tag">
          <div class="collapsible-title-row">
            <span class="collapse-arrow" :class="{ open: tagSections.tag }">&#9654;</span>
            <h3 style="margin: 0;">Free Tags</h3>
          </div>
          <span class="collapsible-hint">{{ tagSections.tag ? 'collapse' : 'expand' }}</span>
        </div>
        <div class="collapsible-body" :class="{ open: tagSections.tag }">
          <div class="collapsible-inner">
            <p class="text-muted" style="margin-bottom: 0.75rem;">Free-form labels you can attach to many targets (e.g. P1, P2, IT, DEVOPS). Stored uppercase; case-insensitive on entry.</p>
            <div class="tag-list">
              <div v-for="t in freeTags" :key="t.id" class="tag-item">
                <span>{{ t.value }}</span>
                <button class="btn btn-sm btn-danger" @click="deleteTag(t.id)">Delete</button>
              </div>
              <div v-if="freeTags.length === 0" class="text-muted">No free tags defined yet.</div>
            </div>
            <div class="tag-add-row">
              <input v-model="newFreeTag" placeholder="New tag (e.g. P1)" @keyup.enter="addTag('tag')" />
              <button class="btn btn-sm btn-primary" @click="addTag('tag')">Add</button>
            </div>
          </div>
        </div>
      </div>

      <div class="card collapsible-card" :class="{ expanded: tagSections.project }" style="margin-bottom: 1rem;">
        <div class="collapsible-header" @click="tagSections.project = !tagSections.project">
          <div class="collapsible-title-row">
            <span class="collapse-arrow" :class="{ open: tagSections.project }">&#9654;</span>
            <h3 style="margin: 0;">Project Tags</h3>
          </div>
          <span class="collapsible-hint">{{ tagSections.project ? 'collapse' : 'expand' }}</span>
        </div>
        <div class="collapsible-body" :class="{ open: tagSections.project }">
          <div class="collapsible-inner">
            <p class="text-muted" style="margin-bottom: 0.75rem;">Assign project names to targets for grouping and filtering.</p>
            <div class="tag-list">
              <div v-for="t in projectTags" :key="t.id" class="tag-item">
                <span>{{ t.value }}</span>
                <button class="btn btn-sm btn-danger" @click="deleteTag(t.id)">Delete</button>
              </div>
              <div v-if="projectTags.length === 0" class="text-muted">No project tags defined yet.</div>
            </div>
            <div class="tag-add-row">
              <input v-model="newProjectTag" placeholder="New project name" @keyup.enter="addTag('project')" />
              <button class="btn btn-sm btn-primary" @click="addTag('project')">Add</button>
            </div>
          </div>
        </div>
      </div>

      <div class="card collapsible-card" :class="{ expanded: tagSections.location }" style="margin-bottom: 1rem;">
        <div class="collapsible-header" @click="tagSections.location = !tagSections.location">
          <div class="collapsible-title-row">
            <span class="collapse-arrow" :class="{ open: tagSections.location }">&#9654;</span>
            <h3 style="margin: 0;">Location Tags</h3>
          </div>
          <span class="collapsible-hint">{{ tagSections.location ? 'collapse' : 'expand' }}</span>
        </div>
        <div class="collapsible-body" :class="{ open: tagSections.location }">
          <div class="collapsible-inner">
            <p class="text-muted" style="margin-bottom: 0.75rem;">Assign locations (datacenter, room, etc.) to targets.</p>
            <div class="tag-list">
              <div v-for="t in locationTags" :key="t.id" class="tag-item">
                <span>{{ t.value }}</span>
                <button class="btn btn-sm btn-danger" @click="deleteTag(t.id)">Delete</button>
              </div>
              <div v-if="locationTags.length === 0" class="text-muted">No location tags defined yet.</div>
            </div>
            <div class="tag-add-row">
              <input v-model="newLocationTag" placeholder="New location name" @keyup.enter="addTag('location')" />
              <button class="btn btn-sm btn-primary" @click="addTag('location')">Add</button>
            </div>
          </div>
        </div>
      </div>

      <div class="card collapsible-card" :class="{ expanded: tagSections.category }" style="margin-bottom: 1rem;">
        <div class="collapsible-header" @click="tagSections.category = !tagSections.category">
          <div class="collapsible-title-row">
            <span class="collapse-arrow" :class="{ open: tagSections.category }">&#9654;</span>
            <h3 style="margin: 0;">Categories</h3>
          </div>
          <span class="collapsible-hint">{{ tagSections.category ? 'collapse' : 'expand' }}</span>
        </div>
        <div class="collapsible-body" :class="{ open: tagSections.category }">
          <div class="collapsible-inner">
            <p class="text-muted" style="margin-bottom: 0.75rem;">Target categories used for grouping in SLA, SOC, and dashboard views.</p>
            <div class="tag-list">
              <div v-for="t in categoryTags" :key="t.id" class="tag-item">
                <template v-if="editingCategoryId === t.id">
                  <input v-model="editingCategoryValue" class="tag-rename-input" @keyup.enter="saveRenameCategory(t)" @keyup.escape="cancelRenameCategory" />
                  <button class="btn btn-sm btn-primary" @click="saveRenameCategory(t)">Save</button>
                  <button class="btn btn-sm btn-danger" @click="cancelRenameCategory">Cancel</button>
                </template>
                <template v-else>
                  <span>{{ t.value }}</span>
                  <button v-if="t.value !== 'Other'" class="btn btn-sm btn-primary" @click="startRenameCategory(t)" title="Rename">Rename</button>
                  <button v-if="t.value !== 'Other'" class="btn btn-sm btn-danger" @click="deleteTag(t.id)">Delete</button>
                </template>
                <div v-if="categoryDeleteError && categoryDeleteError.id === t.id" class="category-delete-notice">
                  Cannot delete — reassign these targets first: {{ categoryDeleteError.targets.join(', ') }}
                </div>
              </div>
              <div v-if="categoryTags.length === 0" class="text-muted">No categories defined yet.</div>
            </div>
            <div class="tag-add-row">
              <input v-model="newCategoryTag" placeholder="New category name" @keyup.enter="addTag('category')" />
              <button class="btn btn-sm btn-primary" @click="addTag('category')">Add</button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* ── Inline test feedback ── */
.inline-success {
  color: #16a34a;
  font-size: 0.85rem;
  margin-left: 0.75rem;
  white-space: nowrap;
}
.inline-error {
  color: #dc2626;
  font-size: 0.85rem;
  margin-left: 0.75rem;
  white-space: nowrap;
}
/* ── Webhook warning ── */
.webhook-warning {
  background: #fef3c7;
  border: 1px solid #f59e0b;
  border-radius: 6px;
  padding: 8px 12px;
  margin-bottom: 16px;
  color: #92400e;
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 8px;
}
.webhook-warning .warning-icon {
  background: #f59e0b;
  color: white;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: bold;
  font-size: 12px;
  flex-shrink: 0;
}

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

/* ── Backup & Restore ── */
.backup-card h3 {
  margin: 0 0 0.25rem;
}
.divider {
  border: none;
  border-top: 1px solid #e2e8f0;
  margin: 1rem 0;
}
.restore-section {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0.75rem;
}
.restore-warning {
  background: #fef3c7;
  color: #92400e;
  padding: 0.5rem 0.75rem;
  border-radius: 6px;
  font-size: 0.875rem;
  align-self: stretch;
}
.btn-danger {
  background: #dc2626;
  color: #fff;
  border-color: #dc2626;
}
.btn-danger:hover {
  background: #b91c1c;
}

/* ── Full Backup ── */
.collapsible-card {
  border-left: 3px solid #e2e8f0;
  transition: border-color 0.3s;
}
.collapsible-card.expanded {
  border-left-color: #ea580c;
}
.collapsible-header {
  cursor: pointer;
  user-select: none;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.collapsible-header:hover {
  color: #ea580c;
}
.collapsible-title-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.collapsible-hint {
  font-size: 0.75rem;
  color: #94a3b8;
  font-weight: 400;
  letter-spacing: 0.02em;
}
.collapsible-header:hover .collapsible-hint {
  color: #ea580c;
}
.collapse-arrow {
  font-size: 0.7rem;
  transition: transform 0.35s ease;
  color: #94a3b8;
}
.collapse-arrow.open {
  transform: rotate(90deg);
}
.collapsible-body {
  display: grid;
  grid-template-rows: 0fr;
  transition: grid-template-rows 0.35s ease;
}
.collapsible-body.open {
  grid-template-rows: 1fr;
}
.collapsible-inner {
  overflow: hidden;
}
.full-backup-options {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0.75rem;
}
.toggle-label {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  cursor: pointer;
  font-size: 0.9rem;
}
.toggle-label input[type="checkbox"] {
  width: 1rem;
  height: 1rem;
  accent-color: #ea580c;
}
.passphrase-section {
  align-self: stretch;
}
.passphrase-display {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.passphrase-text {
  font-size: 1.1rem;
  background: #f1f5f9;
  padding: 0.4rem 0.75rem;
  border-radius: 6px;
  font-family: monospace;
  letter-spacing: 0.5px;
  user-select: all;
}
.btn-small {
  padding: 0.25rem 0.6rem;
  font-size: 0.8rem;
}

/* ── Backup action row ── */
.backup-action-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.backup-dest-label {
  font-size: 0.9rem;
  font-weight: 500;
  white-space: nowrap;
}
.backup-dest-select {
  padding: 0.4rem 0.6rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: 0.875rem;
  background: #fff;
}

/* ── Saved Backups ── */
.saved-backups-section {
  margin-top: 1rem;
  padding-top: 1rem;
  border-top: 1px solid #e2e8f0;
}
.saved-backups-list {
  max-height: 260px;
  overflow-y: auto;
}
.saved-backups-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.825rem;
}
.saved-backups-table th {
  text-align: left;
  font-weight: 600;
  padding: 0.35rem 0.5rem;
  border-bottom: 1px solid #e2e8f0;
  color: #64748b;
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.03em;
}
.saved-backups-table td {
  padding: 0.4rem 0.5rem;
  border-bottom: 1px solid #f1f5f9;
}
.saved-backups-table tbody tr:hover {
  background: #f8fafc;
}
.saved-backup-filename {
  font-family: monospace;
  font-size: 0.8rem;
  word-break: break-all;
}
.saved-backup-actions {
  white-space: nowrap;
  display: flex;
  gap: 0.35rem;
}
.hash-text {
  cursor: pointer;
  font-size: 0.8rem;
  background: #f1f5f9;
  padding: 0.15rem 0.4rem;
  border-radius: 4px;
}
.hash-text:hover {
  background: #e2e8f0;
}
.copy-hint {
  font-size: 0.7rem;
  cursor: help;
  opacity: 0.6;
}
.success-msg {
  background: #dcfce7;
  color: #166534;
  padding: 0.5rem 0.75rem;
  border-radius: 6px;
  font-size: 0.875rem;
}

/* ── Restore confirmation modal ── */
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
.sla-badge-up-ex {
  background: #dcfce7;
  color: #166534;
}
.sla-badge-down-ex {
  background: #fee2e2;
  color: #991b1b;
}
.sla-badge-healthy-ex {
  background: #dcfce7;
  color: #166534;
}
.sla-badge-unhealthy-ex {
  background: #fed7aa;
  color: #9a3412;
}
.sla-info-card {
  margin-top: 0;
}
.sla-info-table td:first-child {
  white-space: nowrap;
  width: 90px;
}
table.sla-info-table th:not(:first-child),
table.sla-info-table td:not(:first-child) {
  text-align: left;
  width: auto;
}
.sla-info-note {
  margin: 0.75rem 0 0;
  font-size: 0.85rem;
  line-height: 1.5;
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
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.75rem;
}
.audit-search {
  padding: 0.35rem 0.65rem;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  font-size: 0.85rem;
  width: 250px;
}
.audit-search:focus {
  outline: none;
  border-color: #ea580c;
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
.users-subtabs {
  margin-bottom: 1rem;
}
.token-reveal {
  border-left: 4px solid #4f46e5;
  background: #eef2ff;
}
.token-plaintext-row {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  flex-wrap: wrap;
}
.token-plaintext {
  flex: 1;
  background: #fff;
  border: 1px solid #c7d2fe;
  border-radius: 4px;
  padding: 0.5rem 0.75rem;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 0.85rem;
  word-break: break-all;
  user-select: all;
}
.privileges-bar {
  display: flex;
  justify-content: center;
  padding: 1rem 0 0.5rem;
}
.privileges-btn {
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
.privileges-btn:hover {
  border-color: #818cf8;
  color: #4338ca;
  background: #eef2ff;
}
.privileges-btn-icon {
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
.privileges-chevron {
  font-size: 0.6rem;
  transition: transform 0.2s;
}
.privileges-chevron.open {
  transform: rotate(180deg);
}
.privileges-card {
  opacity: 0.85;
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
.section-divider {
  border: none;
  border-top: 1px solid #e2e8f0;
  margin: 1rem 0 0.5rem;
}
.form-actions {
  display: flex;
  gap: 0.5rem;
  margin-top: 1rem;
  align-items: center;
}
.test-phone-input {
  width: 180px;
  flex-shrink: 0;
  padding: 0.5rem 0.75rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: 0.875rem;
  background: #fff;
  color: #1e293b;
  font-family: inherit;
}
.test-phone-input:focus {
  outline: none;
  border-color: #ea580c;
  box-shadow: 0 0 0 2px rgba(234, 88, 12, 0.15);
}

/* ── Fail2Ban tab ── */
.f2b-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}
.f2b-header h3 { margin: 0; }
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
.f2b-clickable {
  cursor: pointer;
}
.f2b-clickable:hover {
  text-decoration: underline;
}
.f2b-clickable-text {
  color: #3b82f6;
  font-weight: 600;
}
.f2b-col-active,
.f2b-cell-active {
  outline: 2px solid #3b82f6;
  outline-offset: 2px;
  border-radius: 4px;
}
/* Detail panel */
.f2b-detail {
  margin-top: 1rem;
  border-top: 3px solid #e67e22;
  background: #fef9f4;
  border-radius: 0 0 8px 8px;
  overflow: hidden;
}
.f2b-detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.75rem 1rem;
  background: #fdf0e2;
  color: #7c3a0a;
  border-bottom: 1px solid #f5d5b0;
}
.f2b-detail-header .btn {
  color: #9a5c28;
  border-color: #e0b888;
  font-size: 0.75rem;
  padding: 0.2rem 0.6rem;
}
.f2b-detail-header .btn:hover {
  background: #f5d5b0;
  color: #7c3a0a;
}
.f2b-detail-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.85rem;
}
.f2b-detail-table thead th {
  background: #fdf0e2;
  color: #9a5c28;
  padding: 0.5rem 1rem;
  text-align: left;
  font-weight: 600;
  text-transform: uppercase;
  font-size: 0.7rem;
  letter-spacing: 0.05em;
  border-bottom: 1px solid #f5d5b0;
}
.f2b-detail-table tbody td {
  padding: 0.5rem 1rem;
  color: #4a2c0a;
  border-bottom: 1px solid #fdf0e2;
}
.f2b-detail-table tbody tr:hover {
  background: #fdf0e2;
}
.f2b-ip-cell {
  font-family: 'SF Mono', 'Consolas', 'Monaco', monospace;
  color: #c0392b !important;
}
.tag-list {
  margin-bottom: 0.75rem;
}
.tag-item {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
  padding: 0.4rem 0;
  border-bottom: 1px solid var(--border);
}
.tag-item > span {
  flex: 1;
}
.tag-add-row {
  display: flex;
  gap: 0.5rem;
  margin-top: 0.5rem;
}
.tag-add-row input {
  flex: 1;
  max-width: 300px;
  padding: 0.5rem 0.75rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: 0.875rem;
  background: #fff;
  color: #1e293b;
}
.tag-rename-input {
  flex: 1;
  max-width: 200px;
  font-size: 0.85rem;
  padding: 0.2rem 0.5rem;
}
.category-delete-notice {
  width: 100%;
  margin-top: 0.35rem;
  padding: 0.4rem 0.6rem;
  font-size: 0.8rem;
  color: #b45309;
  background: #fef3c7;
  border: 1px solid #fde68a;
  border-radius: 4px;
}
.user-select-list {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  max-height: 200px;
  overflow-y: auto;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  padding: 0.5rem;
  background: #f8fafc;
}
.user-select-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.85rem;
  cursor: pointer;
}
.user-select-item:hover {
  background: #e2e8f0;
  border-radius: 4px;
  padding: 0.1rem 0.3rem;
  margin: -0.1rem -0.3rem;
}
</style>
