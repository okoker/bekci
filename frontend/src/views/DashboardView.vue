<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'

const router = useRouter()
const dashboardData = ref([])
const loading = ref(true)
const error = ref('')
const lastUpdated = ref(null)

// Per-check history data
const historyData = ref({}) // checkId -> { bar90d: [], bar4h: [] }
const expandedTargetId = ref(null)
const expandedCheckId = ref(null)

let refreshTimer = null

// Pre-built empty bar arrays (gray placeholders before data loads)
const empty90d = Array.from({ length: 90 }, () => ({ date: '', uptime_pct: -1, total_checks: 0 }))
const empty4h = Array.from({ length: 48 }, () => ({ status: 'none', response_ms: 0, checked_at: '' }))

async function loadDashboard() {
  try {
    const { data } = await api.get('/dashboard/status')
    dashboardData.value = data
    lastUpdated.value = new Date()
    error.value = ''

    // Auto-load history for all checks
    const allCheckIds = []
    for (const proj of data) {
      for (const t of proj.targets || []) {
        for (const c of t.checks || []) {
          allCheckIds.push(c.id)
        }
      }
    }
    for (const cid of allCheckIds) {
      if (!historyData.value[cid]) {
        loadHistory(cid)
      }
    }
  } catch (e) {
    error.value = 'Failed to load dashboard'
  } finally {
    loading.value = false
  }
}

async function loadHistory(checkId) {
  try {
    const [res90d, res4h] = await Promise.all([
      api.get(`/dashboard/history/${checkId}?range=90d`),
      api.get(`/dashboard/history/${checkId}?range=4h`),
    ])
    historyData.value[checkId] = {
      bar90d: pad90dBars(res90d.data),
      bar4h: pad4hBars(res4h.data),
    }
  } catch {
    // silently fail
  }
}

// Pad 90d data to exactly 90 entries (one per day, oldest first)
function pad90dBars(data) {
  const map = {}
  for (const d of data) map[d.date] = d

  const bars = []
  const now = new Date()
  for (let i = 89; i >= 0; i--) {
    const dt = new Date(now)
    dt.setDate(dt.getDate() - i)
    const key = dt.toISOString().slice(0, 10)
    if (map[key]) {
      bars.push(map[key])
    } else {
      bars.push({ date: key, uptime_pct: -1, total_checks: 0 })
    }
  }
  return bars
}

// Pad 4h data to exactly 48 entries (one per 5-min slot, oldest first)
function pad4hBars(data) {
  const bars = []
  const now = new Date()
  const slotMs = 5 * 60 * 1000
  const start = new Date(now.getTime() - 4 * 60 * 60 * 1000)

  for (let i = 0; i < 48; i++) {
    const slotStart = new Date(start.getTime() + i * slotMs)
    const slotEnd = new Date(slotStart.getTime() + slotMs)
    const inSlot = data.filter(r => {
      const t = new Date(r.checked_at).getTime()
      return t >= slotStart.getTime() && t < slotEnd.getTime()
    })
    if (inSlot.length > 0) {
      const last = inSlot[inSlot.length - 1]
      bars.push({ status: last.status, response_ms: last.response_ms, checked_at: last.checked_at })
    } else {
      bars.push({ status: 'none', response_ms: 0, checked_at: slotStart.toISOString() })
    }
  }
  return bars
}

function getPreferredCheck(target) {
  return target.checks.find(c => c.type === target.preferred_check_type) || target.checks[0]
}

function toggleTarget(targetId) {
  if (expandedTargetId.value === targetId) {
    expandedTargetId.value = null
  } else {
    expandedTargetId.value = targetId
    expandedCheckId.value = null
  }
}

function toggleCheck(e, checkId) {
  e.stopPropagation()
  expandedCheckId.value = expandedCheckId.value === checkId ? null : checkId
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
  if (pct < 0) return '#d1d5db'
  if (pct >= 99.9) return '#48bb78'
  if (pct >= 95) return '#ecc94b'
  if (pct >= 80) return '#ed8936'
  return '#f56565'
}

function statusColor(status) {
  if (status === 'none') return '#d1d5db'
  return status === 'up' ? '#48bb78' : '#f56565'
}

function formatTooltip90d(day) {
  if (!day) return ''
  const d = new Date(day.date)
  const dateStr = d.toLocaleDateString('en-GB')
  return `${dateStr}: ${day.uptime_pct.toFixed(1)}% (${day.total_checks} checks)`
}

function formatTooltip4h(r) {
  if (!r) return ''
  const d = new Date(r.checked_at)
  const timeStr = d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })
  return `${timeStr}: ${r.status} (${r.response_ms}ms)`
}

onMounted(() => {
  loadDashboard()
  refreshTimer = setInterval(() => {
    loadDashboard()
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
          <!-- Collapsed view: target header + preferred check bars -->
          <div class="target-header" @click="toggleTarget(target.id)">
            <div class="target-header-left">
              <span class="expand-icon">{{ expandedTargetId === target.id ? '&#9660;' : '&#9654;' }}</span>
              <span v-if="target.checks.length > 0"
                :class="['status-dot', hasDownCheckTarget(target) ? 'dot-down' : (getPreferredCheck(target)?.last_status === 'up' ? 'dot-up' : 'dot-unknown')]">
              </span>
              <span class="target-name">{{ target.name }}</span>
              <span class="target-host text-muted">{{ target.host }}</span>
            </div>
            <div class="target-header-right">
              <template v-if="target.checks.length > 0">
                <span class="badge badge-type">{{ getPreferredCheck(target)?.type }}</span>
                <span v-if="getPreferredCheck(target)?.response_ms > 0" class="check-response text-muted">
                  {{ getPreferredCheck(target)?.response_ms }}ms
                </span>
                <span v-if="getPreferredCheck(target)?.uptime_90d_pct >= 0" class="check-uptime"
                  :style="{ color: uptimeColor(getPreferredCheck(target)?.uptime_90d_pct) }">
                  {{ getPreferredCheck(target)?.uptime_90d_pct.toFixed(1) }}%
                </span>
              </template>
              <span v-if="hasDownCheckTarget(target)" class="badge badge-down">DOWN</span>
              <span v-else-if="target.checks.length > 0" class="badge badge-up">UP</span>
            </div>
          </div>

          <!-- Collapsed: preferred check bars -->
          <div v-if="expandedTargetId !== target.id && target.checks.length > 0" class="collapsed-bars" @click="toggleTarget(target.id)">
            <div class="uptime-bars-row">
              <div class="bar-section bar-90d-section">
                <div class="bar-track">
                  <div v-for="(day, i) in (historyData[getPreferredCheck(target)?.id]?.bar90d || empty90d)" :key="'90d-' + i"
                    class="bar-tick"
                    :style="{ background: uptimeColor(day.uptime_pct) }"
                    :title="formatTooltip90d(day)">
                  </div>
                </div>
                <div class="bar-labels">
                  <span>90 days ago</span>
                  <span>Today</span>
                </div>
              </div>
              <div class="bar-section bar-4h-section">
                <div class="bar-track">
                  <div v-for="(r, i) in (historyData[getPreferredCheck(target)?.id]?.bar4h || empty4h)" :key="'4h-' + i"
                    class="bar-tick"
                    :style="{ background: statusColor(r.status) }"
                    :title="formatTooltip4h(r)">
                  </div>
                </div>
                <div class="bar-labels">
                  <span>4h ago</span>
                  <span>Now</span>
                </div>
              </div>
            </div>
          </div>

          <!-- Expanded: all checks with individual bars -->
          <div v-if="expandedTargetId === target.id" class="expanded-checks">
            <div v-if="target.checks.length === 0" class="text-muted" style="padding: 0.5rem 0; font-size: 0.85rem;">
              No checks configured
            </div>

            <div v-for="check in target.checks" :key="check.id" class="check-row" @click="toggleCheck($event, check.id)">
              <div class="check-info">
                <span :class="['status-dot', check.last_status === 'up' ? 'dot-up' : (check.last_status === 'down' ? 'dot-down' : 'dot-unknown')]"></span>
                <span class="check-name">{{ check.name }}</span>
                <span class="badge badge-type">{{ check.type }}</span>
                <span v-if="check.response_ms > 0" class="check-response text-muted">{{ check.response_ms }}ms</span>
                <span v-if="check.uptime_90d_pct >= 0" class="check-uptime" :style="{ color: uptimeColor(check.uptime_90d_pct) }">
                  {{ check.uptime_90d_pct.toFixed(1) }}%
                </span>
              </div>

              <div class="uptime-bars-row">
                <div class="bar-section bar-90d-section">
                  <div class="bar-track">
                    <div v-for="(day, i) in (historyData[check.id]?.bar90d || empty90d)" :key="'90d-' + i"
                      class="bar-tick"
                      :style="{ background: uptimeColor(day.uptime_pct) }"
                      :title="formatTooltip90d(day)">
                    </div>
                  </div>
                  <div class="bar-labels">
                    <span>90 days ago</span>
                    <span>Today</span>
                  </div>
                </div>
                <div class="bar-section bar-4h-section">
                  <div class="bar-track">
                    <div v-for="(r, i) in (historyData[check.id]?.bar4h || empty4h)" :key="'4h-' + i"
                      class="bar-tick"
                      :style="{ background: statusColor(r.status) }"
                      :title="formatTooltip4h(r)">
                    </div>
                  </div>
                  <div class="bar-labels">
                    <span>4h ago</span>
                    <span>Now</span>
                  </div>
                </div>
              </div>

              <div v-if="expandedCheckId === check.id && check.last_message" class="check-detail">
                {{ check.last_message }}
              </div>
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
  font-size: 0.85rem;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 0.5rem;
  font-weight: 700;
}

.target-card {
  padding: 0.75rem 1rem;
  margin-bottom: 1rem;
}
.target-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  cursor: pointer;
}
.target-header-left {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.target-header-right {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.target-name {
  font-weight: 600;
}
.target-host {
  font-weight: 400;
  font-size: 0.85rem;
}

.expand-icon {
  display: inline-block;
  width: 1rem;
  font-size: 0.7rem;
  color: #94a3b8;
}

.collapsed-bars {
  margin-top: 0.5rem;
  cursor: pointer;
}

.expanded-checks {
  margin-top: 0.5rem;
  border-top: 1px solid #f1f5f9;
  padding-top: 0.5rem;
}

.check-row {
  padding: 0.6rem 0;
  border-top: 1px solid #f1f5f9;
  cursor: pointer;
}
.check-row:first-child {
  border-top: none;
}
.check-row:hover { background: #fafbfc; }

.check-info {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.35rem;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.dot-up { background: #48bb78; }
.dot-down { background: #f56565; }
.dot-unknown { background: #d1d5db; }

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

/* Uptime bars — thin vertical barcode style */
.uptime-bars-row {
  display: flex;
  gap: 1rem;
  margin-top: 0.25rem;
}

.bar-section {
  display: flex;
  flex-direction: column;
}

.bar-90d-section { flex: 6; }
.bar-4h-section { flex: 4; }

.bar-track {
  display: flex;
  gap: 1px;
  height: 28px;
  align-items: stretch;
}

.bar-tick {
  flex: 1;
  min-width: 0;
  border-radius: 1px;
  transition: opacity 0.15s;
}

.bar-tick:hover {
  opacity: 0.65;
}

.bar-labels {
  display: flex;
  justify-content: space-between;
  font-size: 0.6rem;
  color: #a0aec0;
  margin-top: 2px;
  padding: 0 1px;
}

.check-detail {
  font-size: 0.8rem;
  color: #64748b;
  padding: 0.25rem 0 0 1.25rem;
}
</style>
