<script setup>
import { ref, computed, watch } from 'vue'
import { useAuthStore } from '../stores/auth'
import api from '../api'

const auth = useAuthStore()

// State
const projects = ref([])
const selectedProjectId = ref('')
const targets = ref([])
const expandedTargetId = ref(null)
const loading = ref(false)
const error = ref('')
const success = ref('')

// Project form
const showProjectForm = ref(false)
const editingProject = ref(null)
const projectForm = ref({ name: '', description: '' })

// Target form
const showTargetForm = ref(false)
const editingTarget = ref(null)
const targetForm = ref({ name: '', host: '', description: '', enabled: true, preferred_check_type: 'ping' })

// Check form
const showCheckForm = ref(false)
const editingCheck = ref(null)
const checkTargetId = ref('')
const checkForm = ref({ type: 'http', name: '', config: {}, interval_s: 60, enabled: true })

const checkTypes = [
  { value: 'http', label: 'HTTP/HTTPS' },
  { value: 'tcp', label: 'TCP' },
  { value: 'ping', label: 'Ping (ICMP)' },
  { value: 'dns', label: 'DNS' },
  { value: 'page_hash', label: 'Page Hash' },
  { value: 'tls_cert', label: 'TLS Certificate' },
]

const selectedProject = computed(() => projects.value.find(p => p.id === selectedProjectId.value))

// Load data
async function loadProjects() {
  try {
    const { data } = await api.get('/projects')
    projects.value = data
    if (data.length > 0 && !selectedProjectId.value) {
      selectedProjectId.value = data[0].id
    }
  } catch (e) {
    error.value = 'Failed to load projects'
  }
}

async function loadTargets() {
  if (!selectedProjectId.value) {
    targets.value = []
    return
  }
  loading.value = true
  try {
    const { data } = await api.get('/targets', { params: { project_id: selectedProjectId.value } })
    targets.value = data
  } catch (e) {
    error.value = 'Failed to load targets'
  } finally {
    loading.value = false
  }
}

watch(selectedProjectId, () => {
  expandedTargetId.value = null
  loadTargets()
})

// Project CRUD
function openProjectForm(project = null) {
  editingProject.value = project
  projectForm.value = project ? { name: project.name, description: project.description } : { name: '', description: '' }
  showProjectForm.value = true
  error.value = ''
}

async function saveProject() {
  error.value = ''
  try {
    if (editingProject.value) {
      await api.put(`/projects/${editingProject.value.id}`, projectForm.value)
    } else {
      const { data } = await api.post('/projects', projectForm.value)
      selectedProjectId.value = data.id
    }
    showProjectForm.value = false
    await loadProjects()
    success.value = editingProject.value ? 'Project updated' : 'Project created'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to save project'
  }
}

async function deleteProject(id) {
  if (!confirm('Delete this project and all its targets/checks?')) return
  try {
    await api.delete(`/projects/${id}`)
    if (selectedProjectId.value === id) selectedProjectId.value = ''
    await loadProjects()
    success.value = 'Project deleted'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to delete project'
  }
}

// Target CRUD
function openTargetForm(target = null) {
  editingTarget.value = target
  targetForm.value = target
    ? { name: target.name, host: target.host, description: target.description, enabled: target.enabled, preferred_check_type: target.preferred_check_type || 'ping' }
    : { name: '', host: '', description: '', enabled: true, preferred_check_type: 'ping' }
  showTargetForm.value = true
  error.value = ''
}

async function saveTarget() {
  error.value = ''
  try {
    if (editingTarget.value) {
      await api.put(`/targets/${editingTarget.value.id}`, targetForm.value)
    } else {
      await api.post('/targets', { ...targetForm.value, project_id: selectedProjectId.value })
    }
    showTargetForm.value = false
    await loadTargets()
    success.value = editingTarget.value ? 'Target updated' : 'Target created'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to save target'
  }
}

async function deleteTarget(id) {
  if (!confirm('Delete this target and all its checks?')) return
  try {
    await api.delete(`/targets/${id}`)
    await loadTargets()
    success.value = 'Target deleted'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to delete target'
  }
}

// Expand target to see checks
const targetChecks = ref({}) // targetId -> checks[]

async function toggleTarget(targetId) {
  if (expandedTargetId.value === targetId) {
    expandedTargetId.value = null
    return
  }
  expandedTargetId.value = targetId
  try {
    const { data } = await api.get(`/targets/${targetId}`)
    targetChecks.value[targetId] = data.checks || []
  } catch (e) {
    error.value = 'Failed to load checks'
  }
}

// Check CRUD
function getDefaultConfig(type_) {
  switch (type_) {
    case 'http': return { scheme: 'https', port: 0, endpoint: '/', expect_status: 200, skip_tls_verify: false, timeout_s: 10 }
    case 'tcp': return { port: 80, timeout_s: 5 }
    case 'ping': return { count: 3, timeout_s: 5 }
    case 'dns': return { query: '', record_type: 'A', expect_value: '', nameserver: '' }
    case 'page_hash': return { scheme: 'https', endpoint: '/', baseline_hash: '', timeout_s: 10 }
    case 'tls_cert': return { port: 443, warn_days: 30, timeout_s: 10 }
    default: return {}
  }
}

function openCheckForm(targetId, check = null) {
  checkTargetId.value = targetId
  editingCheck.value = check
  if (check) {
    let cfg = {}
    try { cfg = JSON.parse(check.config) } catch {}
    checkForm.value = { type: check.type, name: check.name, config: cfg, interval_s: check.interval_s, enabled: check.enabled }
  } else {
    checkForm.value = { type: 'http', name: '', config: getDefaultConfig('http'), interval_s: 60, enabled: true }
  }
  showCheckForm.value = true
  error.value = ''
}

function onCheckTypeChange() {
  if (!editingCheck.value) {
    checkForm.value.config = getDefaultConfig(checkForm.value.type)
  }
}

async function saveCheck() {
  error.value = ''
  try {
    const payload = {
      type: checkForm.value.type,
      name: checkForm.value.name,
      config: JSON.stringify(checkForm.value.config),
      interval_s: Number(checkForm.value.interval_s),
      enabled: checkForm.value.enabled,
    }
    if (editingCheck.value) {
      await api.put(`/checks/${editingCheck.value.id}`, payload)
    } else {
      await api.post(`/targets/${checkTargetId.value}/checks`, payload)
    }
    showCheckForm.value = false
    await toggleTarget(checkTargetId.value) // reload
    expandedTargetId.value = checkTargetId.value
    success.value = editingCheck.value ? 'Check updated' : 'Check created'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to save check'
  }
}

async function deleteCheck(checkId, targetId) {
  if (!confirm('Delete this check?')) return
  try {
    await api.delete(`/checks/${checkId}`)
    await toggleTarget(targetId)
    expandedTargetId.value = targetId
    success.value = 'Check deleted'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to delete check'
  }
}

async function runCheckNow(checkId) {
  try {
    await api.post(`/checks/${checkId}/run`)
    success.value = 'Check queued for immediate run'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to run check'
  }
}

function formatInterval(s) {
  if (s >= 3600) return `${Math.floor(s / 3600)}h`
  if (s >= 60) return `${Math.floor(s / 60)}m`
  return `${s}s`
}

// Init
loadProjects()
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h2>Targets</h2>
    </div>

    <div v-if="error" class="error-msg">{{ error }}</div>
    <div v-if="success" class="success-msg" @click="success = ''">{{ success }}</div>

    <!-- Projects bar -->
    <div class="card">
      <div class="projects-bar">
        <div class="projects-tabs">
          <button v-for="p in projects" :key="p.id"
            :class="['project-tab', { active: p.id === selectedProjectId }]"
            @click="selectedProjectId = p.id">
            {{ p.name }}
          </button>
        </div>
        <div class="projects-actions" v-if="auth.isOperator">
          <button class="btn btn-sm" @click="openProjectForm()">+ Project</button>
          <button v-if="selectedProject" class="btn btn-sm" @click="openProjectForm(selectedProject)">Edit</button>
          <button v-if="selectedProject && auth.isAdmin" class="btn btn-sm btn-danger" @click="deleteProject(selectedProjectId)">Delete</button>
        </div>
      </div>
    </div>

    <!-- Project form modal -->
    <div v-if="showProjectForm" class="modal-overlay" @click.self="showProjectForm = false">
      <div class="modal-card">
        <h3>{{ editingProject ? 'Edit Project' : 'New Project' }}</h3>
        <form @submit.prevent="saveProject">
          <div class="form-group">
            <label>Name</label>
            <input v-model="projectForm.name" required />
          </div>
          <div class="form-group">
            <label>Description</label>
            <input v-model="projectForm.description" />
          </div>
          <div class="form-actions">
            <button type="submit" class="btn btn-primary">Save</button>
            <button type="button" class="btn" @click="showProjectForm = false">Cancel</button>
          </div>
        </form>
      </div>
    </div>

    <!-- Targets table -->
    <div v-if="selectedProjectId" class="card">
      <div class="card-header">
        <h3>Targets</h3>
        <button v-if="auth.isOperator" class="btn btn-sm btn-primary" @click="openTargetForm()">+ Target</button>
      </div>

      <div v-if="loading" class="text-muted" style="padding: 1rem;">Loading...</div>
      <div v-else-if="targets.length === 0" class="text-muted" style="padding: 1rem;">No targets in this project yet.</div>

      <table v-else>
        <thead>
          <tr>
            <th>Name</th>
            <th>Host</th>
            <th>Status</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <template v-for="t in targets" :key="t.id">
            <tr :class="{ 'row-expanded': expandedTargetId === t.id }" @click="toggleTarget(t.id)" style="cursor: pointer;">
              <td>
                <span class="expand-icon">{{ expandedTargetId === t.id ? '&#9660;' : '&#9654;' }}</span>
                {{ t.name }}
                <span v-if="!t.enabled" class="badge badge-suspended">disabled</span>
              </td>
              <td>{{ t.host }}</td>
              <td>{{ t.description || '—' }}</td>
              <td class="actions" @click.stop>
                <button v-if="auth.isOperator" class="btn btn-sm" @click="openTargetForm(t)">Edit</button>
                <button v-if="auth.isOperator" class="btn btn-sm btn-danger" @click="deleteTarget(t.id)">Delete</button>
              </td>
            </tr>
            <!-- Expanded: checks list -->
            <tr v-if="expandedTargetId === t.id">
              <td colspan="4" class="checks-panel">
                <div class="checks-header">
                  <strong>Checks</strong>
                  <button v-if="auth.isOperator" class="btn btn-sm btn-primary" @click="openCheckForm(t.id)">+ Check</button>
                </div>
                <div v-if="!targetChecks[t.id] || targetChecks[t.id].length === 0" class="text-muted" style="padding: 0.5rem 0;">
                  No checks configured.
                </div>
                <table v-else class="checks-table">
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Type</th>
                      <th>Interval</th>
                      <th>Enabled</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="c in targetChecks[t.id]" :key="c.id">
                      <td>{{ c.name }}</td>
                      <td><span class="badge badge-type">{{ c.type }}</span></td>
                      <td>{{ formatInterval(c.interval_s) }}</td>
                      <td><span :class="['badge', c.enabled ? 'badge-active' : 'badge-suspended']">{{ c.enabled ? 'yes' : 'no' }}</span></td>
                      <td class="actions">
                        <button v-if="auth.isOperator" class="btn btn-sm" @click="runCheckNow(c.id)">Run now</button>
                        <button v-if="auth.isOperator" class="btn btn-sm" @click="openCheckForm(t.id, c)">Edit</button>
                        <button v-if="auth.isOperator" class="btn btn-sm btn-danger" @click="deleteCheck(c.id, t.id)">Delete</button>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>

    <div v-else class="card placeholder-card">
      <p>No project selected. Create a project to get started.</p>
    </div>

    <!-- Target form modal -->
    <div v-if="showTargetForm" class="modal-overlay" @click.self="showTargetForm = false">
      <div class="modal-card">
        <h3>{{ editingTarget ? 'Edit Target' : 'New Target' }}</h3>
        <form @submit.prevent="saveTarget">
          <div class="form-group">
            <label>Name</label>
            <input v-model="targetForm.name" required placeholder="e.g. Web Server" />
          </div>
          <div class="form-group">
            <label>Host</label>
            <input v-model="targetForm.host" required placeholder="e.g. google.com or 192.168.1.1" />
          </div>
          <div class="form-group">
            <label>Description</label>
            <input v-model="targetForm.description" placeholder="Optional description" />
          </div>
          <div class="form-group">
            <label>Preferred Check Type</label>
            <select v-model="targetForm.preferred_check_type">
              <option v-for="t in checkTypes" :key="t.value" :value="t.value">{{ t.label }}</option>
            </select>
          </div>
          <div class="form-group">
            <label><input type="checkbox" v-model="targetForm.enabled" /> Enabled</label>
          </div>
          <div class="form-actions">
            <button type="submit" class="btn btn-primary">Save</button>
            <button type="button" class="btn" @click="showTargetForm = false">Cancel</button>
          </div>
        </form>
      </div>
    </div>

    <!-- Check form modal -->
    <div v-if="showCheckForm" class="modal-overlay" @click.self="showCheckForm = false">
      <div class="modal-card modal-wide">
        <h3>{{ editingCheck ? 'Edit Check' : 'New Check' }}</h3>
        <form @submit.prevent="saveCheck">
          <div class="form-row">
            <div class="form-group">
              <label>Name</label>
              <input v-model="checkForm.name" required placeholder="e.g. HTTPS Check" />
            </div>
            <div class="form-group">
              <label>Type</label>
              <select v-model="checkForm.type" @change="onCheckTypeChange" :disabled="!!editingCheck">
                <option v-for="t in checkTypes" :key="t.value" :value="t.value">{{ t.label }}</option>
              </select>
            </div>
          </div>

          <div class="form-row">
            <div class="form-group">
              <label>Interval (seconds)</label>
              <input type="number" v-model.number="checkForm.interval_s" min="10" />
            </div>
            <div class="form-group">
              <label><input type="checkbox" v-model="checkForm.enabled" /> Enabled</label>
            </div>
          </div>

          <!-- HTTP config -->
          <template v-if="checkForm.type === 'http'">
            <div class="form-row">
              <div class="form-group">
                <label>Scheme</label>
                <select v-model="checkForm.config.scheme">
                  <option value="http">HTTP</option>
                  <option value="https">HTTPS</option>
                </select>
              </div>
              <div class="form-group">
                <label>Port (0 = default)</label>
                <input type="number" v-model.number="checkForm.config.port" min="0" max="65535" />
              </div>
            </div>
            <div class="form-row">
              <div class="form-group">
                <label>Endpoint</label>
                <input v-model="checkForm.config.endpoint" placeholder="/" />
              </div>
              <div class="form-group">
                <label>Expected Status</label>
                <input type="number" v-model.number="checkForm.config.expect_status" />
              </div>
            </div>
            <div class="form-row">
              <div class="form-group">
                <label>Timeout (s)</label>
                <input type="number" v-model.number="checkForm.config.timeout_s" min="1" max="60" />
              </div>
              <div class="form-group">
                <label><input type="checkbox" v-model="checkForm.config.skip_tls_verify" /> Skip TLS Verify</label>
              </div>
            </div>
          </template>

          <!-- TCP config -->
          <template v-if="checkForm.type === 'tcp'">
            <div class="form-row">
              <div class="form-group">
                <label>Port</label>
                <input type="number" v-model.number="checkForm.config.port" min="1" max="65535" required />
              </div>
              <div class="form-group">
                <label>Timeout (s)</label>
                <input type="number" v-model.number="checkForm.config.timeout_s" min="1" max="60" />
              </div>
            </div>
          </template>

          <!-- Ping config -->
          <template v-if="checkForm.type === 'ping'">
            <div class="form-row">
              <div class="form-group">
                <label>Ping Count</label>
                <input type="number" v-model.number="checkForm.config.count" min="1" max="10" />
              </div>
              <div class="form-group">
                <label>Timeout (s)</label>
                <input type="number" v-model.number="checkForm.config.timeout_s" min="1" max="30" />
              </div>
            </div>
          </template>

          <!-- DNS config -->
          <template v-if="checkForm.type === 'dns'">
            <div class="form-row">
              <div class="form-group">
                <label>Query (empty = target host)</label>
                <input v-model="checkForm.config.query" placeholder="Leave empty for target host" />
              </div>
              <div class="form-group">
                <label>Record Type</label>
                <select v-model="checkForm.config.record_type">
                  <option value="A">A</option>
                  <option value="AAAA">AAAA</option>
                  <option value="MX">MX</option>
                  <option value="CNAME">CNAME</option>
                </select>
              </div>
            </div>
            <div class="form-row">
              <div class="form-group">
                <label>Expected Value (optional)</label>
                <input v-model="checkForm.config.expect_value" placeholder="e.g. 1.2.3.4" />
              </div>
              <div class="form-group">
                <label>Nameserver (optional)</label>
                <input v-model="checkForm.config.nameserver" placeholder="e.g. 8.8.8.8" />
              </div>
            </div>
          </template>

          <!-- Page Hash config -->
          <template v-if="checkForm.type === 'page_hash'">
            <div class="form-row">
              <div class="form-group">
                <label>Scheme</label>
                <select v-model="checkForm.config.scheme">
                  <option value="http">HTTP</option>
                  <option value="https">HTTPS</option>
                </select>
              </div>
              <div class="form-group">
                <label>Endpoint</label>
                <input v-model="checkForm.config.endpoint" placeholder="/" />
              </div>
            </div>
            <div class="form-group">
              <label>Baseline Hash (empty = auto-capture on first run)</label>
              <input v-model="checkForm.config.baseline_hash" placeholder="Leave empty to auto-capture" />
            </div>
          </template>

          <!-- TLS Cert config -->
          <template v-if="checkForm.type === 'tls_cert'">
            <div class="form-row">
              <div class="form-group">
                <label>Port</label>
                <input type="number" v-model.number="checkForm.config.port" min="1" max="65535" />
              </div>
              <div class="form-group">
                <label>Warning Days</label>
                <input type="number" v-model.number="checkForm.config.warn_days" min="1" />
              </div>
            </div>
          </template>

          <div class="form-actions">
            <button type="submit" class="btn btn-primary">Save</button>
            <button type="button" class="btn" @click="showCheckForm = false">Cancel</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<style scoped>
.projects-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
}
.projects-tabs {
  display: flex;
  gap: 0.25rem;
  flex-wrap: wrap;
}
.project-tab {
  padding: 0.375rem 0.75rem;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  background: #f8fafc;
  color: #475569;
  cursor: pointer;
  font-size: 0.85rem;
  transition: all 0.15s;
}
.project-tab:hover { background: #e2e8f0; }
.project-tab.active { background: #2563eb; color: #fff; border-color: #2563eb; }

.projects-actions {
  display: flex;
  gap: 0.25rem;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.75rem;
}
.card-header h3 { margin: 0; }

.row-expanded > td { background: #f8fafc; }

.expand-icon {
  display: inline-block;
  width: 1rem;
  font-size: 0.7rem;
  color: #94a3b8;
}

.checks-panel {
  background: #f8fafc;
  padding: 0.75rem 1rem;
}
.checks-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}
.checks-table {
  font-size: 0.85rem;
}
.checks-table th { font-size: 0.75rem; }

.badge-type {
  background: #e0e7ff;
  color: #3730a3;
  font-size: 0.7rem;
  text-transform: uppercase;
}

/* Modal */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.3);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}
.modal-card {
  background: #fff;
  border-radius: 8px;
  padding: 1.5rem;
  width: 100%;
  max-width: 420px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.15);
}
.modal-card h3 { margin-bottom: 1rem; }
.modal-wide { max-width: 560px; }

.form-actions {
  display: flex;
  gap: 0.5rem;
  margin-top: 1rem;
}

.btn-danger {
  color: #991b1b;
  border-color: #fca5a5;
}
.btn-danger:hover { background: #fee2e2; }
</style>
