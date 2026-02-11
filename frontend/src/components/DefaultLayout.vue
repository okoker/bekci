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
  <div class="layout-v">
    <nav class="navbar">
      <div class="navbar-left">
        <router-link to="/" class="navbar-brand">Bekci</router-link>
        <router-link to="/" class="nav-link">Dashboard</router-link>
        <router-link to="/targets" class="nav-link">Targets</router-link>
        <router-link to="/soc" class="nav-link">SOC</router-link>
        <router-link v-if="auth.isAdmin" to="/users" class="nav-link">Users</router-link>
        <router-link to="/settings" class="nav-link">Settings</router-link>
        <router-link to="/profile" class="nav-link">Profile</router-link>
      </div>
      <div class="navbar-right">
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
