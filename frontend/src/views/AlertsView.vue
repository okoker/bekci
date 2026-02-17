<script setup>
import { ref, computed, onMounted } from 'vue'
import api from '../api'

const entries = ref([])
const total = ref(0)
const page = ref(1)
const limit = 50
const loading = ref(false)
const error = ref('')

const totalPages = computed(() => Math.ceil(total.value / limit) || 1)

async function loadAlerts() {
  loading.value = true
  error.value = ''
  try {
    const { data } = await api.get('/alerts', { params: { page: page.value, limit } })
    entries.value = data.entries
    total.value = data.total
  } catch (e) {
    error.value = 'Failed to load alert history'
  } finally {
    loading.value = false
  }
}

function prevPage() {
  if (page.value > 1) { page.value--; loadAlerts() }
}
function nextPage() {
  if (page.value < totalPages.value) { page.value++; loadAlerts() }
}

function fmtDate(d) {
  if (!d) return '-'
  const dt = new Date(d)
  return dt.toLocaleDateString('en-GB') + ' ' + dt.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function typeClass(t) {
  if (t === 'firing') return 'badge-firing'
  if (t === 'recovery') return 'badge-recovery'
  if (t === 're-alert') return 'badge-realert'
  return 'badge-default'
}

onMounted(() => loadAlerts())
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h2>Alerts</h2>
      <span class="text-muted">{{ total }} total</span>
    </div>

    <div v-if="error" class="error-msg">{{ error }}</div>

    <div class="card">
      <table>
        <thead>
          <tr>
            <th>Timestamp</th>
            <th>Target</th>
            <th>Type</th>
            <th>Recipient</th>
            <th>Message</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="5" style="text-align:center; color:#94a3b8;">Loading...</td>
          </tr>
          <tr v-else-if="entries.length === 0">
            <td colspan="5" style="text-align:center; color:#94a3b8;">No alerts sent yet</td>
          </tr>
          <tr v-for="e in entries" :key="e.id">
            <td class="nowrap">{{ fmtDate(e.sent_at) }}</td>
            <td>{{ e.target_name }}</td>
            <td><span class="badge" :class="typeClass(e.alert_type)">{{ e.alert_type }}</span></td>
            <td>{{ e.recipient_name }}</td>
            <td class="msg-cell">{{ e.message || '-' }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="pagination" v-if="totalPages > 1">
      <button class="btn btn-sm" :disabled="page <= 1" @click="prevPage">Prev</button>
      <span>Page {{ page }} of {{ totalPages }}</span>
      <button class="btn btn-sm" :disabled="page >= totalPages" @click="nextPage">Next</button>
    </div>
  </div>
</template>

<style scoped>
.nowrap { white-space: nowrap; }
.msg-cell { max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.pagination { display: flex; align-items: center; justify-content: center; gap: 1rem; margin-top: 1rem; }
.badge-firing { background: #fee2e2; color: #991b1b; }
.badge-recovery { background: #dcfce7; color: #166534; }
.badge-realert { background: #fef3c7; color: #92400e; }
.badge-default { background: #f1f5f9; color: #475569; }
</style>
