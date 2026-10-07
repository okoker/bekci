<script setup>
// Settings → System Health. Layout and icons follow ScanTracker's System Health
// panel (Heroicons outline paths copied from scantracker SettingsView.vue).
import { ref, onMounted } from 'vue'
import api from '../api'

const info = ref(null)
const loading = ref(false)
const error = ref('')
const fetchedAt = ref(null)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const { data } = await api.get('/system/info')
    info.value = data
    fetchedAt.value = new Date()
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to load system information'
  } finally {
    loading.value = false
  }
}

onMounted(load)

// ── Formatting ──
function fmtDateTime(d) {
  if (!d) return '—'
  const dt = new Date(d)
  if (isNaN(dt.getTime())) return '—'
  return dt.toLocaleDateString('en-GB') + ' ' + dt.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })
}
function fmtTime(d) {
  if (!d) return ''
  return new Date(d).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })
}
function fmtClock(d) {
  if (!d) return ''
  return new Date(d).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
function fmtBytes(b) {
  if (b == null || b < 0) return '—'
  const mb = b / (1024 * 1024)
  if (mb >= 1024) return (mb / 1024).toFixed(2) + ' GB'
  return mb.toFixed(1) + ' MB'
}
function fmtDuration(sec) {
  if (sec == null || sec < 0) return '—'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return d > 0 ? `${d}d ${h}h ${m}m` : `${h}h ${m}m`
}
function barClass(pct) {
  return pct > 90 ? 'bar-red' : pct > 75 ? 'bar-yellow' : 'bar-green'
}

const updatesLabel = {
  up_to_date: 'Up to date',
  updates: 'Updates ready',
  security: 'Security updates ready',
  unavailable: 'Not available',
}
const updatesDot = { up_to_date: 'dot-green', updates: 'dot-yellow', security: 'dot-red', unavailable: 'dot-grey' }
function updatesSub(u) {
  if (u.status === 'unavailable') return 'apt not available on this host'
  const checked = u.checked_at ? `checked ${fmtTime(u.checked_at)}` : ''
  if (u.status === 'up_to_date') return checked
  const sec = u.security > 0 ? ` (${u.security} security)` : ''
  return `${u.total} pending${sec}` + (checked ? ` · ${checked}` : '')
}

const schedulerLabel = { ok: 'Running', starting: 'Starting', stale: 'Stale', unavailable: 'Unavailable' }
const schedulerDot = { ok: 'dot-green', starting: 'dot-yellow', stale: 'dot-red', unavailable: 'dot-grey' }
function schedulerSub(s) {
  if (s.status === 'unavailable') return ''
  const tick = s.last_tick ? ` · last tick ${s.stale_seconds}s ago` : ''
  return `${s.active_checks} active checks${tick}`
}

const scheduleLabel = { off: 'auto-backup off', weekly: 'weekly', '10days': 'every 10 days', monthly: 'monthly' }
function backupSub(b) {
  const sched = b.schedule === 'off' || !scheduleLabel[b.schedule]
    ? 'auto-backup off'
    : `auto ${scheduleLabel[b.schedule]} at ${b.time}`
  if (!b.last_at) return sched
  return `${fmtBytes(b.last_bytes)}${b.encrypted ? ' · encrypted' : ''} · ${sched}`
}

function databaseSub(db) {
  const wal = db.wal_bytes > 0 ? ` (+${fmtBytes(db.wal_bytes)} WAL)` : ''
  return `${fmtBytes(db.size_bytes)}${wal} · schema v${db.schema_version}`
}

function buildSub(b) {
  if (!b.commit) return 'build info not stamped'
  const date = b.time ? ` · ${new Date(b.time).toLocaleDateString('en-GB')}` : ''
  return `commit ${b.commit}${b.modified ? '+' : ''}${date}`
}
</script>

<template>
  <div class="card sh-panel">
    <div class="sh-header">
      <h3>System Health</h3>
      <div class="sh-actions">
        <span v-if="fetchedAt" class="sh-updated">Updated: {{ fmtClock(fetchedAt) }}</span>
        <button class="btn btn-sm sh-refresh" :disabled="loading" @click="load">
          <svg class="sh-refresh-icon" :class="{ spinning: loading }" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          Refresh
        </button>
      </div>
    </div>

    <div v-if="loading && !info" class="sh-loading">
      <div class="sh-spinner"></div>
      <span>Loading...</span>
    </div>
    <div v-else-if="error && !info" class="sh-body">
      <div class="error-msg">{{ error }}</div>
    </div>

    <div v-else-if="info" class="sh-body">
      <div v-if="error" class="error-msg">{{ error }}</div>

      <!-- Row 1: resource cards -->
      <div class="sh-grid">
        <!-- Disk Space -->
        <div class="sh-card">
          <div class="sh-card-head">
            <svg class="sh-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />
            </svg>
            <span class="sh-label">Disk Space</span>
          </div>
          <template v-if="info.disk.total_gb > 0">
            <div class="sh-value">{{ info.disk.free_gb }} GB</div>
            <div class="sh-sub">{{ info.disk.used_pct }}% used of {{ info.disk.total_gb }} GB</div>
            <div class="sh-bar"><div class="sh-bar-fill" :class="barClass(info.disk.used_pct)" :style="{ width: info.disk.used_pct + '%' }"></div></div>
          </template>
          <div v-else class="sh-value">—</div>
        </div>

        <!-- RAM Usage -->
        <div class="sh-card">
          <div class="sh-card-head">
            <svg class="sh-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z" />
            </svg>
            <span class="sh-label">RAM Usage</span>
          </div>
          <template v-if="info.ram.total_gb > 0">
            <div class="sh-value">{{ info.ram.used_gb }} GB</div>
            <div class="sh-sub">{{ info.ram.used_pct }}% of {{ info.ram.total_gb }} GB</div>
            <div class="sh-bar"><div class="sh-bar-fill" :class="barClass(info.ram.used_pct)" :style="{ width: info.ram.used_pct + '%' }"></div></div>
          </template>
          <div v-else class="sh-value">—</div>
        </div>

        <!-- CPU Load -->
        <div class="sh-card">
          <div class="sh-card-head">
            <svg class="sh-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
            </svg>
            <span class="sh-label">CPU Load</span>
          </div>
          <template v-if="info.cpu.load1 >= 0">
            <div class="sh-value">{{ info.cpu.load1 }}</div>
            <div class="sh-sub">1m: {{ info.cpu.load1 }} | 5m: {{ info.cpu.load5 }} | 15m: {{ info.cpu.load15 }} · {{ info.cpu.num_cpu }} cores</div>
          </template>
          <div v-else class="sh-value">—</div>
        </div>

        <!-- Uptime -->
        <div class="sh-card">
          <div class="sh-card-head">
            <svg class="sh-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <span class="sh-label">Uptime</span>
          </div>
          <div class="sh-value">{{ fmtDuration(info.uptime.system_sec) }}</div>
          <div class="sh-sub">System running · bekci up {{ fmtDuration(info.uptime.process_sec) }}</div>
        </div>
      </div>

      <!-- Row 2: server details -->
      <div class="sh-grid sh-grid-gap">
        <div class="sh-card">
          <div class="sh-label sh-label-block">Server IP</div>
          <div class="sh-mono">{{ info.server_ip || '—' }}</div>
        </div>
        <div class="sh-card">
          <div class="sh-label sh-label-block">Version</div>
          <div class="sh-mono">v{{ info.version }}</div>
          <div class="sh-sub">{{ buildSub(info.build) }}</div>
        </div>
        <div class="sh-card">
          <div class="sh-label sh-label-block">Go Version</div>
          <div class="sh-mono">{{ info.go_version }}</div>
        </div>
        <div class="sh-card">
          <div class="sh-label sh-label-block">OS Updates</div>
          <div class="sh-status">
            <span class="sh-dot" :class="updatesDot[info.updates.status] || 'dot-grey'"></span>
            <span class="sh-status-text">{{ updatesLabel[info.updates.status] || info.updates.status }}</span>
          </div>
          <div class="sh-sub">{{ updatesSub(info.updates) }}</div>
        </div>
      </div>

      <!-- Row 3: backup + log -->
      <div class="sh-grid sh-grid-gap">
        <div class="sh-card">
          <div class="sh-label sh-label-block">Last Backup</div>
          <div class="sh-text">{{ info.backup.last_at ? fmtDateTime(info.backup.last_at) : 'Never' }}</div>
          <div class="sh-sub">{{ backupSub(info.backup) }}</div>
        </div>
        <div class="sh-card">
          <div class="sh-label sh-label-block">Log File</div>
          <div class="sh-mono">{{ fmtBytes(info.log.size_bytes) }}</div>
          <div class="sh-sub sh-path" :title="info.log.path">{{ info.log.path || '—' }}</div>
        </div>
      </div>

      <!-- Row 4: core services -->
      <div class="sh-grid sh-grid-gap">
        <div class="sh-card">
          <div class="sh-label sh-label-block">Scheduler</div>
          <div class="sh-status">
            <span class="sh-dot" :class="schedulerDot[info.scheduler.status] || 'dot-grey'"></span>
            <span class="sh-status-text">{{ schedulerLabel[info.scheduler.status] || info.scheduler.status }}</span>
          </div>
          <div class="sh-sub">{{ schedulerSub(info.scheduler) }}</div>
        </div>
        <div class="sh-card">
          <div class="sh-label sh-label-block">Database</div>
          <div class="sh-status">
            <span class="sh-dot" :class="info.database.status === 'ok' ? 'dot-green' : 'dot-red'"></span>
            <span class="sh-status-text">{{ info.database.status === 'ok' ? 'OK' : 'Error' }}</span>
          </div>
          <div class="sh-sub">{{ databaseSub(info.database) }}</div>
        </div>
        <div class="sh-card">
          <div class="sh-label sh-label-block">Email</div>
          <div class="sh-status">
            <span class="sh-dot" :class="info.email.configured ? 'dot-green' : 'dot-yellow'"></span>
            <span class="sh-status-text">{{ info.email.configured ? 'Configured' : 'Not configured' }}</span>
          </div>
          <div class="sh-sub">via {{ info.email.provider }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* ScanTracker System Health look (Tailwind gray scale) mapped onto bekci's slate palette. */
.sh-panel {
  padding: 0;
  overflow: hidden;
}
.sh-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1rem 1.5rem;
  border-bottom: 1px solid #e2e8f0;
}
.sh-header h3 {
  margin: 0;
  font-size: 1.125rem;
  font-weight: 500;
  color: #0f172a;
}
.sh-actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}
.sh-updated {
  font-size: 0.8rem;
  color: #64748b;
}
.sh-refresh {
  gap: 0.375rem;
}
.sh-refresh-icon {
  width: 0.9rem;
  height: 0.9rem;
}
.sh-refresh-icon.spinning,
.sh-spinner {
  animation: sh-spin 1s linear infinite;
}
@keyframes sh-spin {
  to { transform: rotate(360deg); }
}
.sh-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 1.5rem;
  font-size: 0.875rem;
  color: #64748b;
}
.sh-spinner {
  width: 1.5rem;
  height: 1.5rem;
  border-radius: 50%;
  border: 2px solid #ea580c;
  border-top-color: transparent;
}
.sh-body {
  padding: 1.5rem;
}
.sh-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 1rem;
}
.sh-grid-gap {
  margin-top: 1rem;
}
@media (max-width: 768px) {
  .sh-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
.sh-card {
  background: #f8fafc;
  border-radius: 8px;
  padding: 1rem;
  min-width: 0;
}
.sh-card-head {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
}
.sh-icon {
  width: 1.25rem;
  height: 1.25rem;
  color: #64748b;
  flex-shrink: 0;
}
.sh-label {
  font-size: 0.875rem;
  font-weight: 500;
  color: #334155;
}
.sh-label-block {
  display: block;
  margin-bottom: 0.25rem;
}
.sh-value {
  font-size: 1.5rem;
  line-height: 2rem;
  font-weight: 700;
  color: #0f172a;
}
.sh-sub {
  font-size: 0.75rem;
  line-height: 1rem;
  color: #64748b;
  margin-top: 0.125rem;
}
.sh-path {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sh-bar {
  margin-top: 0.5rem;
  height: 0.5rem;
  background: #e2e8f0;
  border-radius: 9999px;
  overflow: hidden;
}
.sh-bar-fill {
  height: 100%;
  border-radius: 9999px;
}
.bar-green { background: #22c55e; }
.bar-yellow { background: #eab308; }
.bar-red { background: #ef4444; }
.sh-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 1.125rem;
  line-height: 1.75rem;
  color: #0f172a;
}
.sh-text {
  font-size: 1.125rem;
  line-height: 1.75rem;
  color: #0f172a;
}
.sh-status {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.sh-dot {
  width: 0.625rem;
  height: 0.625rem;
  border-radius: 50%;
  flex-shrink: 0;
}
.sh-dot.dot-green { background: #22c55e; }
.sh-dot.dot-yellow { background: #eab308; }
.sh-dot.dot-red { background: #ef4444; }
.sh-dot.dot-grey { background: #94a3b8; }
.sh-status-text {
  font-size: 1.125rem;
  line-height: 1.75rem;
  color: #0f172a;
}
</style>
