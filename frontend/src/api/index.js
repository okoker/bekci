import axios from 'axios'
import { useAuthStore } from '../stores/auth'
import router from '../router'

const api = axios.create({
  baseURL: '/api',
  headers: { 'Content-Type': 'application/json' },
  withCredentials: true,
})

// On 401, force logout — but skip for session probe (/me) and public SOC endpoints
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      const url = error.config?.url || ''
      if (url !== '/me' && !url.startsWith('/soc/') && !url.startsWith('/system/')) {
        const auth = useAuthStore()
        auth.clearAuth()
        router.push('/login')
      }
    }
    return Promise.reject(error)
  }
)

export default api
