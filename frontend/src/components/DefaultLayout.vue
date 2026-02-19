<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useAuthStore } from '../stores/auth'
import { useRouter, useRoute } from 'vue-router'
import api from '../api'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

async function handleLogout() {
  await auth.logout()
  router.push('/login')
}

// --- User Menu ---
const showUserMenu = ref(false)

function toggleUserMenu() {
  showUserMenu.value = !showUserMenu.value
}

function closeUserMenu(e) {
  if (!e.target.closest('.user-menu')) {
    showUserMenu.value = false
  }
}

// --- System Health ---
const health = ref(null)
const appVersion = ref('')
const showPopover = ref(false)
let pollTimer = null

async function fetchHealth() {
  try {
    const { data } = await api.get('/system/health')
    health.value = data
    if (data.version) appVersion.value = data.version
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

watch(() => route.path, () => {
  showUserMenu.value = false
})

onMounted(() => {
  fetchHealth()
  pollTimer = setInterval(fetchHealth, 30000)
  document.addEventListener('click', closePopover)
  document.addEventListener('click', closeUserMenu)
})

onUnmounted(() => {
  clearInterval(pollTimer)
  document.removeEventListener('click', closePopover)
  document.removeEventListener('click', closeUserMenu)
})
</script>

<template>
  <div class="layout-v">
    <nav class="navbar">
      <div class="navbar-left">
        <router-link to="/" class="navbar-brand"><img src="/bekci-icon.png" alt="Bekci" class="navbar-icon" />Bekci</router-link>
        <router-link to="/" class="nav-link">Dashboard</router-link>
        <router-link to="/sla" class="nav-link">SLA</router-link>
        <router-link to="/targets" class="nav-link">Targets</router-link>
        <router-link to="/soc" class="nav-link">SOC</router-link>
        <router-link to="/alerts" class="nav-link">Alerts</router-link>
        <router-link to="/settings" class="nav-link">Settings</router-link>
      </div>
      <div class="navbar-right">
        <span v-if="appVersion" class="app-version">v{{ appVersion }}</span>
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
        <div class="user-menu">
          <div class="user-menu-trigger" @click.stop="toggleUserMenu">
            <span class="navbar-user">{{ auth.user?.username }}</span>
            <span class="navbar-role">({{ auth.user?.role }})</span>
            <span class="user-caret">&#9662;</span>
          </div>
          <div v-if="showUserMenu" class="user-dropdown" @click.stop>
            <router-link to="/profile" class="user-dropdown-item" @click="showUserMenu = false">Profile</router-link>
            <button class="user-dropdown-item" @click="handleLogout">Logout</button>
          </div>
        </div>
      </div>
    </nav>
    <main class="content">
      <slot />
    </main>
  </div>
</template>
