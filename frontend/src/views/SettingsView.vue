<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import api from '../api'

const auth = useAuthStore()
const router = useRouter()
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

// Backup & Restore state
const restoreFile = ref(null)
const restoring = ref(false)

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

onMounted(loadSettings)
</script>

<template>
  <div class="page">
    <h2>Settings</h2>

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

    <div v-if="auth.isAdmin" class="card backup-card">
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
</template>

<style scoped>
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
</style>
