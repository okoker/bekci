import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import api from '../api'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('token') || '')
  const user = ref(null)

  const isLoggedIn = computed(() => !!token.value)
  const isAdmin = computed(() => user.value?.role === 'admin')
  const isOperator = computed(() => user.value?.role === 'operator' || isAdmin.value)

  async function login(username, password) {
    const { data } = await api.post('/login', { username, password })
    token.value = data.token
    user.value = data.user
    localStorage.setItem('token', data.token)
  }

  async function fetchMe() {
    try {
      const { data } = await api.get('/me')
      user.value = data
    } catch {
      clearAuth()
    }
  }

  async function logout() {
    try {
      await api.post('/logout')
    } catch {
      // ignore
    }
    clearAuth()
  }

  function clearAuth() {
    token.value = ''
    user.value = null
    localStorage.removeItem('token')
  }

  return { token, user, isLoggedIn, isAdmin, isOperator, login, fetchMe, logout, clearAuth }
})
