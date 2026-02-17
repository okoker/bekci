<script setup>
import { ref, onMounted } from 'vue'
import { useAuthStore } from '../stores/auth'
import api from '../api'

const auth = useAuthStore()
const email = ref('')
const phone = ref('')
const profileMsg = ref('')
const profileErr = ref('')

const pwForm = ref({ current_password: '', new_password: '' })
const pwMsg = ref('')
const pwErr = ref('')

onMounted(() => {
  email.value = auth.user?.email || ''
  phone.value = auth.user?.phone || ''
})

async function updateProfile() {
  profileErr.value = ''
  try {
    await api.put('/me', { email: email.value, phone: phone.value })
    profileMsg.value = 'Profile updated'
    auth.user.email = email.value
    auth.user.phone = phone.value
  } catch (e) {
    profileErr.value = e.response?.data?.error || 'Update failed'
  }
}

async function changePassword() {
  pwErr.value = ''
  try {
    await api.put('/me/password', pwForm.value)
    pwMsg.value = 'Password changed'
    pwForm.value = { current_password: '', new_password: '' }
  } catch (e) {
    pwErr.value = e.response?.data?.error || 'Password change failed'
  }
}
</script>

<template>
  <div class="page">
    <h2>Profile</h2>

    <div class="card">
      <h3>Account Info</h3>
      <div class="form-group">
        <label>Username</label>
        <input :value="auth.user?.username" disabled />
      </div>
      <div class="form-group">
        <label>Role</label>
        <input :value="auth.user?.role" disabled />
      </div>
      <form @submit.prevent="updateProfile">
        <div class="form-group">
          <label>Email</label>
          <input v-model="email" type="email" />
        </div>
        <div class="form-group">
          <label>Phone</label>
          <input v-model="phone" type="tel" placeholder="+1234567890" />
          <span class="text-muted" style="font-size:0.8rem;">For Signal alerts (coming soon)</span>
        </div>
        <div v-if="profileErr" class="error-msg">{{ profileErr }}</div>
        <div v-if="profileMsg" class="success-msg" @click="profileMsg = ''">{{ profileMsg }}</div>
        <button type="submit" class="btn btn-primary">Save</button>
      </form>
    </div>

    <div class="card">
      <h3>Change Password</h3>
      <form @submit.prevent="changePassword">
        <div class="form-group">
          <label>Current Password</label>
          <input v-model="pwForm.current_password" type="password" required />
        </div>
        <div class="form-group">
          <label>New Password (min 8 chars)</label>
          <input v-model="pwForm.new_password" type="password" required minlength="8" />
        </div>
        <div v-if="pwErr" class="error-msg">{{ pwErr }}</div>
        <div v-if="pwMsg" class="success-msg" @click="pwMsg = ''">{{ pwMsg }}</div>
        <button type="submit" class="btn btn-primary">Change Password</button>
      </form>
    </div>
  </div>
</template>
