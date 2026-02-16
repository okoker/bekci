<script setup>
import { ref, onMounted, computed } from 'vue'
import api from '../api'

const entries = ref([])
const total = ref(0)
const page = ref(1)
const limit = 50
const loading = ref(false)
const error = ref('')

const totalPages = computed(() => Math.ceil(total.value / limit) || 1)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const { data } = await api.get('/audit-log', { params: { page: page.value, limit } })
    entries.value = data.entries
    total.value = data.total
  } catch (e) {
    error.value = 'Failed to load audit log'
  } finally {
    loading.value = false
  }
}

function prevPage() {
  if (page.value > 1) { page.value--; load() }
}
function nextPage() {
  if (page.value < totalPages.value) { page.value++; load() }
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

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h2>Audit Log</h2>
      <span class="text-muted">{{ total }} entries</span>
    </div>

    <div v-if="error" class="error-msg">{{ error }}</div>

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
          <tr v-if="loading">
            <td colspan="7" style="text-align:center; color:#94a3b8;">Loading...</td>
          </tr>
          <tr v-else-if="entries.length === 0">
            <td colspan="7" style="text-align:center; color:#94a3b8;">No audit entries</td>
          </tr>
          <tr v-for="e in entries" :key="e.id">
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

    <div class="pagination" v-if="totalPages > 1">
      <button class="btn btn-sm" :disabled="page <= 1" @click="prevPage">Prev</button>
      <span>Page {{ page }} of {{ totalPages }}</span>
      <button class="btn btn-sm" :disabled="page >= totalPages" @click="nextPage">Next</button>
    </div>
  </div>
</template>

<style scoped>
.nowrap { white-space: nowrap; }
.detail-cell { max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.status-ok { color: #16a34a; font-weight: 600; font-size: 0.8rem; }
.status-fail { color: #dc2626; font-weight: 600; font-size: 0.8rem; }
.pagination { display: flex; align-items: center; justify-content: center; gap: 1rem; margin-top: 1rem; }

.badge-action-auth { background: #dbeafe; color: #1d4ed8; }
.badge-action-create { background: #dcfce7; color: #166534; }
.badge-action-delete { background: #fee2e2; color: #991b1b; }
.badge-action-default { background: #f1f5f9; color: #475569; }
</style>
