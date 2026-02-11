<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'

const router = useRouter()
const dashboardData = ref([])
const loading = ref(true)
const error = ref('')
const lastUpdated = ref(null)

// Per-check history data (loaded on expand)
const historyData = ref({}) // checkId -> { bar90d: [], bar4h: [] }
const expandedCheckId = ref(null)

let refreshTimer = null

async function loadDashboard() {
  try {
    const { data } = await api.get('/dashboard/status')
    dashboardData.value = data
    lastUpdated.value = new Date()
    error.value = ''
  } catch (e) {
    error.value = 'Failed to load dashboard'
  } finally {
    loading.value = false
  }
}

async function loadHistory(checkId) {
  if (historyData.value[checkId]) return // already loaded
  try {
    const [res90d, res4h] = await Promise.all([
      api.get(`/dashboard/history/${checkId}?range=90d`),
      api.get(`/dashboard/history/${checkId}?range=4h`),
    ])
    historyData.value[checkId] = {
      bar90d: res90d.data,
      bar4h: res4h.data,
    }
  } catch {
    // silently fail
  }
}

function toggleCheck(checkId) {
  if (expandedCheckId.value === checkId) {
    expandedCheckId.value = null
  } else {
    expandedCheckId.value = checkId
    loadHistory(checkId)
  }
}

// Sort: problems first (any project with a down check)
function sortedProjects() {
  return [...dashboardData.value].sort((a, b) => {
    const aDown = hasDownCheck(a)
    const bDown = hasDownCheck(b)
    if (aDown && !bDown) return -1
    if (!aDown && bDown) return 1
    return a.name.localeCompare(b.name)
  })
}

function hasDownCheck(project) {
  return project.targets?.some(t => t.checks?.some(c => c.last_status === 'down'))
}

function hasDownCheckTarget(target) {
  return target.checks?.some(c => c.last_status === 'down')
}

// Uptime bar color helpers
function uptimeColor(pct) {
  if (pct < 0) return '#e2e8f0' // no data (gray)
  if (pct >= 99.9) return '#22c55e' // green
  if (pct >= 95) return '#eab308'   // yellow
  if (pct >= 80) return '#f97316'   // orange
  return '#ef4444'                   // red
}

function statusColor(status) {
  return status === 'up' ? '#22c55e' : '#ef4444'
}

function formatTime(dateStr) {
  const d = new Date(dateStr)
  return d.toLocaleDateString('en-GB') + ' ' + d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })
}

function formatDate(dateStr) {
  return dateStr // already YYYY-MM-DD from API
}

onMounted(() => {
  loadDashboard()
  refreshTimer = setInterval(() => {
    loadDashboard()
    // Clear cached history so it refreshes on next expand
    historyData.value = {}
  }, 30000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h2>Dashboard</h2>
      <span v-if="lastUpdated" class="text-muted">
        Updated {{ lastUpdated.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit' }) }}
      </span>
    </div>

    <div v-if="error" class="error-msg">{{ error }}</div>

    <div v-if="loading" class="card placeholder-card">
      <p>Loading dashboard...</p>
    </div>

    <div v-else-if="dashboardData.length === 0" class="card placeholder-card">
      <p>No targets configured yet.</p>
      <p class="text-muted">Go to <router-link to="/targets">Targets</router-link> to add monitoring targets.</p>
    </div>

    <template v-else>
      <div v-for="project in sortedProjects()" :key="project.id" class="project-section">
        <h3 class="project-name">{{ project.name }}</h3>

        <div v-for="target in project.targets" :key="target.id" class="target-card card">
          <div class="target-header" @click="router.push('/targets')">
            <span class="target-name">
              {{ target.name }}
              <span class="target-host text-muted">{{ target.host }}</span>
            </span>
            <span v-if="hasDownCheckTarget(target)" class="badge badge-down">DOWN</span>
            <span v-else class="badge badge-up">UP</span>
          </div>

          <div v-if="target.checks.length === 0" class="text-muted" style="padding: 0.5rem 0; font-size: 0.85rem;">
            No checks configured
          </div>

          <div v-for="check in target.checks" :key="check.id" class="check-row" @click="toggleCheck(check.id)">
            <div class="check-info">
              <span :class="['status-dot', check.last_status === 'up' ? 'dot-up' : (check.last_status === 'down' ? 'dot-down' : 'dot-unknown')]"></span>
              <span class="check-name">{{ check.name }}</span>
              <span class="badge badge-type">{{ check.type }}</span>
              <span v-if="check.response_ms > 0" class="check-response text-muted">{{ check.response_ms }}ms</span>
              <span v-if="check.uptime_90d_pct >= 0" class="check-uptime" :style="{ color: uptimeColor(check.uptime_90d_pct) }">
                {{ check.uptime_90d_pct.toFixed(1) }}%
              </span>
            </div>

            <!-- Uptime bars (always show placeholders, fill when data loaded) -->
            <div class="uptime-bars">
              <!-- 90-day bar -->
              <div class="bar-container bar-90d" title="90-day uptime">
                <template v-if="historyData[check.id]?.bar90d">
                  <div v-for="(day, i) in historyData[check.id].bar90d" :key="'90d-' + i"
                    class="bar-segment"
                    :style="{ background: uptimeColor(day.uptime_pct) }"
                    :title="formatDate(day.date) + ': ' + day.uptime_pct.toFixed(1) + '% (' + day.total_checks + ' checks)'">
                  </div>
                </template>
                <div v-else class="bar-placeholder">90d</div>
              </div>
              <!-- 4-hour bar -->
              <div class="bar-container bar-4h" title="Last 4 hours">
                <template v-if="historyData[check.id]?.bar4h">
                  <div v-for="(r, i) in historyData[check.id].bar4h" :key="'4h-' + i"
                    class="bar-segment"
                    :style="{ background: statusColor(r.status) }"
                    :title="formatTime(r.checked_at) + ': ' + r.status + ' (' + r.response_ms + 'ms)'">
                  </div>
                </template>
                <div v-else class="bar-placeholder">4h</div>
              </div>
            </div>

            <!-- Expanded detail -->
            <div v-if="expandedCheckId === check.id && check.last_message" class="check-detail">
              {{ check.last_message }}
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.project-section {
  margin-bottom: 1.5rem;
}
.project-name {
  font-size: 1rem;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 0.5rem;
}

.target-card {
  padding: 0.75rem 1rem;
}
.target-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
  cursor: pointer;
}
.target-name {
  font-weight: 600;
}
.target-host {
  font-weight: 400;
  font-size: 0.85rem;
  margin-left: 0.5rem;
}

.check-row {
  padding: 0.5rem 0;
  border-top: 1px solid #f1f5f9;
  cursor: pointer;
}
.check-row:hover { background: #fafbfc; }

.check-info {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.25rem;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.dot-up { background: #22c55e; }
.dot-down { background: #ef4444; }
.dot-unknown { background: #e2e8f0; }

.check-name { font-weight: 500; font-size: 0.9rem; }
.check-response { font-size: 0.8rem; }
.check-uptime { font-weight: 600; font-size: 0.85rem; margin-left: auto; }

.badge-type {
  background: #e0e7ff;
  color: #3730a3;
  font-size: 0.65rem;
  text-transform: uppercase;
  padding: 0.1rem 0.375rem;
  border-radius: 10px;
  font-weight: 600;
}
.badge-up {
  background: #dcfce7;
  color: #166534;
  font-size: 0.7rem;
  font-weight: 600;
  padding: 0.1rem 0.5rem;
  border-radius: 10px;
}
.badge-down {
  background: #fee2e2;
  color: #991b1b;
  font-size: 0.7rem;
  font-weight: 600;
  padding: 0.1rem 0.5rem;
  border-radius: 10px;
}

/* Uptime bars */
.uptime-bars {
  display: flex;
  gap: 0.75rem;
  margin-top: 0.25rem;
}

.bar-container {
  display: flex;
  gap: 1px;
  align-items: flex-end;
  height: 20px;
  flex: 1;
  background: #f8fafc;
  border-radius: 3px;
  overflow: hidden;
}

.bar-segment {
  flex: 1;
  height: 100%;
  min-width: 1px;
  transition: opacity 0.15s;
}
.bar-segment:hover {
  opacity: 0.7;
}

.bar-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  font-size: 0.65rem;
  color: #cbd5e1;
}

.bar-90d { max-width: 60%; }
.bar-4h { max-width: 40%; }

.check-detail {
  font-size: 0.8rem;
  color: #64748b;
  padding: 0.25rem 0 0 1.25rem;
}
</style>
