import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import api from '../api'

export const useAuthStore = defineStore('auth', () => {
  const user = ref(null)

  const isLoggedIn = computed(() => !!user.value)
  const isAdmin = computed(() => user.value?.role === 'admin')
  const isOperator = computed(() => user.value?.role === 'operator' || isAdmin.value)

  async function login(username, password) {
    const { data } = await api.post('/login', { username, password })
    user.value = data.user
  }

  async function fetchMe() {
    try {
      const { data } = await api.get('/me')
      user.value = data
    } catch {
      user.value = null
    }
  }

  async function logout() {
    try {
      await api.post('/logout')
    } catch {
      // ignore
    }
    user.value = null
  }

  function clearAuth() {
    user.value = null
  }

  return { user, isLoggedIn, isAdmin, isOperator, login, fetchMe, logout, clearAuth }
})
