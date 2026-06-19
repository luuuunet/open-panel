<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import api, { resolveApiError } from '@/api'
import FileCodeEditor from '@/components/FileCodeEditor.vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Plus,
  RefreshRight,
  VideoPlay,
  VideoPause,
  Document,
  FolderOpened,
  Delete,
  Edit,
  Download,
  CopyDocument,
  Box,
  Search,
} from '@element-plus/icons-vue'

interface ComposeServiceInfo {
  name: string
  container: string
  image: string
  state: string
  status: string
  ports: string
}

interface ComposeProject {
  id: number
  name: string
  path: string
  status: string
  live_status?: string
  compose_file?: string
  services?: ComposeServiceInfo[]
  service_count?: number
}

const { t } = useI18n()
const router = useRouter()

const list = ref<ComposeProject[]>([])
const templates = ref<any[]>([])
const loading = ref(false)
const actionLoading = ref<number | null>(null)
const dockerOk = ref(true)

const search = ref('')
const createVisible = ref(false)
const createMode = ref<'template' | 'import'>('template')
const creating = ref(false)
const form = ref({
  name: '',
  path: '/opt/compose/',
  scaffold: true,
  template: 'nginx',
  auto_start: true,
})

const drawerVisible = ref(false)
const activeProject = ref<ComposeProject | null>(null)

const logVisible = ref(false)
const logContent = ref('')
const logLoading = ref(false)
const logTargetId = ref<number | null>(null)

const fileVisible = ref(false)
const fileContent = ref('')
const fileTargetId = ref<number | null>(null)
const fileSaving = ref(false)

const templateMeta: Record<string, { icon: string; color: string }> = {
  nginx: { icon: 'N', color: '#009639' },
  mysql: { icon: 'DB', color: '#00758f' },
  redis: { icon: 'R', color: '#dc382d' },
  wordpress: { icon: 'WP', color: '#21759b' },
  openpanel: { icon: 'OP', color: '#6366f1' },
}

const stats = computed(() => {
  const total = list.value.length
  let running = 0
  let stopped = 0
  for (const p of list.value) {
    if (projectStatus(p) === 'running') running++
    else if (projectStatus(p) !== 'unknown') stopped++
  }
  return { total, running, stopped }
})

const filteredList = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return list.value
  return list.value.filter(
    (p) =>
      p.name.toLowerCase().includes(q) ||
      p.path.toLowerCase().includes(q) ||
      (p.services || []).some((s) => s.name.toLowerCase().includes(q) || s.image.toLowerCase().includes(q)),
  )
})

function projectStatus(row: ComposeProject) {
  return row.live_status || row.status || 'stopped'
}

function statusTagType(st: string) {
  if (st === 'running') return 'success'
  if (st === 'unknown') return 'warning'
  return 'info'
}

function templateStyle(id: string) {
  const m = templateMeta[id] || { icon: '?', color: '#909399' }
  return { background: m.color }
}

function templateIcon(id: string) {
  return templateMeta[id]?.icon || id.slice(0, 2).toUpperCase()
}

watch(
  () => form.value.name,
  (name) => {
    if (createMode.value === 'template' && name.trim()) {
      const slug = name.trim().replace(/\s+/g, '-').toLowerCase()
      form.value.path = `/opt/compose/${slug}`
    }
  },
)

watch(createMode, (mode) => {
  form.value.scaffold = mode === 'template'
  if (mode === 'import') form.value.auto_start = false
  else if (mode === 'template') form.value.auto_start = true
})

async function load() {
  loading.value = true
  try {
    const res: any = await api.get('/compose')
    list.value = res.data || []
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    loading.value = false
  }
}

async function loadDockerStatus() {
  try {
    const res: any = await api.get('/docker/status')
    dockerOk.value = !!(res.data?.installed && res.data?.daemon_ok)
  } catch {
    dockerOk.value = false
  }
}

async function loadTemplates() {
  try {
    const res: any = await api.get('/compose/templates')
    templates.value = res.data || []
  } catch {
    templates.value = [{ id: 'nginx', name: 'Nginx', description: '' }]
  }
}

function openCreate(mode: 'template' | 'import' = 'template', templateId?: string) {
  createMode.value = mode
  form.value = {
    name: '',
    path: mode === 'import' ? '/opt/compose/my-project' : '/opt/compose/',
    scaffold: mode === 'template',
    template: templateId || 'nginx',
    auto_start: mode === 'template',
  }
  createVisible.value = true
}

function quickCreateFromTemplate(tpl: any) {
  openCreate('template', tpl.id)
  form.value.name = tpl.name
  form.value.path = `/opt/compose/${tpl.id}`
}

async function handleCreate() {
  if (!form.value.name.trim()) {
    ElMessage.warning(t('compose.nameRequired'))
    return
  }
  if (!form.value.path.trim()) {
    ElMessage.warning(t('compose.pathRequired'))
    return
  }
  creating.value = true
  try {
    await api.post('/compose', {
      name: form.value.name.trim(),
      path: form.value.path.trim(),
      scaffold: form.value.scaffold,
      template: form.value.template,
      auto_start: form.value.auto_start,
    })
    ElMessage.success(t('compose.created'))
    createVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    creating.value = false
  }
}

async function refreshProject(row: ComposeProject) {
  try {
    const res: any = await api.post(`/compose/${row.id}/sync`)
    const updated = res.data
    const idx = list.value.findIndex((p) => p.id === row.id)
    if (idx >= 0 && updated) list.value[idx] = updated
    if (activeProject.value?.id === row.id && updated) activeProject.value = updated
    ElMessage.success(t('compose.synced'))
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function toggle(row: ComposeProject) {
  const next = projectStatus(row) === 'running' ? 'stopped' : 'running'
  actionLoading.value = row.id
  try {
    await api.patch(`/compose/${row.id}/toggle`, { status: next })
    ElMessage.success(next === 'running' ? t('compose.started') : t('compose.stopped'))
    await load()
    if (activeProject.value?.id === row.id) {
      const found = list.value.find((p) => p.id === row.id)
      if (found) activeProject.value = found
    }
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    actionLoading.value = null
  }
}

async function restartProject(row: ComposeProject) {
  actionLoading.value = row.id
  try {
    const res: any = await api.post(`/compose/${row.id}/restart`)
    if (res.data) {
      const idx = list.value.findIndex((p) => p.id === row.id)
      if (idx >= 0) list.value[idx] = res.data
      if (activeProject.value?.id === row.id) activeProject.value = res.data
    }
    ElMessage.success(t('compose.restarted'))
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    actionLoading.value = null
  }
}

async function handleDelete(row: ComposeProject) {
  try {
    await ElMessageBox.confirm(t('compose.deleteConfirm'), t('common.warning'), { type: 'warning' })
    await api.delete(`/compose/${row.id}`)
    ElMessage.success(t('common.deleted'))
    if (activeProject.value?.id === row.id) drawerVisible.value = false
    await load()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function showLogs(row: ComposeProject) {
  logTargetId.value = row.id
  logVisible.value = true
  await fetchLogs()
}

async function fetchLogs() {
  if (!logTargetId.value) return
  logLoading.value = true
  try {
    const res: any = await api.get(`/compose/${logTargetId.value}/logs`)
    logContent.value = res.data?.log || t('compose.noLog')
  } catch (e: any) {
    logContent.value = resolveApiError(e, t('common.failed'))
  } finally {
    logLoading.value = false
  }
}

async function pullImages(row: ComposeProject) {
  actionLoading.value = row.id
  try {
    await api.post(`/compose/${row.id}/pull`)
    ElMessage.success(t('compose.pulled'))
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    actionLoading.value = null
  }
}

async function editCompose(row: ComposeProject) {
  try {
    const res: any = await api.get(`/compose/${row.id}/compose-file`)
    fileContent.value = res.data?.content || ''
    fileTargetId.value = row.id
    fileVisible.value = true
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function saveComposeFile() {
  if (!fileTargetId.value) return
  fileSaving.value = true
  try {
    await api.put(`/compose/${fileTargetId.value}/compose-file`, { content: fileContent.value })
    ElMessage.success(t('compose.fileSaved'))
    fileVisible.value = false
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    fileSaving.value = false
  }
}

function openDrawer(row: ComposeProject) {
  activeProject.value = row
  drawerVisible.value = true
}

function openInFiles(path: string) {
  router.push({ name: 'files', query: { path } })
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(t('compose.copied'))
  } catch {
    ElMessage.error(t('common.failed'))
  }
}

onMounted(() => {
  load()
  loadTemplates()
  loadDockerStatus()
})
</script>

<template>
  <div class="compose-page">
    <div class="page-header">
      <div>
        <h2>{{ t('compose.title') }}</h2>
        <p class="page-sub">{{ t('compose.subtitle') }}</p>
      </div>
      <div class="header-actions">
        <el-button :icon="RefreshRight" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
        <el-button type="primary" :icon="Plus" @click="openCreate('template')">{{ t('compose.add') }}</el-button>
      </div>
    </div>

    <el-alert v-if="!dockerOk" type="warning" :closable="false" show-icon class="hint">{{ t('compose.dockerUnavailable') }}</el-alert>

    <div v-if="stats.total > 0" class="stats-row">
      <div class="stat-card">
        <span class="stat-value">{{ stats.total }}</span>
        <span class="stat-label">{{ t('compose.statTotal') }}</span>
      </div>
      <div class="stat-card running">
        <span class="stat-value">{{ stats.running }}</span>
        <span class="stat-label">{{ t('compose.statRunning') }}</span>
      </div>
      <div class="stat-card stopped">
        <span class="stat-value">{{ stats.stopped }}</span>
        <span class="stat-label">{{ t('compose.statStopped') }}</span>
      </div>
    </div>

    <div v-if="list.length" class="toolbar">
      <el-input v-model="search" :placeholder="t('compose.searchPlaceholder')" clearable :prefix-icon="Search" class="search-input" />
    </div>

    <!-- Empty state with quick templates -->
    <div v-if="!list.length && !loading" class="empty-wrap">
      <el-empty :description="t('compose.empty')">
        <template #default>
          <p class="empty-hint">{{ t('compose.emptyHint') }}</p>
          <div class="template-grid">
            <button
              v-for="tpl in templates"
              :key="tpl.id"
              type="button"
              class="template-card"
              @click="quickCreateFromTemplate(tpl)"
            >
              <span class="template-icon" :style="templateStyle(tpl.id)">{{ templateIcon(tpl.id) }}</span>
              <span class="template-name">{{ tpl.name }}</span>
              <span v-if="tpl.description" class="template-desc">{{ tpl.description }}</span>
            </button>
          </div>
          <div class="empty-actions">
            <el-button type="primary" @click="openCreate('template')">{{ t('compose.add') }}</el-button>
            <el-button @click="openCreate('import')">{{ t('compose.importExisting') }}</el-button>
          </div>
        </template>
      </el-empty>
    </div>

    <!-- Project cards -->
    <div v-else v-loading="loading" class="project-grid">
      <article
        v-for="row in filteredList"
        :key="row.id"
        class="project-card"
        :class="{ running: projectStatus(row) === 'running' }"
        @click="openDrawer(row)"
      >
        <div class="card-top">
          <div class="card-title">
            <el-icon class="card-icon"><Box /></el-icon>
            <span class="name">{{ row.name }}</span>
          </div>
          <el-tag :type="statusTagType(projectStatus(row))" size="small" effect="plain">
            {{ projectStatus(row) }}
          </el-tag>
        </div>
        <div class="card-path" :title="row.path">{{ row.path }}</div>
        <div v-if="row.service_count" class="card-services">
          {{ t('compose.serviceCount', { n: row.service_count }) }}
        </div>
        <div class="card-actions" @click.stop>
          <el-tooltip :content="projectStatus(row) === 'running' ? t('common.stop') : t('common.start')" placement="top">
            <el-button
              circle
              size="small"
              :type="projectStatus(row) === 'running' ? 'warning' : 'success'"
              :icon="projectStatus(row) === 'running' ? VideoPause : VideoPlay"
              :loading="actionLoading === row.id"
              @click="toggle(row)"
            />
          </el-tooltip>
          <el-tooltip :content="t('compose.logs')" placement="top">
            <el-button circle size="small" :icon="Document" @click="showLogs(row)" />
          </el-tooltip>
          <el-tooltip :content="t('compose.editFile')" placement="top">
            <el-button circle size="small" :icon="Edit" @click="editCompose(row)" />
          </el-tooltip>
          <el-tooltip :content="t('compose.openFolder')" placement="top">
            <el-button circle size="small" :icon="FolderOpened" @click="openInFiles(row.path)" />
          </el-tooltip>
        </div>
      </article>
    </div>

    <el-empty v-if="list.length && !filteredList.length && !loading" :description="t('compose.noMatch')" />

    <!-- Create dialog -->
    <el-dialog v-model="createVisible" :title="t('compose.addTitle')" width="640px" destroy-on-close>
      <div class="create-tabs">
        <el-radio-group v-model="createMode">
          <el-radio-button value="template">{{ t('compose.modeTemplate') }}</el-radio-button>
          <el-radio-button value="import">{{ t('compose.modeImport') }}</el-radio-button>
        </el-radio-group>
      </div>

      <el-form :model="form" label-width="100px" class="create-form">
        <el-form-item :label="t('common.name')" required>
          <el-input v-model="form.name" :placeholder="t('compose.namePlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('common.path')" required>
          <el-input v-model="form.path" :placeholder="t('compose.pathPlaceholder')" />
        </el-form-item>

        <template v-if="createMode === 'template'">
          <el-form-item :label="t('compose.template')">
            <div class="template-picker">
              <button
                v-for="tpl in templates"
                :key="tpl.id"
                type="button"
                class="template-pick"
                :class="{ active: form.template === tpl.id }"
                @click="form.template = tpl.id"
              >
                <span class="template-icon sm" :style="templateStyle(tpl.id)">{{ templateIcon(tpl.id) }}</span>
                <span>{{ tpl.name }}</span>
              </button>
            </div>
          </el-form-item>
          <el-form-item :label="t('compose.autoStart')">
            <el-switch v-model="form.auto_start" />
            <span class="form-hint-inline">{{ t('compose.autoStartHint') }}</span>
          </el-form-item>
        </template>

        <template v-else>
          <el-alert type="info" :closable="false" show-icon>{{ t('compose.importHint') }}</el-alert>
          <el-form-item :label="t('compose.autoStart')">
            <el-switch v-model="form.auto_start" />
          </el-form-item>
        </template>
      </el-form>

      <template #footer>
        <el-button @click="createVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="creating" @click="handleCreate">{{ t('common.create') }}</el-button>
      </template>
    </el-dialog>

    <!-- Detail drawer -->
    <el-drawer v-model="drawerVisible" :title="activeProject?.name || t('compose.title')" size="520px" destroy-on-close>
      <template v-if="activeProject">
        <div class="drawer-status">
          <el-tag :type="statusTagType(projectStatus(activeProject))" effect="dark">
            {{ projectStatus(activeProject) }}
          </el-tag>
          <span v-if="activeProject.service_count" class="drawer-meta">
            {{ t('compose.serviceCount', { n: activeProject.service_count }) }}
          </span>
        </div>

        <div class="drawer-section">
          <div class="section-label">{{ t('common.path') }}</div>
          <div class="path-row">
            <code class="path-text">{{ activeProject.path }}</code>
            <el-button text size="small" :icon="CopyDocument" @click="copyText(activeProject.path)" />
            <el-button text size="small" :icon="FolderOpened" @click="openInFiles(activeProject.path)" />
          </div>
          <div v-if="activeProject.compose_file" class="path-sub">{{ activeProject.compose_file }}</div>
        </div>

        <div class="drawer-actions">
          <el-button
            :type="projectStatus(activeProject) === 'running' ? 'warning' : 'success'"
            :icon="projectStatus(activeProject) === 'running' ? VideoPause : VideoPlay"
            :loading="actionLoading === activeProject.id"
            @click="toggle(activeProject)"
          >
            {{ projectStatus(activeProject) === 'running' ? t('common.stop') : t('common.start') }}
          </el-button>
          <el-button :icon="RefreshRight" :loading="actionLoading === activeProject.id" @click="restartProject(activeProject)">
            {{ t('compose.restart') }}
          </el-button>
          <el-button :icon="Download" :loading="actionLoading === activeProject.id" @click="pullImages(activeProject)">
            {{ t('compose.pull') }}
          </el-button>
          <el-button :icon="Document" @click="showLogs(activeProject)">{{ t('compose.logs') }}</el-button>
          <el-button :icon="Edit" @click="editCompose(activeProject)">{{ t('compose.editFile') }}</el-button>
          <el-button :icon="RefreshRight" @click="refreshProject(activeProject)">{{ t('common.refresh') }}</el-button>
          <el-button type="danger" plain :icon="Delete" @click="handleDelete(activeProject)">{{ t('common.delete') }}</el-button>
        </div>

        <div v-if="activeProject.services?.length" class="drawer-section">
          <div class="section-label">{{ t('compose.services') }}</div>
          <el-table :data="activeProject.services" size="small" stripe>
            <el-table-column prop="name" :label="t('common.name')" width="90" />
            <el-table-column prop="image" :label="t('docker.image')" min-width="120" show-overflow-tooltip />
            <el-table-column prop="state" :label="t('common.status')" width="80">
              <template #default="{ row: svc }">
                <el-tag :type="svc.state === 'running' ? 'success' : 'info'" size="small">{{ svc.state }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="ports" :label="t('common.port')" min-width="100" show-overflow-tooltip />
          </el-table>
        </div>
        <el-empty v-else :description="t('compose.noServices')" :image-size="64" />
      </template>
    </el-drawer>

    <!-- Logs -->
    <el-dialog v-model="logVisible" :title="t('compose.logs')" width="800px" destroy-on-close>
      <div class="log-toolbar">
        <el-button size="small" :icon="RefreshRight" :loading="logLoading" @click="fetchLogs">{{ t('common.refresh') }}</el-button>
      </div>
      <pre v-loading="logLoading" class="log-box">{{ logContent }}</pre>
    </el-dialog>

    <!-- YAML editor -->
    <el-dialog v-model="fileVisible" :title="t('compose.editFile')" width="860px" destroy-on-close class="file-dialog">
      <div class="editor-wrap">
        <FileCodeEditor v-model="fileContent" path="docker-compose.yml" />
      </div>
      <template #footer>
        <el-button @click="fileVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="fileSaving" @click="saveComposeFile">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.compose-page { padding-bottom: 24px; }
.page-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 16px; flex-wrap: wrap; }
.page-header h2 { margin: 0 0 4px; }
.page-sub { margin: 0; font-size: 13px; color: var(--el-text-color-secondary); }
.header-actions { display: flex; gap: 8px; flex-shrink: 0; }
.hint { margin-bottom: 16px; }

.stats-row { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; margin-bottom: 16px; max-width: 480px; }
.stat-card { background: var(--el-fill-color-light); border-radius: 10px; padding: 12px 16px; display: flex; flex-direction: column; gap: 2px; }
.stat-card.running { border-left: 3px solid var(--el-color-success); }
.stat-card.stopped { border-left: 3px solid var(--el-text-color-placeholder); }
.stat-value { font-size: 22px; font-weight: 600; line-height: 1.2; }
.stat-label { font-size: 12px; color: var(--el-text-color-secondary); }

.toolbar { margin-bottom: 16px; }
.search-input { max-width: 320px; }

.empty-wrap { padding: 24px 0; }
.empty-hint { margin: 0 0 20px; color: var(--el-text-color-secondary); font-size: 14px; }
.empty-actions { display: flex; gap: 8px; justify-content: center; margin-top: 20px; }

.template-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 12px; max-width: 720px; margin: 0 auto; }
.template-card {
  display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 16px 12px;
  border: 1px solid var(--el-border-color-lighter); border-radius: 12px; background: var(--el-bg-color);
  cursor: pointer; transition: border-color .2s, box-shadow .2s; text-align: center;
}
.template-card:hover { border-color: var(--el-color-primary); box-shadow: 0 4px 12px rgba(0,0,0,.06); }
.template-icon {
  width: 40px; height: 40px; border-radius: 10px; color: #fff; font-weight: 700; font-size: 14px;
  display: flex; align-items: center; justify-content: center;
}
.template-icon.sm { width: 28px; height: 28px; font-size: 11px; border-radius: 6px; }
.template-name { font-weight: 600; font-size: 14px; }
.template-desc { font-size: 11px; color: var(--el-text-color-secondary); line-height: 1.3; }

.project-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }
.project-card {
  border: 1px solid var(--el-border-color-lighter); border-radius: 12px; padding: 16px;
  background: var(--el-bg-color); cursor: pointer; transition: border-color .2s, box-shadow .2s;
}
.project-card:hover { border-color: var(--el-color-primary-light-5); box-shadow: 0 4px 16px rgba(0,0,0,.06); }
.project-card.running { border-left: 3px solid var(--el-color-success); }
.card-top { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 8px; }
.card-title { display: flex; align-items: center; gap: 8px; min-width: 0; }
.card-icon { color: var(--el-color-primary); flex-shrink: 0; }
.name { font-weight: 600; font-size: 15px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.card-path { font-size: 12px; color: var(--el-text-color-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; margin-bottom: 8px; }
.card-services { font-size: 12px; color: var(--el-text-color-regular); margin-bottom: 12px; }
.card-actions { display: flex; gap: 4px; flex-wrap: wrap; }

.create-tabs { margin-bottom: 16px; }
.create-form { margin-top: 8px; }
.template-picker { display: flex; flex-wrap: wrap; gap: 8px; }
.template-pick {
  display: inline-flex; align-items: center; gap: 6px; padding: 6px 12px;
  border: 1px solid var(--el-border-color); border-radius: 8px; background: var(--el-fill-color-blank);
  cursor: pointer; font-size: 13px; transition: border-color .2s;
}
.template-pick.active { border-color: var(--el-color-primary); color: var(--el-color-primary); }
.form-hint-inline { margin-left: 8px; font-size: 12px; color: var(--el-text-color-secondary); }

.drawer-status { display: flex; align-items: center; gap: 12px; margin-bottom: 20px; }
.drawer-meta { font-size: 13px; color: var(--el-text-color-secondary); }
.drawer-section { margin-bottom: 20px; }
.section-label { font-size: 12px; font-weight: 600; color: var(--el-text-color-secondary); margin-bottom: 8px; text-transform: uppercase; letter-spacing: .04em; }
.path-row { display: flex; align-items: center; gap: 4px; }
.path-text { flex: 1; font-size: 12px; word-break: break-all; background: var(--el-fill-color-light); padding: 6px 8px; border-radius: 6px; }
.path-sub { margin-top: 6px; font-size: 11px; color: var(--el-text-color-placeholder); }
.drawer-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 24px; }

.log-toolbar { margin-bottom: 8px; }
.log-box {
  max-height: 420px; overflow: auto; background: #1e1e1e; color: #d4d4d4;
  padding: 12px; border-radius: 6px; font-size: 12px; line-height: 1.5; white-space: pre-wrap; word-break: break-all; margin: 0;
}
.editor-wrap { height: 420px; border: 1px solid var(--el-border-color); border-radius: 6px; overflow: hidden; }

@media (max-width: 640px) {
  .stats-row { grid-template-columns: 1fr; max-width: none; }
  .project-grid { grid-template-columns: 1fr; }
}
</style>
