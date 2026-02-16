<script setup>
import { ref, onMounted } from 'vue'
import api from '../api'

const users = ref([])
const showCreate = ref(false)
const showResetPw = ref(null)
const error = ref('')
const success = ref('')

const form = ref({ username: '', email: '', password: '', role: 'operator' })
const resetPwForm = ref({ password: '' })

async function loadUsers() {
  try {
    const { data } = await api.get('/users')
    users.value = data
  } catch (e) {
    error.value = 'Failed to load users'
  }
}

async function createUser() {
  error.value = ''
  try {
    await api.post('/users', form.value)
    success.value = 'User created'
    showCreate.value = false
    form.value = { username: '', email: '', password: '', role: 'operator' }
    await loadUsers()
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to create user'
  }
}

async function toggleSuspend(user) {
  error.value = ''
  try {
    await api.put(`/users/${user.id}/suspend`, { suspended: user.status === 'active' })
    success.value = `User ${user.status === 'active' ? 'suspended' : 'activated'}`
    await loadUsers()
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to update user'
  }
}

async function resetPassword(userId) {
  error.value = ''
  try {
    await api.put(`/users/${userId}/password`, { password: resetPwForm.value.password })
    success.value = 'Password reset'
    showResetPw.value = null
    resetPwForm.value = { password: '' }
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to reset password'
  }
}

onMounted(loadUsers)
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h2>Users</h2>
      <button class="btn btn-primary" @click="showCreate = !showCreate">
        {{ showCreate ? 'Cancel' : 'Create User' }}
      </button>
    </div>

    <div v-if="error" class="error-msg">{{ error }}</div>
    <div v-if="success" class="success-msg" @click="success = ''">{{ success }}</div>

    <div v-if="showCreate" class="card">
      <h3>Create User</h3>
      <form @submit.prevent="createUser">
        <div class="form-row">
          <div class="form-group">
            <label>Username</label>
            <input v-model="form.username" required />
          </div>
          <div class="form-group">
            <label>Email</label>
            <input v-model="form.email" type="email" />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label>Password (min 8 chars)</label>
            <input v-model="form.password" type="password" required minlength="8" />
          </div>
          <div class="form-group">
            <label>Role</label>
            <select v-model="form.role">
              <option value="admin">Admin</option>
              <option value="operator">Operator</option>
              <option value="viewer">Viewer</option>
            </select>
          </div>
        </div>
        <button type="submit" class="btn btn-primary">Create</button>
      </form>
    </div>

    <div class="card">
      <table>
        <thead>
          <tr>
            <th>Username</th>
            <th>Email</th>
            <th>Role</th>
            <th>Status</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="u in users" :key="u.id">
            <td>{{ u.username }}</td>
            <td>{{ u.email || '-' }}</td>
            <td><span :class="'badge badge-' + u.role">{{ u.role }}</span></td>
            <td><span :class="'badge badge-' + u.status">{{ u.status }}</span></td>
            <td class="actions">
              <button class="btn btn-sm" @click="toggleSuspend(u)">
                {{ u.status === 'active' ? 'Suspend' : 'Activate' }}
              </button>
              <button class="btn btn-sm" @click="showResetPw = (showResetPw === u.id ? null : u.id)">
                Reset PW
              </button>
              <div v-if="showResetPw === u.id" class="inline-form">
                <input v-model="resetPwForm.password" type="password" placeholder="New password" minlength="8" />
                <button class="btn btn-sm btn-primary" @click="resetPassword(u.id)">Set</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="card privileges-card">
      <h3>Role Privileges</h3>
      <table class="privileges-table">
        <thead>
          <tr>
            <th>Action</th>
            <th>Viewer</th>
            <th>Operator</th>
            <th>Admin</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>View dashboard &amp; SOC</td>
            <td class="perm-yes">Yes</td>
            <td class="perm-yes">Yes</td>
            <td class="perm-yes">Yes</td>
          </tr>
          <tr>
            <td>View targets, checks &amp; results</td>
            <td class="perm-yes">Yes</td>
            <td class="perm-yes">Yes</td>
            <td class="perm-yes">Yes</td>
          </tr>
          <tr>
            <td>View settings</td>
            <td class="perm-yes">Yes</td>
            <td class="perm-yes">Yes</td>
            <td class="perm-yes">Yes</td>
          </tr>
          <tr>
            <td>Edit own profile &amp; password</td>
            <td class="perm-yes">Yes</td>
            <td class="perm-yes">Yes</td>
            <td class="perm-yes">Yes</td>
          </tr>
          <tr>
            <td>Create, edit &amp; delete targets</td>
            <td class="perm-no">No</td>
            <td class="perm-yes">Yes</td>
            <td class="perm-yes">Yes</td>
          </tr>
          <tr>
            <td>Run checks manually</td>
            <td class="perm-no">No</td>
            <td class="perm-yes">Yes</td>
            <td class="perm-yes">Yes</td>
          </tr>
          <tr>
            <td>View audit log</td>
            <td class="perm-no">No</td>
            <td class="perm-yes">Yes</td>
            <td class="perm-yes">Yes</td>
          </tr>
          <tr>
            <td>Modify settings</td>
            <td class="perm-no">No</td>
            <td class="perm-no">No</td>
            <td class="perm-yes">Yes</td>
          </tr>
          <tr>
            <td>Manage users</td>
            <td class="perm-no">No</td>
            <td class="perm-no">No</td>
            <td class="perm-yes">Yes</td>
          </tr>
          <tr>
            <td>Backup &amp; restore</td>
            <td class="perm-no">No</td>
            <td class="perm-no">No</td>
            <td class="perm-yes">Yes</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.privileges-card {
  margin-top: 1.5rem;
  opacity: 0.85;
}
.privileges-card h3 {
  margin-bottom: 0.75rem;
  font-size: 0.95rem;
}
.privileges-table {
  font-size: 0.85rem;
}
.privileges-table th:not(:first-child),
.privileges-table td:not(:first-child) {
  text-align: center;
  width: 100px;
}
.perm-yes {
  color: #16a34a;
  font-weight: 600;
}
.perm-no {
  color: #9ca3af;
}
</style>
