<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()

const username = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function handleLogin() {
  error.value = ''
  loading.value = true
  try {
    await auth.login(username.value, password.value)
    router.push('/')
  } catch (e) {
    error.value = e.response?.data?.error || 'Login failed'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <div class="login-card">
      <h1>Bekci</h1>
      <form @submit.prevent="handleLogin">
        <div class="form-group">
          <label for="username">Username</label>
          <input id="username" v-model="username" type="text" autocomplete="username" required />
        </div>
        <div class="form-group">
          <label for="password">Password</label>
          <input id="password" v-model="password" type="password" autocomplete="current-password" required />
        </div>
        <div v-if="error" class="error-msg">{{ error }}</div>
        <button type="submit" class="btn btn-primary btn-block" :disabled="loading">
          {{ loading ? 'Signing in...' : 'Sign in' }}
        </button>
      </form>
    </div>
  </div>
</template>
