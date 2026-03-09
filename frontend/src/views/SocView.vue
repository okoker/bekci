<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import api from '../api'

const dashboardData = ref([])
const historyData = ref({})
const loading = ref(true)
const error = ref('')
const lastUpdated = ref(null)
const activeCategory = ref('All')
const categories = ['All', 'Network', 'Security', 'Physical Security', 'Key Services', 'Other']
let refreshTimer = null
let healthTimer = null

// --- System Health ---
const health = ref(null)
const showPopover = ref(false)

async function fetchHealth() {
  try {
    const { data } = await api.get('/system/health')
    health.value = data
  } catch {
    health.value = null
  }
}

function dotColor(metric) {
  if (!health.value) return 'dot-grey'
  if (metric === 'net') return health.value.net.status === 'ok' ? 'dot-green' : 'dot-red'
  if (metric === 'disk') {
    const d = health.value.disk
    if (!d.total_gb) return 'dot-grey'
    const pct = d.free_gb / d.total_gb
    return pct > 0.2 ? 'dot-green' : pct > 0.1 ? 'dot-yellow' : 'dot-red'
  }
  if (metric === 'cpu') {
    const c = health.value.cpu
    if (c.load1 < 0) return 'dot-grey'
    return c.load1 < c.num_cpu ? 'dot-green' : c.load1 < c.num_cpu * 2 ? 'dot-yellow' : 'dot-red'
  }
  return 'dot-grey'
}

const netLabel = computed(() => {
  if (!health.value) return 'Net: —'
  const n = health.value.net
  return n.status === 'ok' ? `Net: OK (${n.latency_ms}ms)` : 'Net: Unreachable'
})
const diskLabel = computed(() => {
  if (!health.value || !health.value.disk.total_gb) return 'Disk: —'
  const d = health.value.disk
  return `Disk: ${d.free_gb}/${d.total_gb} GB free`
})
const cpuLabel = computed(() => {
  if (!health.value) return 'Load: —'
  const c = health.value.cpu
  return c.load1 < 0 ? 'Load: —' : `Load: ${c.load1} (${c.num_cpu} cores)`
})

function togglePopover() { showPopover.value = !showPopover.value }
function closePopover(e) { if (!e.target.closest('.health-indicator')) showPopover.value = false }

const historyError = ref({}) // checkId -> true if history load failed
const empty90d = Array.from({ length: 90 }, () => ({ date: '', uptime_pct: -1, total_checks: 0 }))
const empty4h = Array.from({ length: 48 }, () => ({ status: 'none', response_ms: 0, checked_at: '' }))

async function loadDashboard() {
  try {
    const { data } = await api.get('/soc/status')
    // Pre-compute on plain objects BEFORE reactive assignment
    for (const t of data) {
      t._worstUptime = getWorstUptime(t)
      t._preferredCheck = getPreferredCheck(t)
    }
    dashboardData.value = data
    lastUpdated.value = new Date()
    error.value = ''

    // Batch-load history for all preferred checks
    const checkIds = []
    for (const t of data) {
      const pref = t._preferredCheck
      if (pref && !historyData.value[pref.id]) {
        checkIds.push(pref.id)
      }
    }
    if (checkIds.length > 0) {
      const results = await Promise.allSettled(
        checkIds.map(id => loadHistorySingle(id))
      )
      const updated = { ...historyData.value }
      const errors = { ...historyError.value }
      for (let i = 0; i < checkIds.length; i++) {
        if (results[i].status === 'fulfilled') {
          updated[checkIds[i]] = results[i].value
        } else {
          errors[checkIds[i]] = true
        }
      }
      historyData.value = updated
      historyError.value = errors
    }
  } catch (e) {
    if (e.response?.status === 401) {
      error.value = 'SOC view requires authentication. Enable public access in Settings or log in.'
    } else {
      error.value = 'Failed to load SOC data'
    }
  } finally {
    loading.value = false
  }
}

async function loadHistorySingle(checkId) {
  const [res90d, res4h] = await Promise.all([
    api.get(`/soc/history/${checkId}?range=90d`),
    api.get(`/soc/history/${checkId}?range=4h`),
  ])
  return {
    bar90d: pad90dBars(res90d.data),
    bar4h: pad4hBars(res4h.data),
  }
}

function getPreferredCheck(target) {
  return target.checks?.find(c => c.type === target.preferred_check_type) || target.checks?.[0]
}

function pad90dBars(data) {
  const map = {}
  for (const d of data) map[d.date] = d
  const bars = []
  const now = new Date()
  for (let i = 89; i >= 0; i--) {
    const dt = new Date(now)
    dt.setDate(dt.getDate() - i)
    const key = dt.toISOString().slice(0, 10)
    bars.push(map[key] || { date: key, uptime_pct: -1, total_checks: 0 })
  }
  return bars
}

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

function hasDownCheckTarget(target) {
  if (target.state === 'paused') return false
  if (target.state === 'unhealthy') return true
  if (target.state === 'healthy') return false
  return target.checks?.some(c => c.last_status === 'down')
}

function isTargetPaused(target) {
  return target.paused === true || target.state === 'paused'
}

const categoryStats = computed(() => {
  const stats = {}
  for (const cat of categories) {
    if (cat === 'All') {
      stats[cat] = {
        count: dashboardData.value.length,
        hasProblems: dashboardData.value.some(t => hasDownCheckTarget(t))
      }
    } else {
      const targets = dashboardData.value.filter(t => t.category === cat)
      stats[cat] = { count: targets.length, hasProblems: targets.some(t => hasDownCheckTarget(t)) }
    }
  }
  return stats
})

function getWorstUptime(target) {
  if (!target.checks || target.checks.length === 0) return 100
  return Math.min(...target.checks.map(c => c.uptime_90d_pct >= 0 ? c.uptime_90d_pct : 100))
}

function categoryClass(cat) {
  if (cat === 'Security') return 'soc-cat-security'
  if (cat === 'Network') return 'soc-cat-network'
  if (cat === 'Physical Security') return 'soc-cat-physical'
  if (cat === 'Key Services') return 'soc-cat-server'
  return 'soc-cat-other'
}

function slaLabel(target) {
  if (target.sla_status === 'healthy') return 'HEALTHY'
  if (target.sla_status === 'unhealthy') return 'UNHEALTHY'
  return ''
}

function slaClass(target) {
  if (target.sla_status === 'healthy') return 'soc-badge-sla-healthy'
  if (target.sla_status === 'unhealthy') return 'soc-badge-sla-unhealthy'
  return ''
}

const filteredAndSortedTargets = computed(() => {
  let list = dashboardData.value
  if (activeCategory.value !== 'All') {
    list = list.filter(t => t.category === activeCategory.value)
  }
  return [...list].sort((a, b) => {
    const aPaused = isTargetPaused(a)
    const bPaused = isTargetPaused(b)
    const aDown = hasDownCheckTarget(a)
    const bDown = hasDownCheckTarget(b)
    if (aDown && !bDown) return -1
    if (!aDown && bDown) return 1
    if (aDown && bDown) return a._worstUptime - b._worstUptime
    const aUnhealthy = a.sla_status === 'unhealthy'
    const bUnhealthy = b.sla_status === 'unhealthy'
    if (aUnhealthy && !bUnhealthy) return -1
    if (!aUnhealthy && bUnhealthy) return 1
    if (aPaused && !bPaused) return 1
    if (!aPaused && bPaused) return -1
    const diff = a._worstUptime - b._worstUptime
    return diff !== 0 ? diff : a.name.localeCompare(b.name)
  })
})

function uptimeColor(pct) {
  if (pct < 0) return '#374151'
  if (pct >= 99.9) return '#48bb78'
  if (pct >= 95) return '#ecc94b'
  if (pct >= 80) return '#ed8936'
  return '#f56565'
}

function statusColor(status) {
  if (status === 'none') return '#374151'
  return status === 'up' ? '#48bb78' : '#f56565'
}

function formatTooltip90d(day) {
  if (!day) return ''
  const d = new Date(day.date)
  return `${d.toLocaleDateString('en-GB')}: ${day.uptime_pct.toFixed(1)}%`
}

function formatTooltip4h(r) {
  if (!r) return ''
  const d = new Date(r.checked_at)
  return `${d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })}: ${r.status} (${r.response_ms}ms)`
}

onMounted(() => {
  loadDashboard()
  refreshTimer = setInterval(loadDashboard, 15000)
  fetchHealth()
  healthTimer = setInterval(fetchHealth, 30000)
  document.addEventListener('click', closePopover)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
  if (healthTimer) clearInterval(healthTimer)
  document.removeEventListener('click', closePopover)
})
</script>

<template>
  <div class="soc-page">
    <header class="soc-header">
      <a href="/" class="soc-brand"><img src="/bekci-icon.png" alt="Bekci" class="soc-icon" />SOC</a>
      <div v-if="!loading && dashboardData.length > 0" class="soc-filter-bar">
        <button v-for="cat in categories" :key="cat"
          :class="['soc-filter-btn', { active: activeCategory === cat, 'has-problems': categoryStats[cat].hasProblems }]"
          @click="activeCategory = cat">
          {{ cat }} <span class="soc-filter-count">({{ categoryStats[cat].count }})</span>
        </button>
      </div>
      <div v-if="health" class="health-indicator" @click.stop="togglePopover">
        <div class="health-dots">
          <span class="health-dot" :class="dotColor('net')" title="Network"></span>
          <span class="health-dot" :class="dotColor('disk')" title="Disk"></span>
          <span class="health-dot" :class="dotColor('cpu')" title="CPU"></span>
        </div>
        <div v-if="showPopover" class="health-popover soc-health-popover">
          <div class="health-row" :class="dotColor('net')">{{ netLabel }}</div>
          <div class="health-row" :class="dotColor('disk')">{{ diskLabel }}</div>
          <div class="health-row" :class="dotColor('cpu')">{{ cpuLabel }}</div>
        </div>
      </div>
      <span v-if="lastUpdated" class="soc-updated">
        {{ lastUpdated.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit' }) }}
      </span>
    </header>

    <div v-if="error" class="soc-error">{{ error }}</div>

    <div v-if="loading" class="soc-loading">Loading...</div>

    <div v-else class="soc-grid">
      <div v-for="target in filteredAndSortedTargets" :key="target.id" class="soc-card" :class="{ 'soc-card-down': hasDownCheckTarget(target), 'soc-card-paused': isTargetPaused(target) }">
        <div class="soc-hover-label">
          <span class="soc-hover-name">{{ target.name }}</span>
          <span class="soc-hover-host">{{ target.host }}</span>
        </div>
        <div class="soc-card-header">
          <span class="soc-target-name">{{ target.name }}</span>
          <span class="soc-host">{{ target.host }}</span>
          <span v-if="!isTargetPaused(target) && target._preferredCheck?.uptime_90d_pct >= 0" class="soc-uptime"
            :style="{ color: uptimeColor(target._preferredCheck?.uptime_90d_pct) }">
            {{ target._preferredCheck?.uptime_90d_pct.toFixed(1) }}%
          </span>
          <div class="soc-header-badges">
            <span :class="['soc-cat-badge', categoryClass(target.category)]">{{ target.category }}</span>
            <span v-if="isTargetPaused(target)" class="soc-status-badge soc-badge-paused">PAUSED</span>
            <template v-else>
              <span v-if="slaLabel(target)" :class="['soc-status-badge', slaClass(target)]">{{ slaLabel(target) }}</span>
              <span :class="['soc-status-badge', hasDownCheckTarget(target) ? 'soc-badge-down' : 'soc-badge-up']">
                {{ hasDownCheckTarget(target) ? 'DOWN' : 'UP' }}
              </span>
            </template>
          </div>
        </div>
        <!-- Compact bars -->
        <div v-if="target._preferredCheck" class="soc-bars">
          <template v-if="historyError[target._preferredCheck?.id]">
            <div class="soc-history-error" title="History data unavailable">
              <span class="soc-history-error-icon">!</span>
            </div>
          </template>
          <template v-else>
            <div class="soc-bar-track">
              <div v-for="(day, i) in (historyData[target._preferredCheck?.id]?.bar90d || empty90d)" :key="'90d-' + i"
                class="soc-bar-tick"
                :style="{ background: uptimeColor(day.uptime_pct) }"
                :title="formatTooltip90d(day)">
              </div>
            </div>
            <div class="soc-bar-track soc-bar-4h">
              <div v-for="(r, i) in (historyData[target._preferredCheck?.id]?.bar4h || empty4h)" :key="'4h-' + i"
                class="soc-bar-tick"
                :style="{ background: statusColor(r.status) }"
                :title="formatTooltip4h(r)">
              </div>
            </div>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.soc-page {
  min-height: 100vh;
  background: #0f172a;
  color: #e2e8f0;
  padding: 1rem 1.5rem;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}

.soc-header {
  display: flex;
  align-items: center;
  gap: 1rem;
  margin-bottom: 1rem;
  padding-bottom: 0.75rem;
  border-bottom: 1px solid #1e293b;
}
.soc-brand {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 1.25rem;
  font-weight: 700;
  color: #fff;
  text-decoration: none;
  white-space: nowrap;
  flex-shrink: 0;
}
.soc-brand:hover {
  opacity: 0.85;
}
.soc-icon {
  width: 28px;
  height: 28px;
}
.soc-updated {
  color: #64748b;
  font-size: 0.8rem;
  white-space: nowrap;
  flex-shrink: 0;
  margin-left: auto;
}

.soc-error {
  background: rgba(239, 68, 68, 0.15);
  color: #fca5a5;
  padding: 0.75rem 1rem;
  border-radius: 8px;
  margin-bottom: 1rem;
  font-size: 0.9rem;
}

.soc-loading {
  text-align: center;
  color: #64748b;
  padding: 3rem;
  font-size: 1.1rem;
}

.soc-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 0.75rem;
}

@media (max-width: 1200px) {
  .soc-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 768px) {
  .soc-grid {
    grid-template-columns: 1fr;
  }
}

.soc-card {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 8px;
  padding: 0.6rem 0.8rem;
  min-width: 0;
  position: relative;
}

/* Hover overlay: full hostname in x-large font */
.soc-hover-label {
  display: none;
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  z-index: 10;
  background: rgba(15, 23, 42, 0.95);
  border: 1px solid #475569;
  border-radius: 8px;
  padding: 0.5rem 0.75rem;
  pointer-events: none;
}
.soc-card:hover .soc-hover-label {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.soc-hover-name {
  font-size: 1.4rem;
  font-weight: 700;
  color: #fff;
  line-height: 1.2;
  word-break: break-word;
}
.soc-hover-host {
  font-size: 1.1rem;
  font-weight: 500;
  color: #94a3b8;
  word-break: break-all;
}
.soc-card-down {
  border-color: #f56565;
}
.soc-card-paused {
  border-color: #6366f1;
  opacity: 0.75;
}
.soc-badge-paused {
  background: rgba(99, 102, 241, 0.2);
  color: #818cf8;
}

.soc-card-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.4rem;
  overflow: hidden;
}
.soc-target-name {
  font-weight: 600;
  font-size: 0.9rem;
  color: #fff;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}
.soc-host {
  font-size: 0.75rem;
  color: #64748b;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}
.soc-uptime {
  font-weight: 700;
  font-size: 0.8rem;
  white-space: nowrap;
  margin-left: auto;
}
.soc-status-badge {
  font-size: 0.65rem;
  font-weight: 700;
  padding: 0.1rem 0.4rem;
  border-radius: 10px;
  text-transform: uppercase;
}
.soc-badge-up {
  background: rgba(72, 187, 120, 0.2);
  color: #48bb78;
}
.soc-badge-down {
  background: rgba(245, 101, 101, 0.2);
  color: #f56565;
}
.soc-badge-sla-healthy {
  background: rgba(72, 187, 120, 0.2);
  color: #48bb78;
}
.soc-badge-sla-unhealthy {
  background: rgba(251, 146, 60, 0.2);
  color: #fb923c;
}

.soc-bars {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.soc-bar-track {
  display: flex;
  gap: 1px;
  height: 16px;
  align-items: stretch;
}

.soc-bar-tick {
  flex: 1;
  min-width: 0;
  border-radius: 1px;
  transition: opacity 0.15s;
}
.soc-bar-tick:hover {
  opacity: 0.65;
}

/* Filter bar (inline in header) */
.soc-filter-bar {
  display: flex;
  gap: 0.35rem;
  flex-wrap: wrap;
  justify-content: center;
  flex: 1;
  min-width: 0;
}
.soc-filter-btn {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 20px;
  padding: 0.25rem 0.65rem;
  font-size: 0.75rem;
  font-weight: 500;
  color: #94a3b8;
  cursor: pointer;
  transition: all 0.15s;
}
.soc-filter-btn:hover {
  background: #334155;
  color: #e2e8f0;
}
.soc-filter-btn.active {
  background: #3b82f6;
  color: #fff;
  border-color: #3b82f6;
}
.soc-filter-btn.has-problems:not(.active)::before {
  content: '';
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #f56565;
  margin-right: 0.35rem;
  vertical-align: middle;
}
.soc-filter-count {
  font-weight: 400;
  opacity: 0.7;
}

/* Category badges (dark theme) */
.soc-header-badges {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}
.soc-cat-badge {
  font-size: 0.6rem;
  font-weight: 600;
  padding: 0.1rem 0.4rem;
  border-radius: 10px;
  text-transform: uppercase;
  letter-spacing: 0.03em;
}
.soc-cat-security {
  background: rgba(139, 92, 246, 0.2);
  color: #a78bfa;
}
.soc-cat-network {
  background: rgba(59, 130, 246, 0.2);
  color: #93c5fd;
}
.soc-cat-physical {
  background: rgba(245, 158, 11, 0.2);
  color: #fbbf24;
}
.soc-cat-server {
  background: rgba(219, 39, 119, 0.2);
  color: #f9a8d4;
}
.soc-cat-other {
  background: rgba(148, 163, 184, 0.15);
  color: #94a3b8;
}

/* Dark theme overrides for health popover */
.soc-health-popover {
  background: #1e293b;
  color: #e2e8f0;
  border-color: #334155;
}

.soc-history-error {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 35px;
}
.soc-history-error-icon {
  display: inline-block;
  width: 16px;
  height: 16px;
  line-height: 16px;
  text-align: center;
  font-size: 0.7rem;
  font-weight: 700;
  color: #f59e0b;
  background: rgba(245, 158, 11, 0.15);
  border-radius: 50%;
}
</style>
