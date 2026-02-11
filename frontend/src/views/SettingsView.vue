<script setup>
import { ref, onMounted } from 'vue'
import { useAuthStore } from '../stores/auth'
import api from '../api'

const auth = useAuthStore()
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
    await api.put('/settings', settings.value)
    success.value = 'Settings saved'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to save settings'
  } finally {
    loading.value = false
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
  </div>
</template>
