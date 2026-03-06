<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { Line } from 'vue-chartjs'
import {
  Chart as ChartJS,
  LineElement,
  PointElement,
  LinearScale,
  CategoryScale,
  Tooltip,
  Filler,
} from 'chart.js'
import annotationPlugin from 'chartjs-plugin-annotation'
import api from '../api'

ChartJS.register(LineElement, PointElement, LinearScale, CategoryScale, Tooltip, Filler, annotationPlugin)

const loading = ref(true)
const error = ref('')
const categories = ref([])
const pauseStats = ref({ count: 0, affected_hosts: 0 })
const modalCat = ref(null)

const palette = [
  '#3b82f6', '#ef4444', '#10b981', '#f59e0b', '#8b5cf6',
  '#ec4899', '#06b6d4', '#84cc16', '#f97316', '#6366f1',
  '#14b8a6', '#e11d48', '#0ea5e9', '#a855f7', '#d946ef',
  '#22c55e', '#eab308', '#64748b', '#dc2626', '#2563eb',
]

async function loadHistory() {
  try {
    const { data } = await api.get('/sla/history')
    categories.value = data.categories || []
    pauseStats.value = data.pause_stats || { count: 0, affected_hosts: 0 }
  } catch {
    error.value = 'Failed to load SLA history'
  } finally {
    loading.value = false
  }
}

// Compute 90d average uptime for a target from its daily data
function target90dAvg(target) {
  const vals = target.daily_uptime.map(d => d.uptime_pct).filter(v => v != null)
  if (vals.length === 0) return null
  return vals.reduce((a, b) => a + b, 0) / vals.length
}

const insights = computed(() => {
  const cats = categories.value
  let totalHosts = 0
  let failingHosts = 0

  const groupAvgs = []
  for (const cat of cats) {
    const targets = cat.targets || []
    const avgs = targets.map(t => target90dAvg(t)).filter(v => v !== null)
    totalHosts += targets.length

    const threshold = cat.sla_threshold || 0
    if (threshold > 0) {
      failingHosts += avgs.filter(a => a < threshold).length
    }

    const avg = avgs.length > 0 ? avgs.reduce((a, b) => a + b, 0) / avgs.length : null
    groupAvgs.push({ name: cat.name, avg })
  }

  // "All" average
  const allAvgs = cats.flatMap(c => (c.targets || []).map(t => target90dAvg(t))).filter(v => v !== null)
  const allAvg = allAvgs.length > 0 ? allAvgs.reduce((a, b) => a + b, 0) / allAvgs.length : null

  return { totalHosts, failingHosts, allAvg, groupAvgs }
})

function padDays(dailyUptime) {
  const map = {}
  for (const d of dailyUptime) {
    map[d.date] = d.uptime_pct
  }
  const days = []
  const now = new Date()
  for (let i = 89; i >= 0; i--) {
    const dt = new Date(now)
    dt.setDate(dt.getDate() - i)
    const key = dt.toISOString().slice(0, 10)
    days.push({ date: key, uptime_pct: map[key] !== undefined ? map[key] : null })
  }
  return days
}

function formatLabel(dateStr) {
  const d = new Date(dateStr + 'T00:00:00')
  return d.toLocaleDateString('en-GB', { day: '2-digit', month: 'short' })
}

function buildChartData(cat) {
  if (!cat.targets || cat.targets.length === 0) return null

  const firstPadded = padDays(cat.targets[0].daily_uptime)
  const labels = firstPadded.map(d => formatLabel(d.date))

  const datasets = cat.targets.map((t, idx) => {
    const padded = padDays(t.daily_uptime)
    return {
      label: t.name,
      data: padded.map(d => d.uptime_pct),
      borderColor: palette[idx % palette.length],
      backgroundColor: 'transparent',
      borderWidth: 1.5,
      pointRadius: 2,
      pointHoverRadius: 5,
      spanGaps: true,
      tension: 0.2,
    }
  })

  return { labels, datasets }
}

function computeYMin(cat) {
  const threshold = cat.sla_threshold || 0
  let lowestPct = 100
  for (const t of cat.targets) {
    for (const d of t.daily_uptime) {
      if (d.uptime_pct < lowestPct) lowestPct = d.uptime_pct
    }
  }
  const dataFloor = lowestPct < 100 ? Math.floor(lowestPct - 2) : 95
  const thresholdFloor = threshold > 0 ? threshold - 2 : 95
  return Math.min(95, dataFloor, thresholdFloor)
}

function buildChartOptions(cat, isModal) {
  const threshold = cat.sla_threshold || 0
  const yMin = computeYMin(cat)

  return {
    responsive: true,
    maintainAspectRatio: false,
    interaction: {
      mode: 'index',
      intersect: false,
    },
    plugins: {
      legend: isModal ? {
        display: true,
        position: 'top',
        labels: { usePointStyle: true, pointStyle: 'circle', boxWidth: 8, padding: 16, font: { size: 12 } },
      } : { display: false },
      tooltip: {
        callbacks: {
          title(items) {
            return items[0]?.label || ''
          },
          label(item) {
            const val = item.raw !== null ? item.raw.toFixed(2) + '%' : 'N/A'
            return `${item.dataset.label}: ${val}`
          },
        },
      },
      annotation: threshold > 0 ? {
        annotations: {
          slaLine: {
            type: 'line',
            yMin: threshold,
            yMax: threshold,
            borderColor: '#94a3b8',
            borderWidth: 1.5,
            borderDash: [6, 4],
            label: {
              display: true,
              content: `SLA ${threshold}%`,
              position: 'start',
              backgroundColor: 'rgba(148,163,184,0.8)',
              color: '#fff',
              font: { size: isModal ? 12 : 10 },
              padding: 3,
            },
          },
        },
      } : {},
    },
    scales: {
      x: {
        ticks: {
          maxTicksLimit: isModal ? 15 : 8,
          font: { size: isModal ? 11 : 10 },
          color: '#94a3b8',
        },
        grid: { display: false },
      },
      y: {
        min: yMin,
        max: 100.5,
        ticks: {
          callback: v => v + '%',
          font: { size: isModal ? 11 : 10 },
          color: '#94a3b8',
        },
        grid: { color: '#f1f5f9' },
      },
    },
    onHover(event, elements, chart) {
      if (elements.length > 0) {
        const idx = elements[0].datasetIndex
        chart.data.datasets.forEach((ds, i) => {
          ds.borderWidth = i === idx ? (isModal ? 3 : 2.5) : 1
          ds.borderColor = i === idx
            ? palette[i % palette.length]
            : palette[i % palette.length] + '40'
        })
      } else {
        chart.data.datasets.forEach((ds, i) => {
          ds.borderWidth = 1.5
          ds.borderColor = palette[i % palette.length]
        })
      }
      chart.update('none')
    },
  }
}

function openModal(cat) {
  if (cat.targets.length === 0) return
  modalCat.value = cat
}

function closeModal() {
  modalCat.value = null
}

function onKeydown(e) {
  if (e.key === 'Escape' && modalCat.value) closeModal()
}

onMounted(() => {
  loadHistory()
  document.addEventListener('keydown', onKeydown)
})

onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div class="sla-page">
    <div class="page-header">
      <h1>SLA Compliance</h1>
      <span class="subtitle">90-day daily uptime trend per category</span>
    </div>

    <div v-if="loading" class="loading-msg">Loading SLA data...</div>
    <div v-else-if="error" class="error-msg">{{ error }}</div>

    <template v-else>
    <!-- Insights card -->
    <div class="insights-card">
      <div class="insights-row">
        <!-- Left: key stats -->
        <div class="insights-stats">
          <div class="insight-stat">
            <span class="insight-stat-value">{{ insights.totalHosts }}</span>
            <span class="insight-stat-label">Hosts Monitored</span>
          </div>
          <div class="insight-stat" :class="{ 'insight-warn': insights.failingHosts > 0 }">
            <span class="insight-stat-value">{{ insights.failingHosts }}</span>
            <span class="insight-stat-label">Not Meeting SLA</span>
          </div>
        </div>

        <div class="insight-divider"></div>

        <!-- Center: SLA averages as mini cards -->
        <div class="insights-averages">
          <span class="insights-averages-title">90-Day Averages</span>
          <div class="insights-avg-grid">
            <div class="avg-card avg-card-all">
              <span class="avg-card-label">All</span>
              <span class="avg-card-value">{{ insights.allAvg !== null ? insights.allAvg.toFixed(1) + '%' : '—' }}</span>
            </div>
            <template v-for="g in insights.groupAvgs" :key="g.name">
              <div class="avg-card">
                <span class="avg-card-label">{{ g.name }}</span>
                <span class="avg-card-value">{{ g.avg !== null ? g.avg.toFixed(1) + '%' : '—' }}</span>
              </div>
            </template>
          </div>
        </div>

        <div class="insight-divider"></div>

        <!-- Right: planned work -->
        <div class="insights-pause">
          <div class="insight-stat">
            <span class="insight-stat-value">{{ pauseStats.count }}</span>
            <span class="insight-stat-label">Planned Work <span class="insight-sub">({{ new Date().toLocaleDateString('en-GB', { month: 'short' }) }})</span></span>
            <span v-if="pauseStats.affected_hosts > 0" class="insight-sub">{{ pauseStats.affected_hosts }} host{{ pauseStats.affected_hosts !== 1 ? 's' : '' }} affected</span>
          </div>
        </div>
      </div>
    </div>

    <div class="sla-grid">
      <div
        v-for="cat in categories"
        :key="cat.name"
        class="sla-card"
        :class="{ clickable: cat.targets.length > 0 }"
        @click="openModal(cat)"
      >
        <div class="card-header">
          <span class="cat-name">{{ cat.name }}</span>
          <span v-if="cat.sla_threshold > 0" class="threshold-label">
            Target: {{ cat.sla_threshold }}%
          </span>
        </div>
        <div v-if="cat.targets.length === 0" class="empty-cat">
          No targets in this category
        </div>
        <div v-else class="chart-wrap">
          <Line :data="buildChartData(cat)" :options="buildChartOptions(cat, false)" />
        </div>
      </div>
    </div>

    </template>

    <!-- Modal -->
    <Teleport to="body">
      <div v-if="modalCat" class="modal-backdrop" @click.self="closeModal">
        <div class="modal-content">
          <div class="modal-header">
            <div class="modal-title-row">
              <span class="modal-cat-name">{{ modalCat.name }}</span>
              <span v-if="modalCat.sla_threshold > 0" class="threshold-label">
                Target: {{ modalCat.sla_threshold }}%
              </span>
            </div>
            <button class="modal-close" @click="closeModal">&times;</button>
          </div>
          <div class="modal-chart-wrap">
            <Line
              :key="modalCat.name"
              :data="buildChartData(modalCat)"
              :options="buildChartOptions(modalCat, true)"
            />
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.sla-page {
  padding: 24px 32px;
  max-width: 1400px;
  margin: 0 auto;
}

.page-header {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 24px;
}

.page-header h1 {
  font-size: 1.5rem;
  font-weight: 700;
  color: #1e293b;
  margin: 0;
}

.subtitle {
  font-size: 0.85rem;
  color: #94a3b8;
}

.loading-msg, .error-msg {
  text-align: center;
  padding: 48px;
  color: #64748b;
}

.error-msg { color: #ef4444; }

.sla-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 20px;
}

@media (max-width: 900px) {
  .sla-grid { grid-template-columns: 1fr; }
}

.sla-card {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 16px;
  transition: box-shadow 0.15s, border-color 0.15s;
}

.sla-card.clickable {
  cursor: pointer;
}

.sla-card.clickable:hover {
  border-color: #cbd5e1;
  box-shadow: 0 2px 8px rgba(0,0,0,0.06);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.cat-name {
  font-weight: 600;
  font-size: 0.95rem;
  color: #334155;
}

.threshold-label {
  font-size: 0.75rem;
  color: #94a3b8;
  background: #f1f5f9;
  padding: 2px 8px;
  border-radius: 10px;
}

/* Insights card */
.insights-card {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 16px 24px;
  margin-bottom: 20px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}
.insights-row {
  display: flex;
  align-items: center;
  gap: 24px;
  flex-wrap: wrap;
}

/* Left stats section */
.insights-stats {
  display: flex;
  gap: 24px;
  flex-shrink: 0;
}
.insight-stat {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
}
.insight-stat-value {
  font-size: 1.5rem;
  font-weight: 700;
  color: #1e293b;
  line-height: 1.2;
}
.insight-stat-label {
  font-size: 0.7rem;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.03em;
  font-weight: 500;
  white-space: nowrap;
}
.insight-warn .insight-stat-value {
  color: #dc2626;
}
.insight-sub {
  font-size: 0.7rem;
  color: #94a3b8;
  text-transform: none;
  letter-spacing: normal;
  font-weight: 400;
}

/* Divider */
.insight-divider {
  width: 1px;
  align-self: stretch;
  min-height: 48px;
  background: #e2e8f0;
  flex-shrink: 0;
}

/* Center: SLA averages */
.insights-averages {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.insights-averages-title {
  font-size: 0.65rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: #94a3b8;
  font-weight: 600;
}
.insights-avg-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

/* Mini cards */
.avg-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1px;
  padding: 6px 12px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  min-width: 72px;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.avg-card:hover {
  border-color: #cbd5e1;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.05);
}
.avg-card-label {
  font-size: 0.65rem;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.03em;
  font-weight: 500;
  white-space: nowrap;
}
.avg-card-value {
  font-size: 0.95rem;
  font-weight: 600;
  color: #334155;
  line-height: 1.3;
}

/* "All" card — visually distinct */
.avg-card-all {
  background: #eff6ff;
  border-color: #bfdbfe;
  min-width: 80px;
  padding: 6px 16px;
}
.avg-card-all .avg-card-label {
  color: #1e40af;
  font-weight: 600;
}
.avg-card-all .avg-card-value {
  font-size: 1.1rem;
  font-weight: 700;
  color: #1e40af;
}

/* Right: planned work */
.insights-pause {
  flex-shrink: 0;
}

@media (max-width: 768px) {
  .insights-row {
    gap: 16px;
  }
  .insight-divider {
    display: none;
  }
  .insights-stats {
    width: 100%;
    justify-content: center;
  }
  .insights-averages {
    width: 100%;
  }
  .insights-pause {
    width: 100%;
    display: flex;
    justify-content: center;
  }
}

.empty-cat {
  text-align: center;
  padding: 48px 16px;
  color: #cbd5e1;
  font-size: 0.85rem;
}

.chart-wrap {
  height: 260px;
  position: relative;
}

/* Modal */
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px;
}

.modal-content {
  background: #fff;
  border-radius: 12px;
  width: 100%;
  max-width: 1400px;
  padding: 24px;
  box-shadow: 0 20px 60px rgba(0,0,0,0.2);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.modal-title-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.modal-cat-name {
  font-weight: 700;
  font-size: 1.2rem;
  color: #1e293b;
}

.modal-close {
  background: none;
  border: none;
  font-size: 1.6rem;
  color: #94a3b8;
  cursor: pointer;
  padding: 4px 10px;
  border-radius: 6px;
  line-height: 1;
}

.modal-close:hover {
  background: #f1f5f9;
  color: #475569;
}

.modal-chart-wrap {
  height: 500px;
  position: relative;
}
</style>
