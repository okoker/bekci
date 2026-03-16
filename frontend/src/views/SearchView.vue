<script setup>
import { ref, computed, onMounted } from 'vue'
import { useAuthStore } from '../stores/auth'
import TargetEditModal from '../components/TargetEditModal.vue'
import api from '../api'

const auth = useAuthStore()

// Data
const targets = ref([])
const projectOptions = ref([])
const locationOptions = ref([])
const allUsers = ref([])
const loading = ref(false)
const error = ref('')
const success = ref('')

// Filters
const searchText = ref('')
const filterProject = ref('')
const filterLocation = ref('')

// Expand
const expandedTargetId = ref(null)
const expandedDetail = ref(null)
const expandLoading = ref(false)

// Edit modal
const showEditModal = ref(false)
const editTargetId = ref(null)

// Saved searches (localStorage)
const STORAGE_KEY = 'bekci_saved_searches'
const savedSearches = ref([])
const showSaveName = ref(false)
const saveName = ref('')

function loadSavedSearches() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    savedSearches.value = raw ? JSON.parse(raw) : []
  } catch { savedSearches.value = [] }
}

function saveCurrentSearch() {
  const name = saveName.value.trim()
  if (!name) return
  const entry = {
    id: Date.now().toString(),
    name,
    text: searchText.value.trim(),
    project: filterProject.value,
    location: filterLocation.value
  }
  savedSearches.value.push(entry)
  localStorage.setItem(STORAGE_KEY, JSON.stringify(savedSearches.value))
  saveName.value = ''
  showSaveName.value = false
}

function applySavedSearch(s) {
  searchText.value = s.text || ''
  filterProject.value = s.project || ''
  filterLocation.value = s.location || ''
}

function deleteSavedSearch(id) {
  savedSearches.value = savedSearches.value.filter(s => s.id !== id)
  localStorage.setItem(STORAGE_KEY, JSON.stringify(savedSearches.value))
}

// Confirmation modals
const showDeleteConfirm = ref(false)
const pendingDeleteId = ref(null)
const showPauseConfirm = ref(false)
const showUnpauseConfirm = ref(false)
const pendingPauseId = ref(null)

const hasActiveFilter = computed(() => {
  return searchText.value.trim() !== '' || filterProject.value !== '' || filterLocation.value !== ''
})

const filteredTargets = computed(() => {
  if (!hasActiveFilter.value) return []
  let list = targets.value
  const q = searchText.value.toLowerCase().trim()
  if (q) {
    list = list.filter(t =>
      t.name.toLowerCase().includes(q) || t.host.toLowerCase().includes(q)
    )
  }
  if (filterProject.value) {
    list = list.filter(t => t.project === filterProject.value)
  }
  if (filterLocation.value) {
    list = list.filter(t => t.location === filterLocation.value)
  }
  return list
})

function recipientNames(recipientIds) {
  if (!recipientIds || recipientIds.length === 0) return []
  return recipientIds.map(id => {
    const u = allUsers.value.find(u => u.id === id)
    return u ? u.username : id.slice(0, 8)
  })
}

async function loadData() {
  loading.value = true
  try {
    const [tRes, pRes, lRes, uRes] = await Promise.all([
      api.get('/targets'),
      api.get('/tags?group=project'),
      api.get('/tags?group=location'),
      api.get('/users').catch(() => ({ data: [] }))
    ])
    targets.value = tRes.data
    projectOptions.value = pRes.data
    locationOptions.value = lRes.data
    allUsers.value = uRes.data || []
  } catch (e) {
    error.value = 'Failed to load data'
  } finally {
    loading.value = false
  }
}

async function toggleExpand(targetId) {
  if (expandedTargetId.value === targetId) {
    expandedTargetId.value = null
    expandedDetail.value = null
    return
  }
  expandedTargetId.value = targetId
  expandedDetail.value = null
  expandLoading.value = true
  try {
    const { data } = await api.get(`/targets/${targetId}`)
    expandedDetail.value = data
  } catch {
    error.value = 'Failed to load target details'
  } finally {
    expandLoading.value = false
  }
}

// Delete
function deleteTarget(id) {
  pendingDeleteId.value = id
  showDeleteConfirm.value = true
}

async function confirmDelete() {
  showDeleteConfirm.value = false
  const id = pendingDeleteId.value
  pendingDeleteId.value = null
  error.value = ''
  success.value = ''
  try {
    await api.delete(`/targets/${id}`)
    if (expandedTargetId.value === id) {
      expandedTargetId.value = null
      expandedDetail.value = null
    }
    await loadData()
    success.value = 'Target deleted'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to delete target'
  }
}

// Pause / Unpause
function pauseTarget(id) {
  pendingPauseId.value = id
  showPauseConfirm.value = true
}

async function confirmPause() {
  showPauseConfirm.value = false
  const id = pendingPauseId.value
  pendingPauseId.value = null
  error.value = ''
  success.value = ''
  try {
    await api.post(`/targets/${id}/pause`)
    await loadData()
    success.value = 'Target paused'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to pause target'
  }
}

function unpauseTarget(id) {
  pendingPauseId.value = id
  showUnpauseConfirm.value = true
}

async function confirmUnpause() {
  showUnpauseConfirm.value = false
  const id = pendingPauseId.value
  pendingPauseId.value = null
  error.value = ''
  success.value = ''
  try {
    await api.post(`/targets/${id}/unpause`)
    await loadData()
    success.value = 'Target unpaused — checks running now'
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to unpause target'
  }
}

function isTargetPaused(t) {
  return t.paused_at != null
}

function stateClass(state) {
  if (state === 'healthy') return 'badge-active'
  if (state === 'unhealthy') return 'badge-suspended'
  if (state === 'paused') return 'badge-paused'
  return 'badge-unknown'
}

function categoryClass(cat) {
  if (cat === 'Security') return 'badge-cat-security'
  if (cat === 'Network') return 'badge-cat-network'
  if (cat === 'Physical Security') return 'badge-cat-physical'
  if (cat === 'Key Services') return 'badge-cat-server'
  return 'badge-cat-other'
}

function formatInterval(s) {
  if (s >= 3600) return `${Math.floor(s / 3600)}h`
  if (s >= 60) return `${Math.floor(s / 60)}m`
  return `${s}s`
}

function formatDate(d) {
  if (!d) return '—'
  const dt = new Date(d)
  return dt.toLocaleDateString('en-GB') + ' ' + dt.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })
}

function openEdit(id) {
  editTargetId.value = id
  showEditModal.value = true
}

async function onTargetSaved() {
  showEditModal.value = false
  await loadData()
  success.value = 'Target updated'
}

onMounted(() => {
  loadData()
  loadSavedSearches()
})
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h2>Search</h2>
    </div>

    <div v-if="error" class="error-msg">{{ error }}</div>
    <div v-if="success" class="success-msg" @click="success = ''">{{ success }}</div>

    <!-- Search bar -->
    <div class="search-bar">
      <div class="search-input-wrap">
        <input
          v-model="searchText"
          type="text"
          class="search-input"
          placeholder="Search by name, host, or IP..."
          autofocus
        />
      </div>
      <div class="search-filters">
        <select v-model="filterProject" class="filter-select">
          <option value="">All Projects</option>
          <option v-for="p in projectOptions" :key="p.id" :value="p.value">{{ p.value }}</option>
        </select>
        <select v-model="filterLocation" class="filter-select">
          <option value="">All Locations</option>
          <option v-for="l in locationOptions" :key="l.id" :value="l.value">{{ l.value }}</option>
        </select>
      </div>
      <button v-if="hasActiveFilter && !showSaveName" class="btn btn-sm btn-save-search" @click="showSaveName = true" title="Save this search">Save</button>
      <div v-if="showSaveName" class="save-search-inline">
        <input v-model="saveName" type="text" class="save-search-input" placeholder="Search name..." @keyup.enter="saveCurrentSearch" @keyup.escape="showSaveName = false" autofocus />
        <button class="btn btn-sm btn-primary" @click="saveCurrentSearch" :disabled="!saveName.trim()">Save</button>
        <button class="btn btn-sm" @click="showSaveName = false">Cancel</button>
      </div>
    </div>

    <!-- Saved searches chip bar -->
    <div v-if="savedSearches.length > 0" class="saved-searches-bar">
      <span class="saved-label">Saved:</span>
      <div v-for="s in savedSearches" :key="s.id" class="saved-chip" @click="applySavedSearch(s)">
        <span class="saved-chip-name">{{ s.name }}</span>
        <span class="saved-chip-x" @click.stop="deleteSavedSearch(s.id)" title="Remove">&times;</span>
      </div>
    </div>

    <!-- Results -->
    <div class="card">
      <div v-if="loading" class="text-muted" style="padding: 1rem;">Loading...</div>
      <div v-else-if="!hasActiveFilter" class="text-muted" style="padding: 1.5rem;">Type a name, host, or IP to search across {{ targets.length }} targets.</div>
      <div v-else-if="filteredTargets.length === 0" class="text-muted" style="padding: 1rem;">No targets match your search.</div>

      <template v-else>
        <div class="results-count">{{ filteredTargets.length }} of {{ targets.length }} targets</div>
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Host</th>
              <th>State</th>
              <th>Category</th>
              <th>Project</th>
              <th>Location</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <template v-for="t in filteredTargets" :key="t.id">
              <tr :class="{ 'row-expanded': expandedTargetId === t.id }" @click="toggleExpand(t.id)" style="cursor: pointer;">
                <td>
                  <span class="expand-icon">{{ expandedTargetId === t.id ? '&#9660;' : '&#9654;' }}</span>
                  {{ t.name }}
                </td>
                <td class="host-cell">{{ t.host }}</td>
                <td>
                  <span v-if="isTargetPaused(t)" class="badge badge-paused">paused</span>
                  <span v-else-if="t.state" :class="['badge', stateClass(t.state?.current_state)]">
                    {{ t.state?.current_state || '—' }}
                  </span>
                  <span v-else class="text-muted">—</span>
                </td>
                <td><span :class="['badge', categoryClass(t.category)]">{{ t.category }}</span></td>
                <td>
                  <span v-if="t.project" class="badge badge-tag-project">{{ t.project }}</span>
                  <span v-else class="text-muted">&mdash;</span>
                </td>
                <td>
                  <span v-if="t.location" class="badge badge-tag-location">{{ t.location }}</span>
                  <span v-else class="text-muted">&mdash;</span>
                </td>
                <td class="actions" @click.stop>
                  <template v-if="auth.isOperator">
                    <button v-if="isTargetPaused(t)" class="btn btn-sm btn-unpause" @click="unpauseTarget(t.id)">Unpause</button>
                    <button v-else class="btn btn-sm btn-pause" @click="pauseTarget(t.id)">Pause</button>
                    <button class="btn btn-sm" @click="openEdit(t.id)">Edit</button>
                    <button class="btn btn-sm btn-danger" @click="deleteTarget(t.id)">Delete</button>
                  </template>
                </td>
              </tr>

              <!-- Expanded detail -->
              <tr v-if="expandedTargetId === t.id">
                <td colspan="7" class="detail-panel">
                  <div v-if="expandLoading" class="text-muted" style="padding: 1rem 1rem 1rem 2.5rem;">Loading details...</div>
                  <div v-else-if="expandedDetail" class="detail-card">
                    <!-- Info grid -->
                    <div class="detail-grid">
                      <div v-if="expandedDetail.description" class="detail-field">
                        <span class="detail-label">Description</span>
                        <span>{{ expandedDetail.description }}</span>
                      </div>
                      <div v-if="expandedDetail.notes" class="detail-field">
                        <span class="detail-label">Notes</span>
                        <span class="detail-pre">{{ expandedDetail.notes }}</span>
                      </div>
                      <div class="detail-field">
                        <span class="detail-label">Contacts</span>
                        <span v-if="expandedDetail.contacts" class="detail-pre">{{ expandedDetail.contacts }}</span>
                        <span v-else class="text-muted">None set</span>
                      </div>
                    </div>

                    <!-- Alert Recipients -->
                    <div class="detail-section">
                      <span class="detail-label">Alert Recipients</span>
                      <div v-if="expandedDetail.recipient_ids && expandedDetail.recipient_ids.length > 0" class="detail-recipients-list">
                        <span v-for="name in recipientNames(expandedDetail.recipient_ids)" :key="name" class="badge badge-recipient">{{ name }}</span>
                      </div>
                      <span v-else class="text-muted">No recipients configured</span>
                    </div>

                    <!-- Timestamps -->
                    <div class="detail-timestamps">
                      <span>Created: {{ formatDate(expandedDetail.created_at) }}</span>
                      <span>Updated: {{ formatDate(expandedDetail.updated_at) }}</span>
                    </div>

                    <!-- Checks table -->
                    <div class="detail-checks">
                      <span class="detail-label">Checks</span>
                      <div v-if="!expandedDetail.conditions || expandedDetail.conditions.length === 0" class="text-muted" style="margin-top: 0.25rem;">
                        No checks configured.
                      </div>
                      <table v-else class="checks-table">
                        <thead>
                          <tr>
                            <th>Name</th>
                            <th>Type</th>
                            <th>Interval</th>
                            <th>Enabled</th>
                          </tr>
                        </thead>
                        <tbody>
                          <tr v-for="c in expandedDetail.conditions" :key="c.check_id">
                            <td>{{ c.check_name }}</td>
                            <td>
                              <span class="badge badge-type">{{ c.check_type }}</span>
                              <span v-if="c.check_type === expandedDetail.preferred_check_type" class="badge badge-primary-check" title="Primary check used for SLA &amp; dashboard status">PRIMARY</span>
                            </td>
                            <td>{{ formatInterval(c.interval_s) }}</td>
                            <td><span :class="['badge', c.enabled !== false ? 'badge-active' : 'badge-suspended']">{{ c.enabled !== false ? 'yes' : 'no' }}</span></td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </template>
    </div>

    <!-- Target edit modal -->
    <TargetEditModal :show="showEditModal" :target-id="editTargetId" @close="showEditModal = false" @saved="onTargetSaved" />

    <!-- Delete confirmation modal -->
    <div v-if="showDeleteConfirm" class="modal-overlay" @click.self="showDeleteConfirm = false">
      <div class="modal-card">
        <h3>Delete Target</h3>
        <p>Delete this target and all its checks? This cannot be undone.</p>
        <div class="form-actions">
          <button class="btn btn-danger" @click="confirmDelete">Delete</button>
          <button class="btn" @click="showDeleteConfirm = false">Cancel</button>
        </div>
      </div>
    </div>

    <!-- Pause confirmation modal -->
    <div v-if="showPauseConfirm" class="modal-overlay" @click.self="showPauseConfirm = false">
      <div class="modal-card">
        <h3>Pause Target</h3>
        <p>Testing will stop for this target. SLA will not be affected during pause. Are you sure?</p>
        <div class="form-actions">
          <button class="btn btn-pause" @click="confirmPause">Pause</button>
          <button class="btn" @click="showPauseConfirm = false">Cancel</button>
        </div>
      </div>
    </div>

    <!-- Unpause confirmation modal -->
    <div v-if="showUnpauseConfirm" class="modal-overlay" @click.self="showUnpauseConfirm = false">
      <div class="modal-card">
        <h3>Unpause Target</h3>
        <p>Testing will resume immediately for all checks. Continue?</p>
        <div class="form-actions">
          <button class="btn btn-primary" @click="confirmUnpause">Unpause</button>
          <button class="btn" @click="showUnpauseConfirm = false">Cancel</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Search bar */
.search-bar {
  display: flex;
  gap: 1rem;
  margin-bottom: 1.25rem;
  align-items: center;
  flex-wrap: wrap;
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 0.75rem 1rem;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.06);
}
.search-input-wrap {
  flex: 1;
  min-width: 200px;
}
.search-input {
  width: 100%;
  padding: 0.6rem 0.85rem;
  border: 2px solid #cbd5e1;
  border-radius: 8px;
  font-size: 0.95rem;
  background: #f8fafc;
  color: var(--text);
  transition: border-color 0.15s, box-shadow 0.15s;
}
.search-input:focus {
  outline: none;
  border-color: #ea580c;
  background: #fff;
  box-shadow: 0 0 0 3px rgba(234, 88, 12, 0.12);
}
.search-filters {
  display: flex;
  gap: 0.5rem;
}
.filter-select {
  padding: 0.6rem 0.6rem;
  border-radius: 8px;
  border: 2px solid #cbd5e1;
  font-size: 0.88rem;
  background: #f8fafc;
  color: var(--text);
  cursor: pointer;
  transition: border-color 0.15s;
}
.filter-select:focus {
  outline: none;
  border-color: #ea580c;
}

/* Save search */
.btn-save-search {
  color: #4338ca;
  border-color: #c7d2fe;
  white-space: nowrap;
}
.btn-save-search:hover { background: #e0e7ff; }
.save-search-inline {
  display: flex;
  gap: 0.4rem;
  align-items: center;
}
.save-search-input {
  padding: 0.4rem 0.6rem;
  border: 2px solid #cbd5e1;
  border-radius: 6px;
  font-size: 0.85rem;
  width: 140px;
  background: #f8fafc;
  color: var(--text);
}
.save-search-input:focus {
  outline: none;
  border-color: #6366f1;
}

/* Saved searches chip bar */
.saved-searches-bar {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  flex-wrap: wrap;
  margin-bottom: 1rem;
  padding: 0.5rem 0;
}
.saved-label {
  font-size: 0.75rem;
  font-weight: 600;
  color: #94a3b8;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.saved-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.3rem 0.6rem;
  background: #e0e7ff;
  color: #3730a3;
  border-radius: 20px;
  font-size: 0.8rem;
  font-weight: 500;
  cursor: pointer;
  transition: background 0.15s, box-shadow 0.15s;
  user-select: none;
}
.saved-chip:hover {
  background: #c7d2fe;
  box-shadow: 0 1px 3px rgba(99, 102, 241, 0.2);
}
.saved-chip-name {
  max-width: 150px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.saved-chip-x {
  font-size: 1rem;
  line-height: 1;
  color: #6366f1;
  opacity: 0.5;
  cursor: pointer;
  padding: 0 0.1rem;
}
.saved-chip-x:hover {
  opacity: 1;
  color: #dc2626;
}

/* Results count */
.results-count {
  padding: 0.5rem 1rem;
  font-size: 0.8rem;
  color: #64748b;
  border-bottom: 1px solid var(--border);
}

/* Table tweaks */
.host-cell {
  font-family: monospace;
  font-size: 0.85rem;
}
.row-expanded > td { background: #e8ecf1; }

.expand-icon {
  display: inline-block;
  width: 1rem;
  font-size: 0.7rem;
  color: #94a3b8;
}

/* Badges — same as TargetsView */
.badge-tag-project {
  background: #e0e7ff;
  color: #3730a3;
}
.badge-tag-location {
  background: #fef3c7;
  color: #92400e;
}
.badge-unknown {
  background: #f1f5f9;
  color: #64748b;
}
.badge-paused {
  background: #e0e7ff;
  color: #4338ca;
}
.badge-cat-security {
  background: #ede9fe;
  color: #6d28d9;
}
.badge-cat-network {
  background: #dbeafe;
  color: #1d4ed8;
}
.badge-cat-server {
  background: #fce7f3;
  color: #9d174d;
}
.badge-cat-physical {
  background: #fef3c7;
  color: #92400e;
}
.badge-cat-other {
  background: #e5e7eb;
  color: #374151;
}
.badge-type {
  background: #e0e7ff;
  color: #3730a3;
  font-size: 0.7rem;
  text-transform: uppercase;
}
.badge-primary-check {
  background: #fef3c7;
  color: #92400e;
  font-size: 0.6rem;
  font-weight: 700;
  text-transform: uppercase;
}

/* Action buttons */
.btn-pause {
  color: #4338ca;
  border-color: #c7d2fe;
}
.btn-pause:hover { background: #e0e7ff; }
.btn-unpause {
  color: #166534;
  border-color: #bbf7d0;
}
.btn-unpause:hover { background: #dcfce7; }

/* Expanded detail panel — matches TargetsView checks-panel */
.detail-panel {
  padding: 0;
  border-left: 3px solid #6366f1;
  background: #eef2ff;
  box-shadow: inset 0 2px 8px rgba(0, 0, 0, 0.04);
}
.detail-card {
  padding: 1.25rem 1.5rem 1.25rem 2.5rem;
}
.detail-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.75rem 2rem;
  margin-bottom: 1rem;
}
.detail-field {
  min-width: 0;
}
.detail-label {
  display: block;
  font-size: 0.7rem;
  font-weight: 700;
  color: #4338ca;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 0.2rem;
}
.detail-pre {
  white-space: pre-wrap;
  font-size: 0.85rem;
}
.detail-section {
  margin-bottom: 1rem;
}
.detail-recipients-list {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
  margin-top: 0.25rem;
}
.badge-recipient {
  background: #dbeafe;
  color: #1e40af;
  font-size: 0.78rem;
}
.detail-timestamps {
  display: flex;
  gap: 1.5rem;
  font-size: 0.78rem;
  color: #64748b;
  margin-bottom: 1rem;
  padding-top: 0.5rem;
  border-top: 1px solid #c7d2fe;
}
.detail-checks {
  padding-top: 0.5rem;
  border-top: 1px solid #c7d2fe;
}
.checks-table {
  font-size: 0.8rem;
  margin-top: 0.25rem;
}
.checks-table th { font-size: 0.7rem; }

/* Modal */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.3);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  z-index: 100;
  overflow-y: auto;
  padding: 2rem 1rem;
}
</style>
