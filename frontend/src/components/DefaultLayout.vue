<script setup>
import { useAuthStore } from '../stores/auth'
import { useRouter } from 'vue-router'

const auth = useAuthStore()
const router = useRouter()

async function handleLogout() {
  await auth.logout()
  router.push('/login')
}
</script>

<template>
  <div class="layout">
    <aside class="sidebar">
      <div class="sidebar-brand">Bekci</div>
      <nav class="sidebar-nav">
        <router-link to="/" class="nav-item">Dashboard</router-link>
        <router-link to="/targets" class="nav-item">Targets</router-link>
        <router-link v-if="auth.isAdmin" to="/users" class="nav-item">Users</router-link>
        <router-link to="/settings" class="nav-item">Settings</router-link>
        <router-link to="/profile" class="nav-item">Profile</router-link>
      </nav>
    </aside>
    <div class="main">
      <header class="topbar">
        <div class="topbar-left"></div>
        <div class="topbar-right">
          <span class="topbar-user">{{ auth.user?.username }}</span>
          <span class="topbar-role">({{ auth.user?.role }})</span>
          <button class="btn btn-sm" @click="handleLogout">Logout</button>
        </div>
      </header>
      <main class="content">
        <slot />
      </main>
    </div>
  </div>
</template>
