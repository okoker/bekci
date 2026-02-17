<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useAuthStore } from '../stores/auth'
import { useRouter } from 'vue-router'
import api from '../api'

const auth = useAuthStore()
const router = useRouter()

async function handleLogout() {
  await auth.logout()
  router.push('/login')
}

// --- System Health ---
const health = ref(null)
const showPopover = ref(false)
let pollTimer = null

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
  if (metric === 'net') {
    return health.value.net.status === 'ok' ? 'dot-green' : 'dot-red'
  }
  if (metric === 'disk') {
    const d = health.value.disk
    if (!d.total_gb) return 'dot-grey'
    const pct = d.free_gb / d.total_gb
    if (pct > 0.2) return 'dot-green'
    if (pct > 0.1) return 'dot-yellow'
    return 'dot-red'
  }
  if (metric === 'cpu') {
    const c = health.value.cpu
    if (c.load1 < 0) return 'dot-grey'
    if (c.load1 < c.num_cpu) return 'dot-green'
    if (c.load1 < c.num_cpu * 2) return 'dot-yellow'
    return 'dot-red'
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
  if (c.load1 < 0) return 'Load: —'
  return `Load: ${c.load1} (${c.num_cpu} cores)`
})

function togglePopover() {
  showPopover.value = !showPopover.value
}

function closePopover(e) {
  if (!e.target.closest('.health-indicator')) {
    showPopover.value = false
  }
}

onMounted(() => {
  fetchHealth()
  pollTimer = setInterval(fetchHealth, 30000)
  document.addEventListener('click', closePopover)
})

onUnmounted(() => {
  clearInterval(pollTimer)
  document.removeEventListener('click', closePopover)
})
</script>

<template>
  <div class="layout-v">
    <nav class="navbar">
      <div class="navbar-left">
        <router-link to="/" class="navbar-brand"><img src="/bekci-icon.png" alt="Bekci" class="navbar-icon" />Bekci</router-link>
        <router-link to="/" class="nav-link">Dashboard</router-link>
        <router-link to="/targets" class="nav-link">Targets</router-link>
        <router-link to="/soc" class="nav-link">SOC</router-link>
        <router-link to="/settings" class="nav-link">Settings</router-link>
        <router-link to="/profile" class="nav-link">Profile</router-link>
      </div>
      <div class="navbar-right">
        <div class="health-indicator" @click.stop="togglePopover">
          <div class="health-dots">
            <span class="health-dot" :class="dotColor('net')" title="Network"></span>
            <span class="health-dot" :class="dotColor('disk')" title="Disk"></span>
            <span class="health-dot" :class="dotColor('cpu')" title="CPU"></span>
          </div>
          <div v-if="showPopover" class="health-popover">
            <div class="health-row" :class="dotColor('net')">{{ netLabel }}</div>
            <div class="health-row" :class="dotColor('disk')">{{ diskLabel }}</div>
            <div class="health-row" :class="dotColor('cpu')">{{ cpuLabel }}</div>
          </div>
        </div>
        <span class="navbar-user">{{ auth.user?.username }}</span>
        <span class="navbar-role">({{ auth.user?.role }})</span>
        <button class="btn btn-sm" @click="handleLogout">Logout</button>
      </div>
    </nav>
    <main class="content">
      <slot />
    </main>
  </div>
</template>
