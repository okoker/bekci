<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'

const router = useRouter()
const dashboardData = ref([])
const loading = ref(true)
const error = ref('')
const lastUpdated = ref(null)
const activeCategory = ref('All')
const categories = ['All', 'Network', 'Security', 'Physical Security', 'Key Services', 'Other']

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
    for (const t of data) {
      for (const c of t.checks || []) {
        if (!historyData.value[c.id]) {
          loadHistory(c.id)
        }
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

// Filter by category, sort: problems first (worst uptime at top), then healthy alphabetically
function filteredAndSortedTargets() {
  let list = dashboardData.value
  if (activeCategory.value !== 'All') {
    list = list.filter(t => t.category === activeCategory.value)
  }
  return [...list].sort((a, b) => {
    const aDown = isTargetDown(a)
    const bDown = isTargetDown(b)
    if (aDown && !bDown) return -1
    if (!aDown && bDown) return 1
    if (aDown && bDown) return getWorstUptime(a) - getWorstUptime(b)
    // UNHEALTHY SLA before HEALTHY/no-SLA
    const aUnhealthy = a.sla_status === 'unhealthy'
    const bUnhealthy = b.sla_status === 'unhealthy'
    if (aUnhealthy && !bUnhealthy) return -1
    if (!aUnhealthy && bUnhealthy) return 1
    const diff = getWorstUptime(a) - getWorstUptime(b)
    return diff !== 0 ? diff : a.name.localeCompare(b.name)
  })
}

function isTargetDown(target) {
  if (target.state === 'unhealthy') return true
  if (target.state === 'healthy') return false
  // Fallback: no rule state — derive from raw check status
  return target.checks?.some(c => c.last_status === 'down')
}

function targetStateLabel(target) {
  if (target.state === 'unhealthy') return 'DOWN'
  if (target.state === 'healthy') return 'UP'
  // Fallback: no rule state
  if (target.checks?.some(c => c.last_status === 'down')) return 'DOWN'
  if (target.checks?.some(c => c.last_status === 'up')) return 'UP'
  return ''
}

function targetStateClass(target) {
  const label = targetStateLabel(target)
  if (label === 'DOWN') return 'badge-down'
  if (label === 'UP') return 'badge-up'
  return ''
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

function slaLabel(target) {
  if (target.sla_status === 'healthy') return 'HEALTHY'
  if (target.sla_status === 'unhealthy') return 'UNHEALTHY'
  return ''
}

function slaClass(target) {
  if (target.sla_status === 'healthy') return 'badge-sla-healthy'
  if (target.sla_status === 'unhealthy') return 'badge-sla-unhealthy'
  return ''
}

function categoryClass(cat) {
  if (cat === 'Security') return 'badge-cat-security'
  if (cat === 'Network') return 'badge-cat-network'
  if (cat === 'Physical Security') return 'badge-cat-physical'
  if (cat === 'Key Services') return 'badge-cat-server'
  return 'badge-cat-other'
}

function categoryCount(cat) {
  if (cat === 'All') return dashboardData.value.length
  return dashboardData.value.filter(t => t.category === cat).length
}

function categoryHasProblems(cat) {
  const targets = cat === 'All' ? dashboardData.value : dashboardData.value.filter(t => t.category === cat)
  return targets.some(t => isTargetDown(t))
}

function getWorstUptime(target) {
  if (!target.checks || target.checks.length === 0) return 100
  return Math.min(...target.checks.map(c => c.uptime_90d_pct >= 0 ? c.uptime_90d_pct : 100))
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
      <!-- Category filter bar -->
      <div class="filter-bar">
        <button v-for="cat in categories" :key="cat"
          :class="['filter-btn', { active: activeCategory === cat, 'has-problems': categoryHasProblems(cat) }]"
          @click="activeCategory = cat">
          {{ cat }} <span class="filter-count">({{ categoryCount(cat) }})</span>
        </button>
      </div>

      <div v-for="target in filteredAndSortedTargets()" :key="target.id" class="target-card card">
        <!-- Collapsed view: target header + preferred check bars -->
        <div class="target-header" @click="toggleTarget(target.id)">
          <div class="target-header-left">
            <span class="expand-icon">{{ expandedTargetId === target.id ? '&#9660;' : '&#9654;' }}</span>
            <span v-if="target.checks.length > 0"
              :class="['status-dot', isTargetDown(target) ? 'dot-down' : (targetStateLabel(target) === 'UP' ? 'dot-up' : 'dot-unknown')]">
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
            <span :class="['badge', categoryClass(target.category)]" style="font-size: 0.6rem;">
              {{ target.category }}
            </span>
            <span v-if="slaLabel(target)" :class="['badge', slaClass(target)]">{{ slaLabel(target) }}</span>
            <span v-if="targetStateLabel(target)" :class="['badge', targetStateClass(target)]">{{ targetStateLabel(target) }}</span>
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
    </template>
  </div>
</template>

<style scoped>
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
  margin-left: 1rem;
  padding: 0.35rem 0 0.15rem 0.75rem;
  border-left: 2px solid #cbd5e1;
  background: #e2e8f0;
  border-radius: 0 0 6px 0;
}

.check-row {
  padding: 0.5rem 0.5rem;
  margin-bottom: 2px;
  border-radius: 4px;
  cursor: pointer;
}
.check-row:hover { background: #cbd5e1; }

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
.badge-sla-healthy {
  background: #dcfce7;
  color: #166534;
  font-size: 0.7rem;
  font-weight: 600;
  padding: 0.1rem 0.5rem;
  border-radius: 10px;
}
.badge-sla-unhealthy {
  background: #fed7aa;
  color: #9a3412;
  font-size: 0.7rem;
  font-weight: 600;
  padding: 0.1rem 0.5rem;
  border-radius: 10px;
}

/* Filter bar */
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
  color: #475569;
  cursor: pointer;
  transition: all 0.15s;
}
.filter-btn:hover {
  background: #e2e8f0;
}
.filter-btn.active {
  background: #1e40af;
  color: #fff;
  border-color: #1e40af;
}
.filter-btn.has-problems:not(.active)::before {
  content: '';
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #f56565;
  margin-right: 0.35rem;
  vertical-align: middle;
}
.filter-count {
  font-weight: 400;
  opacity: 0.75;
}

.badge-cat-security {
  background: #ede9fe;
  color: #6d28d9;
}
.badge-cat-network {
  background: #dbeafe;
  color: #1d4ed8;
}
.badge-cat-physical {
  background: #fef3c7;
  color: #92400e;
}
.badge-cat-server {
  background: #fce7f3;
  color: #9d174d;
}
.badge-cat-other {
  background: #e5e7eb;
  color: #374151;
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

.expanded-checks .bar-track {
  height: 22px;
}

.check-detail {
  font-size: 0.8rem;
  color: #64748b;
  padding: 0.25rem 0 0 1.25rem;
}
</style>
