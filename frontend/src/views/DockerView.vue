<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import api, { resolveApiError } from '@/api'
import SoftwareInstallLogDialog from '@/components/SoftwareInstallLogDialog.vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Setting, Document, RefreshRight, Delete, VideoPlay, VideoPause, Link, CopyDocument, FolderOpened } from '@element-plus/icons-vue'

interface DockerStatus {
  installed: boolean
  running: boolean
  version: string
  daemon_ok: boolean
}

interface PortMapping {
  host_ip: string
  host_port: string
  container_port: string
  protocol: string
}

interface MountMapping {
  type: string
  source: string
  destination: string
  read_only: boolean
}

interface ContainerDetail {
  id: string
  name: string
  image: string
  status: string
  ports: PortMapping[]
  env: string[]
  mounts: MountMapping[]
  networks: string[]
  restart_policy: string
  command: string[]
  working_dir: string
}

interface DockerVolume {
  name: string
  driver: string
  mountpoint: string
  containers?: string[]
  in_use?: boolean
  category?: string
}

interface DockerNetworkEndpoint {
  name: string
  ipv4: string
}

interface DockerNetwork {
  id: string
  name: string
  driver: string
  scope: string
  subnet?: string
  gateway?: string
  endpoints?: DockerNetworkEndpoint[]
  container_count?: number
  in_use?: boolean
  is_system?: boolean
}

const { t } = useI18n()
const router = useRouter()
const containers = ref<any[]>([])
const images = ref<any[]>([])
const volumes = ref<any[]>([])
const networks = ref<any[]>([])
const status = ref<DockerStatus | null>(null)
const activeTab = ref('containers')
const loading = ref(false)
const installing = ref(false)
const uninstalling = ref(false)
const actionLoading = ref<string | null>(null)

const installLogVisible = ref(false)
const installLogKey = ref('docker')
const installLogName = ref('Docker')
const installTrigger = ref(false)

const logsVisible = ref(false)
const logsContent = ref('')
const logsTitle = ref('')
const logsLoading = ref(false)

const detailVisible = ref(false)
const detailLoading = ref(false)
const detailSaving = ref(false)
const detail = ref<ContainerDetail | null>(null)
const editPorts = ref<PortMapping[]>([])
const editEnvText = ref('')
const editRestart = ref('no')

const createVisible = ref(false)
const createSaving = ref(false)
const createForm = ref({
  name: '',
  image: '',
  ports_text: '',
  env_text: '',
  mounts_text: '',
  restart_policy: 'unless-stopped',
  command: '',
})

const pullVisible = ref(false)
const pullImage = ref('')
const pullLoading = ref(false)

const volumeCreateVisible = ref(false)
const volumeName = ref('')
const volumeDriver = ref('')
const volumeCreating = ref(false)
const volumeSearch = ref('')
const volumeViewMode = ref<'board' | 'table'>('board')

const networkCreateVisible = ref(false)
const networkName = ref('')
const networkDriver = ref('bridge')
const networkSubnet = ref('')
const networkGateway = ref('')
const networkCreating = ref(false)
const networkSearch = ref('')
const networkViewMode = ref<'board' | 'table'>('board')
const networkDrawerVisible = ref(false)
const activeNetwork = ref<DockerNetwork | null>(null)

const domainVisible = ref(false)
const domainSaving = ref(false)
const domainContainer = ref<any>(null)
const domainInput = ref('')
const domainHostPort = ref<number | null>(null)
const domainPortOptions = ref<number[]>([])

const dockerReady = computed(() => status.value?.installed && status.value?.daemon_ok)

const volumeStats = computed(() => {
  const list = volumes.value as DockerVolume[]
  return {
    total: list.length,
    inUse: list.filter((v) => v.in_use).length,
    unused: list.filter((v) => !v.in_use).length,
    panel: list.filter((v) => v.category === 'panel').length,
  }
})

const filteredVolumes = computed(() => {
  const q = volumeSearch.value.trim().toLowerCase()
  const list = volumes.value as DockerVolume[]
  if (!q) return list
  return list.filter((v) =>
    v.name?.toLowerCase().includes(q)
    || v.mountpoint?.toLowerCase().includes(q)
    || v.driver?.toLowerCase().includes(q)
    || (v.containers || []).some((c) => c.toLowerCase().includes(q)),
  )
})

const volumeColumns = computed(() => {
  const list = filteredVolumes.value
  return [
    {
      key: 'panel',
      title: t('docker.volumePanel'),
      volumes: list.filter((v) => v.category === 'panel'),
    },
    {
      key: 'in_use',
      title: t('docker.volumeInUse'),
      volumes: list.filter((v) => v.in_use && v.category !== 'panel'),
    },
    {
      key: 'unused',
      title: t('docker.volumeUnused'),
      volumes: list.filter((v) => !v.in_use && v.category !== 'panel'),
    },
  ]
})

function volumeDisplayName(vol: DockerVolume) {
  if (vol.category === 'panel' && vol.name.startsWith('owpanel-')) {
    return vol.name.slice('owpanel-'.length)
  }
  return vol.name
}

function truncateText(text: string, max = 36) {
  if (!text || text.length <= max) return text
  return `${text.slice(0, max)}…`
}

async function copyToClipboard(text: string) {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(t('docker.pathCopied'))
  } catch {
    ElMessage.error(t('common.failed'))
  }
}

function openVolumeInFiles(vol: DockerVolume) {
  if (!vol.mountpoint) return
  router.push({ name: 'files', query: { path: vol.mountpoint } })
}

const networkStats = computed(() => {
  const list = networks.value as DockerNetwork[]
  return {
    total: list.length,
    system: list.filter((n) => n.is_system).length,
    inUse: list.filter((n) => !n.is_system && n.in_use).length,
    unused: list.filter((n) => !n.is_system && !n.in_use).length,
  }
})

const filteredNetworks = computed(() => {
  const q = networkSearch.value.trim().toLowerCase()
  const list = networks.value as DockerNetwork[]
  if (!q) return list
  return list.filter((n) =>
    n.name?.toLowerCase().includes(q)
    || n.driver?.toLowerCase().includes(q)
    || n.subnet?.toLowerCase().includes(q)
    || n.id?.toLowerCase().includes(q)
    || (n.endpoints || []).some((e) => e.name.toLowerCase().includes(q) || e.ipv4.includes(q)),
  )
})

const networkColumns = computed(() => {
  const list = filteredNetworks.value
  return [
    {
      key: 'system',
      title: t('docker.networkSystem'),
      networks: list.filter((n) => n.is_system),
    },
    {
      key: 'in_use',
      title: t('docker.networkInUse'),
      networks: list.filter((n) => !n.is_system && n.in_use),
    },
    {
      key: 'unused',
      title: t('docker.networkUnused'),
      networks: list.filter((n) => !n.is_system && !n.in_use),
    },
  ]
})

function openNetworkDrawer(net: DockerNetwork) {
  activeNetwork.value = net
  networkDrawerVisible.value = true
}

function networkDriverLabel(driver: string) {
  return driver || 'bridge'
}

async function loadStatus() {
  try {
    const res: any = await api.get('/docker/status')
    status.value = res.data || null
  } catch {
    status.value = null
  }
}

async function load() {
  loading.value = true
  try {
    await loadStatus()
    if (!status.value?.installed) {
      containers.value = []
      images.value = []
      volumes.value = []
      networks.value = []
      return
    }
    const [c, i, v, n]: any[] = await Promise.all([
      api.get('/docker/containers'),
      api.get('/docker/images'),
      api.get('/docker/volumes'),
      api.get('/docker/networks'),
    ])
    containers.value = c.data || []
    images.value = i.data || []
    volumes.value = v.data || []
    networks.value = n.data || []
  } finally {
    loading.value = false
  }
}

async function autoDetectInstall() {
  installing.value = true
  try {
    await api.post('/software/docker/install', { version: '' }, { timeout: 120000 })
    ElMessage.success(t('docker.installStarted'))
    installTrigger.value = false
    installLogVisible.value = true
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('software.installFailed')))
  } finally {
    installing.value = false
  }
}

async function onInstallDone(payload: { success: boolean }) {
  installTrigger.value = false
  await load()
  if (payload.success) ElMessage.success(t('software.installSuccessShort'))
}

async function uninstallDocker() {
  await ElMessageBox.confirm(t('docker.uninstallConfirm'), t('common.warning'), { type: 'warning' })
  uninstalling.value = true
  try {
    await api.post('/software/docker/uninstall')
    ElMessage.success(t('software.uninstallSuccess'))
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    uninstalling.value = false
  }
}

async function startContainer(id: string) {
  actionLoading.value = id
  try {
    await api.post(`/docker/containers/${id}/start`)
    ElMessage.success(t('common.success'))
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    actionLoading.value = null
  }
}

async function stopContainer(id: string) {
  actionLoading.value = id
  try {
    await api.post(`/docker/containers/${id}/stop`)
    ElMessage.success(t('common.success'))
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    actionLoading.value = null
  }
}

async function restartContainer(id: string) {
  actionLoading.value = id
  try {
    await api.post(`/docker/containers/${id}/restart`)
    ElMessage.success(t('docker.restarted'))
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    actionLoading.value = null
  }
}

async function removeContainer(row: any) {
  await ElMessageBox.confirm(t('docker.removeConfirm', { name: row.name }), t('common.warning'), { type: 'warning' })
  actionLoading.value = row.id
  try {
    await api.delete(`/docker/containers/${row.id}`)
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    actionLoading.value = null
  }
}

async function openLogs(row: any) {
  logsTitle.value = row.name
  logsVisible.value = true
  logsLoading.value = true
  logsContent.value = ''
  try {
    const res: any = await api.get(`/docker/containers/${row.id}/logs`, { params: { tail: 500 } })
    logsContent.value = res.data?.content || t('docker.noLogs')
  } catch (e: any) {
    logsContent.value = resolveApiError(e, t('common.failed'))
  } finally {
    logsLoading.value = false
  }
}

async function openDetail(row: any) {
  detailVisible.value = true
  detailLoading.value = true
  detail.value = null
  try {
    const res: any = await api.get(`/docker/containers/${row.id}`)
    detail.value = { ...res.data, image: row.image || res.data.image }
    editPorts.value = (res.data.ports || []).map((p: PortMapping) => ({ ...p, protocol: p.protocol || 'tcp' }))
    if (!editPorts.value.length) editPorts.value = [{ host_ip: '', host_port: '', container_port: '', protocol: 'tcp' }]
    editEnvText.value = (res.data.env || []).join('\n')
    editRestart.value = res.data.restart_policy || 'no'
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
    detailVisible.value = false
  } finally {
    detailLoading.value = false
  }
}

function addPortRow() {
  editPorts.value.push({ host_ip: '', host_port: '', container_port: '', protocol: 'tcp' })
}

function removePortRow(i: number) {
  editPorts.value.splice(i, 1)
}

function parsePortsText(text: string): PortMapping[] {
  return text.split('\n').map((line) => line.trim()).filter(Boolean).map((line) => {
    const [host, container] = line.split(':')
    const [cport, proto] = (container || '').split('/')
    return {
      host_ip: '',
      host_port: (host || '').trim(),
      container_port: (cport || '').trim(),
      protocol: (proto || 'tcp').trim() || 'tcp',
    }
  })
}

function parseLines(text: string) {
  return text.split('\n').map((l) => l.trim()).filter(Boolean)
}

async function saveContainerDetail() {
  if (!detail.value) return
  try {
    await ElMessageBox.confirm(t('docker.recreateConfirm'), t('common.confirm'), { type: 'warning' })
  } catch {
    return
  }
  detailSaving.value = true
  try {
    const ports = editPorts.value.filter((p) => p.host_port && p.container_port)
    await api.post(`/docker/containers/${detail.value.id}/recreate`, {
      ports,
      env: parseLines(editEnvText.value),
      restart_policy: editRestart.value,
    })
    ElMessage.success(t('docker.recreateSuccess'))
    detailVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    detailSaving.value = false
  }
}

function openCreateContainer() {
  createForm.value = {
    name: '',
    image: '',
    ports_text: '',
    env_text: '',
    mounts_text: '',
    restart_policy: 'unless-stopped',
    command: '',
  }
  createVisible.value = true
}

async function submitCreateContainer() {
  if (!createForm.value.image.trim()) {
    ElMessage.warning(t('docker.imageRequired'))
    return
  }
  createSaving.value = true
  try {
    const mounts = parseLines(createForm.value.mounts_text).map((line) => {
      const [src, dst] = line.split(':')
      return { type: 'bind', source: (src || '').trim(), destination: (dst || '').trim(), read_only: false }
    }).filter((m) => m.destination)
    await api.post('/docker/containers/run', {
      name: createForm.value.name.trim(),
      image: createForm.value.image.trim(),
      ports: parsePortsText(createForm.value.ports_text),
      env: parseLines(createForm.value.env_text),
      mounts,
      restart_policy: createForm.value.restart_policy,
      command: createForm.value.command.trim() ? createForm.value.command.trim().split(/\s+/) : [],
    })
    ElMessage.success(t('docker.createSuccess'))
    createVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    createSaving.value = false
  }
}

async function pullImageAction() {
  if (!pullImage.value.trim()) return
  pullLoading.value = true
  try {
    await api.post('/docker/images/pull', { image: pullImage.value.trim() }, { timeout: 600000 })
    ElMessage.success(t('docker.pullSuccess'))
    pullVisible.value = false
    pullImage.value = ''
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    pullLoading.value = false
  }
}

async function removeImage(row: any) {
  await ElMessageBox.confirm(t('docker.removeImageConfirm', { name: row.repo_tags || row.id }), t('common.warning'), { type: 'warning' })
  try {
    await api.delete(`/docker/images/${encodeURIComponent(row.id)}`, { params: { force: 1 } })
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function pruneImages() {
  try {
    const res: any = await api.post('/docker/images/prune')
    ElMessage.success(res.data?.message || t('docker.pruneDone'))
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function createVolumeAction() {
  if (!volumeName.value.trim()) return
  volumeCreating.value = true
  try {
    await api.post('/docker/volumes', { name: volumeName.value.trim(), driver: volumeDriver.value.trim() })
    ElMessage.success(t('common.success'))
    volumeCreateVisible.value = false
    volumeName.value = ''
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    volumeCreating.value = false
  }
}

async function removeVolume(row: any) {
  await ElMessageBox.confirm(t('docker.removeVolumeConfirm', { name: row.name }), t('common.warning'), { type: 'warning' })
  try {
    await api.delete(`/docker/volumes/${encodeURIComponent(row.name)}`)
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function pruneVolumes() {
  try {
    const res: any = await api.post('/docker/volumes/prune')
    ElMessage.success(res.data?.message || t('docker.pruneDone'))
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function createNetworkAction() {
  if (!networkName.value.trim()) return
  networkCreating.value = true
  try {
    await api.post('/docker/networks', {
      name: networkName.value.trim(),
      driver: networkDriver.value.trim() || 'bridge',
      subnet: networkSubnet.value.trim(),
      gateway: networkGateway.value.trim(),
    })
    ElMessage.success(t('common.success'))
    networkCreateVisible.value = false
    networkName.value = ''
    networkSubnet.value = ''
    networkGateway.value = ''
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    networkCreating.value = false
  }
}

async function removeNetwork(row: any) {
  if (row.is_system || ['bridge', 'host', 'none'].includes(row.name)) {
    ElMessage.warning(t('docker.systemNetworkProtected'))
    return
  }
  await ElMessageBox.confirm(t('docker.removeNetworkConfirm', { name: row.name }), t('common.warning'), { type: 'warning' })
  try {
    await api.delete(`/docker/networks/${encodeURIComponent(row.id)}`)
    ElMessage.success(t('common.deleted'))
    if (activeNetwork.value?.id === row.id) networkDrawerVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function pruneNetworks() {
  try {
    const res: any = await api.post('/docker/networks/prune')
    ElMessage.success(res.data?.message || t('docker.pruneDone'))
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

function parsePortsFromString(ports: string): number[] {
  const out: number[] = []
  const seen = new Set<number>()
  for (const part of (ports || '').split(',')) {
    const m = part.trim().match(/:(\d+)->\d+\/tcp/)
    if (!m) continue
    const p = parseInt(m[1], 10)
    if (p > 0 && !seen.has(p)) {
      seen.add(p)
      out.push(p)
    }
  }
  return out
}

async function openDomainDialog(row: any) {
  domainContainer.value = row
  domainInput.value = row.bind_domain || ''
  domainPortOptions.value = parsePortsFromString(row.ports)
  domainHostPort.value = row.host_port || domainPortOptions.value[0] || null
  if (!domainPortOptions.value.length) {
    try {
      const res: any = await api.get(`/docker/containers/${row.id}`)
      const ports = (res.data?.ports || []) as PortMapping[]
      domainPortOptions.value = ports
        .filter((p) => p.host_port && (!p.protocol || p.protocol === 'tcp'))
        .map((p) => parseInt(p.host_port, 10))
        .filter((p) => p > 0)
      if (!domainHostPort.value && domainPortOptions.value.length) {
        domainHostPort.value = domainPortOptions.value[0]
      }
    } catch {
      /* ignore */
    }
  }
  domainVisible.value = true
}

async function saveDomainBinding() {
  if (!domainContainer.value) return
  const domain = domainInput.value.trim()
  if (!domain) {
    ElMessage.warning(t('docker.domainRequired'))
    return
  }
  if (!domainPortOptions.value.length) {
    ElMessage.warning(t('docker.noPortMapping'))
    return
  }
  domainSaving.value = true
  try {
    await api.put(`/docker/containers/${domainContainer.value.id}/domain`, {
      domain,
      host_port: domainHostPort.value || domainPortOptions.value[0],
    })
    ElMessage.success(t('docker.bindSuccess'))
    domainVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    domainSaving.value = false
  }
}

async function unbindDomain() {
  if (!domainContainer.value?.bind_domain) return
  try {
    await ElMessageBox.confirm(t('docker.unbindConfirm', { name: domainContainer.value.name }), t('common.warning'), { type: 'warning' })
  } catch {
    return
  }
  domainSaving.value = true
  try {
    await api.delete(`/docker/containers/${domainContainer.value.id}/domain`)
    ElMessage.success(t('common.success'))
    domainVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    domainSaving.value = false
  }
}

function onFabClick() {
  if (activeTab.value === 'containers') openCreateContainer()
  else if (activeTab.value === 'images') { pullImage.value = ''; pullVisible.value = true }
  else if (activeTab.value === 'volumes') { volumeName.value = ''; volumeCreateVisible.value = true }
  else if (activeTab.value === 'networks') {
    networkName.value = ''
    networkSubnet.value = ''
    networkGateway.value = ''
    networkCreateVisible.value = true
  }
}

function statusTagType(s: string) {
  if (!s) return 'info'
  if (s.toLowerCase().includes('up')) return 'success'
  if (s.toLowerCase().includes('exit')) return 'danger'
  return 'warning'
}

onMounted(load)
</script>

<template>
  <div class="docker-page">
    <div class="page-header">
      <h2>{{ t('page.docker') }}</h2>
      <div class="header-actions">
        <template v-if="status?.installed">
          <span class="status-badge">
            <span class="status-dot" :class="status.running ? 'running' : 'stopped'" />
            <span class="status-text">{{ status.running ? t('docker.statusRunning') : t('docker.statusStopped') }}</span>
            <span v-if="status.version" class="status-version">{{ t('docker.version') }}: {{ status.version }}</span>
          </span>
          <el-button :loading="uninstalling" @click="uninstallDocker">{{ t('common.uninstall') }}</el-button>
        </template>
        <el-button v-else type="primary" :loading="installing" @click="autoDetectInstall">{{ t('docker.autoDetectInstall') }}</el-button>
        <el-button :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
      </div>
    </div>

    <el-alert v-if="!status?.installed" type="warning" :closable="false" show-icon class="hint">
      <p>{{ t('docker.notInstalled') }}</p>
      <p class="hint-sub">{{ t('docker.notInstalledHint') }}</p>
    </el-alert>
    <el-alert v-else-if="!status.daemon_ok" type="warning" :closable="false" show-icon class="hint">
      {{ t('compose.dockerUnavailable') }}
    </el-alert>

    <el-tabs v-model="activeTab">
      <el-tab-pane :label="t('docker.containers')" name="containers">
        <el-table v-loading="loading" :data="containers" stripe>
          <el-table-column prop="name" :label="t('common.name')" min-width="140" />
          <el-table-column prop="image" :label="t('docker.image')" min-width="160" show-overflow-tooltip />
          <el-table-column prop="status" :label="t('common.status')" width="130">
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)" size="small" effect="plain">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="ports" :label="t('common.port')" min-width="160" show-overflow-tooltip />
          <el-table-column :label="t('docker.bindDomain')" min-width="140" show-overflow-tooltip>
            <template #default="{ row }">
              <a v-if="row.bind_domain" class="domain-link" :href="row.access_url || `http://${row.bind_domain}`" target="_blank" rel="noopener noreferrer" @click.stop>{{ row.bind_domain }}</a>
              <span v-else class="muted-text">{{ t('docker.domainUnbound') }}</span>
            </template>
          </el-table-column>
          <el-table-column v-if="dockerReady" :label="t('common.actions')" width="230" fixed="right" align="center">
            <template #default="{ row }">
              <div class="docker-actions">
                <el-tooltip :content="t('docker.bindDomainTitle')" placement="top">
                  <el-button text type="primary" size="small" :icon="Link" @click="openDomainDialog(row)" />
                </el-tooltip>
                <el-tooltip :content="t('docker.settings')" placement="top">
                  <el-button text type="primary" size="small" :icon="Setting" @click="openDetail(row)" />
                </el-tooltip>
                <el-tooltip :content="t('docker.logs')" placement="top">
                  <el-button text type="primary" size="small" :icon="Document" @click="openLogs(row)" />
                </el-tooltip>
                <el-tooltip :content="t('docker.restart')" placement="top">
                  <el-button text type="primary" size="small" :icon="RefreshRight" :loading="actionLoading === row.id" @click="restartContainer(row.id)" />
                </el-tooltip>
                <el-tooltip :content="t('common.start')" placement="top">
                  <el-button text type="success" size="small" :icon="VideoPlay" :loading="actionLoading === row.id" @click="startContainer(row.id)" />
                </el-tooltip>
                <el-tooltip :content="t('common.stop')" placement="top">
                  <el-button text type="warning" size="small" :icon="VideoPause" :loading="actionLoading === row.id" @click="stopContainer(row.id)" />
                </el-tooltip>
                <el-tooltip :content="t('common.delete')" placement="top">
                  <el-button text type="danger" size="small" :icon="Delete" @click="removeContainer(row)" />
                </el-tooltip>
              </div>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-if="!containers.length && !loading" :description="t('docker.emptyContainers')" />
      </el-tab-pane>

      <el-tab-pane :label="t('docker.images')" name="images">
        <div class="tab-toolbar">
          <el-button type="primary" plain @click="pullVisible = true; pullImage = ''">{{ t('docker.pullImage') }}</el-button>
          <el-button @click="pruneImages">{{ t('docker.pruneUnused') }}</el-button>
        </div>
        <el-table v-loading="loading" :data="images" stripe>
          <el-table-column prop="repo_tags" :label="t('docker.image')" min-width="220" />
          <el-table-column prop="id" label="ID" width="140" show-overflow-tooltip />
          <el-table-column prop="size" :label="t('files.size')" width="100" />
          <el-table-column prop="created" :label="t('files.modified')" width="120" />
          <el-table-column v-if="dockerReady" :label="t('common.actions')" width="72" fixed="right" align="center">
            <template #default="{ row }">
              <el-tooltip :content="t('common.delete')" placement="top">
                <el-button text type="danger" size="small" :icon="Delete" @click="removeImage(row)" />
              </el-tooltip>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-if="!images.length && !loading" :description="t('docker.emptyImages')" />
      </el-tab-pane>

      <el-tab-pane :label="t('docker.volumes')" name="volumes">
        <div class="volume-stats">
          <div class="volume-stat">
            <span class="volume-stat-value">{{ volumeStats.total }}</span>
            <span class="volume-stat-label">{{ t('docker.volumeTotal') }}</span>
          </div>
          <div class="volume-stat">
            <span class="volume-stat-value">{{ volumeStats.inUse }}</span>
            <span class="volume-stat-label">{{ t('docker.volumeInUseCount') }}</span>
          </div>
          <div class="volume-stat">
            <span class="volume-stat-value">{{ volumeStats.unused }}</span>
            <span class="volume-stat-label">{{ t('docker.volumeUnusedCount') }}</span>
          </div>
          <div class="volume-stat">
            <span class="volume-stat-value">{{ volumeStats.panel }}</span>
            <span class="volume-stat-label">{{ t('docker.volumePanelCount') }}</span>
          </div>
        </div>
        <div class="tab-toolbar volume-toolbar">
          <el-button type="primary" plain @click="volumeCreateVisible = true">{{ t('docker.createVolume') }}</el-button>
          <el-button @click="pruneVolumes">{{ t('docker.pruneUnused') }}</el-button>
          <el-input
            v-model="volumeSearch"
            class="volume-search"
            clearable
            :placeholder="t('docker.volumeSearchPlaceholder')"
          />
          <el-radio-group v-model="volumeViewMode" class="volume-view-toggle">
            <el-radio-button value="board">{{ t('docker.volumeViewBoard') }}</el-radio-button>
            <el-radio-button value="table">{{ t('docker.volumeViewTable') }}</el-radio-button>
          </el-radio-group>
        </div>
        <div v-if="volumeViewMode === 'board'" v-loading="loading" class="volume-kanban">
          <div v-for="col in volumeColumns" :key="col.key" class="volume-column">
            <div class="volume-column-header">
              <span class="volume-column-title">{{ col.title }}</span>
              <el-tag size="small" type="info" effect="plain">{{ col.volumes.length }}</el-tag>
            </div>
            <div class="volume-column-body">
              <div v-for="vol in col.volumes" :key="vol.name" class="volume-card">
                <div class="volume-card-head">
                  <el-tooltip :content="vol.name" placement="top" :show-after="400">
                    <span class="volume-card-name">{{ truncateText(volumeDisplayName(vol), 28) }}</span>
                  </el-tooltip>
                  <el-tag size="small" effect="plain">{{ vol.driver || 'local' }}</el-tag>
                </div>
                <div v-if="vol.category === 'anonymous'" class="volume-card-meta">
                  <el-tag size="small" type="warning" effect="plain">{{ t('docker.anonymousVolume') }}</el-tag>
                </div>
                <div class="volume-card-path">
                  <el-tooltip :content="vol.mountpoint" placement="top" :show-after="400">
                    <span class="volume-path-text">{{ truncateText(vol.mountpoint, 42) }}</span>
                  </el-tooltip>
                  <el-tooltip :content="t('docker.copyPath')" placement="top">
                    <el-button text size="small" :icon="CopyDocument" @click="copyToClipboard(vol.mountpoint)" />
                  </el-tooltip>
                </div>
                <div v-if="vol.containers?.length" class="volume-card-containers">
                  <span class="volume-containers-label">{{ t('docker.linkedContainers') }}</span>
                  <div class="volume-container-tags">
                    <el-tag v-for="c in vol.containers" :key="c" size="small" type="success" effect="plain">{{ c }}</el-tag>
                  </div>
                </div>
                <div v-if="dockerReady" class="volume-card-actions">
                  <el-tooltip :content="t('docker.copyPath')" placement="top">
                    <el-button text type="primary" size="small" :icon="CopyDocument" @click="copyToClipboard(vol.mountpoint)" />
                  </el-tooltip>
                  <el-tooltip :content="t('docker.openInFiles')" placement="top">
                    <el-button text type="primary" size="small" :icon="FolderOpened" @click="openVolumeInFiles(vol)" />
                  </el-tooltip>
                  <el-tooltip :content="t('common.delete')" placement="top">
                    <el-button text type="danger" size="small" :icon="Delete" @click="removeVolume(vol)" />
                  </el-tooltip>
                </div>
              </div>
              <div v-if="!col.volumes.length" class="volume-column-empty">—</div>
            </div>
          </div>
        </div>
        <el-table v-else v-loading="loading" :data="filteredVolumes" stripe>
          <el-table-column prop="name" :label="t('common.name')" min-width="180" show-overflow-tooltip />
          <el-table-column :label="t('common.type')" width="100">
            <template #default="{ row }">
              <span>{{ row.category || 'custom' }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="driver" :label="t('common.type')" width="100" />
          <el-table-column prop="mountpoint" :label="t('common.path')" min-width="220" show-overflow-tooltip />
          <el-table-column :label="t('docker.linkedContainers')" min-width="160" show-overflow-tooltip>
            <template #default="{ row }">
              {{ (row.containers || []).join(', ') || '—' }}
            </template>
          </el-table-column>
          <el-table-column v-if="dockerReady" :label="t('common.actions')" width="120" fixed="right" align="center">
            <template #default="{ row }">
              <div class="docker-actions">
                <el-tooltip :content="t('docker.openInFiles')" placement="top">
                  <el-button text type="primary" size="small" :icon="FolderOpened" @click="openVolumeInFiles(row)" />
                </el-tooltip>
                <el-tooltip :content="t('common.delete')" placement="top">
                  <el-button text type="danger" size="small" :icon="Delete" @click="removeVolume(row)" />
                </el-tooltip>
              </div>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-if="!filteredVolumes.length && !loading" :description="t('docker.emptyVolumes')" />
      </el-tab-pane>

      <el-tab-pane :label="t('docker.networks')" name="networks">
        <div class="volume-stats">
          <div class="volume-stat">
            <span class="volume-stat-value">{{ networkStats.total }}</span>
            <span class="volume-stat-label">{{ t('docker.networkTotal') }}</span>
          </div>
          <div class="volume-stat">
            <span class="volume-stat-value">{{ networkStats.system }}</span>
            <span class="volume-stat-label">{{ t('docker.networkSystemCount') }}</span>
          </div>
          <div class="volume-stat">
            <span class="volume-stat-value">{{ networkStats.inUse }}</span>
            <span class="volume-stat-label">{{ t('docker.networkInUseCount') }}</span>
          </div>
          <div class="volume-stat">
            <span class="volume-stat-value">{{ networkStats.unused }}</span>
            <span class="volume-stat-label">{{ t('docker.networkUnusedCount') }}</span>
          </div>
        </div>
        <div class="tab-toolbar volume-toolbar">
          <el-button type="primary" plain @click="networkCreateVisible = true">{{ t('docker.createNetwork') }}</el-button>
          <el-button @click="pruneNetworks">{{ t('docker.pruneUnused') }}</el-button>
          <el-input
            v-model="networkSearch"
            class="volume-search"
            clearable
            :placeholder="t('docker.networkSearchPlaceholder')"
          />
          <el-radio-group v-model="networkViewMode" class="volume-view-toggle">
            <el-radio-button value="board">{{ t('docker.volumeViewBoard') }}</el-radio-button>
            <el-radio-button value="table">{{ t('docker.volumeViewTable') }}</el-radio-button>
          </el-radio-group>
        </div>
        <div v-if="networkViewMode === 'board'" v-loading="loading" class="volume-kanban">
          <div v-for="col in networkColumns" :key="col.key" class="volume-column">
            <div class="volume-column-header">
              <span class="volume-column-title">{{ col.title }}</span>
              <el-tag size="small" type="info" effect="plain">{{ col.networks.length }}</el-tag>
            </div>
            <div class="volume-column-body">
              <div
                v-for="net in col.networks"
                :key="net.id"
                class="volume-card network-card"
                :class="{ 'network-card-system': net.is_system }"
                @click="openNetworkDrawer(net)"
              >
                <div class="volume-card-head">
                  <span class="volume-card-name">{{ net.name }}</span>
                  <el-tag size="small" effect="plain">{{ networkDriverLabel(net.driver) }}</el-tag>
                </div>
                <div v-if="net.is_system" class="volume-card-meta">
                  <el-tag size="small" type="info" effect="plain">{{ t('docker.networkSystemBadge') }}</el-tag>
                </div>
                <div v-if="net.subnet" class="network-meta-row">
                  <span class="network-meta-label">{{ t('docker.subnet') }}</span>
                  <span class="network-meta-value">{{ net.subnet }}</span>
                </div>
                <div v-if="net.gateway" class="network-meta-row">
                  <span class="network-meta-label">{{ t('docker.gateway') }}</span>
                  <span class="network-meta-value">{{ net.gateway }}</span>
                </div>
                <div class="network-id-row">
                  <el-tooltip :content="net.id" placement="top" :show-after="400">
                    <span class="volume-path-text">{{ truncateText(net.id, 16) }}</span>
                  </el-tooltip>
                  <el-tooltip :content="t('docker.copyNetworkId')" placement="top">
                    <el-button text size="small" :icon="CopyDocument" @click.stop="copyToClipboard(net.id)" />
                  </el-tooltip>
                </div>
                <div v-if="net.endpoints?.length" class="volume-card-containers">
                  <span class="volume-containers-label">{{ t('docker.linkedContainers') }}</span>
                  <div class="volume-container-tags">
                    <el-tag v-for="ep in net.endpoints.slice(0, 4)" :key="ep.name" size="small" type="success" effect="plain">
                      {{ ep.name }}
                    </el-tag>
                    <el-tag v-if="(net.endpoints?.length || 0) > 4" size="small" type="info" effect="plain">
                      +{{ (net.endpoints?.length || 0) - 4 }}
                    </el-tag>
                  </div>
                </div>
                <div v-if="dockerReady && !net.is_system" class="volume-card-actions" @click.stop>
                  <el-tooltip :content="t('common.delete')" placement="top">
                    <el-button text type="danger" size="small" :icon="Delete" @click="removeNetwork(net)" />
                  </el-tooltip>
                </div>
              </div>
              <div v-if="!col.networks.length" class="volume-column-empty">—</div>
            </div>
          </div>
        </div>
        <el-table v-else v-loading="loading" :data="filteredNetworks" stripe>
          <el-table-column prop="name" :label="t('common.name')" min-width="120" />
          <el-table-column prop="driver" :label="t('common.type')" width="90" />
          <el-table-column prop="subnet" :label="t('docker.subnet')" min-width="130" show-overflow-tooltip />
          <el-table-column :label="t('docker.linkedContainers')" min-width="160" show-overflow-tooltip>
            <template #default="{ row }">
              {{ (row.endpoints || []).map((e: DockerNetworkEndpoint) => e.name).join(', ') || '—' }}
            </template>
          </el-table-column>
          <el-table-column prop="scope" :label="t('docker.scope')" width="80" />
          <el-table-column prop="id" :label="t('docker.networkId')" min-width="120" show-overflow-tooltip />
          <el-table-column v-if="dockerReady" :label="t('common.actions')" width="72" fixed="right" align="center">
            <template #default="{ row }">
              <el-tooltip :content="t('common.delete')" placement="top">
                <el-button text type="danger" size="small" :icon="Delete" :disabled="row.is_system" @click="removeNetwork(row)" />
              </el-tooltip>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-if="!filteredNetworks.length && !loading" :description="t('docker.emptyNetworks')" />
      </el-tab-pane>
    </el-tabs>

    <el-button v-if="dockerReady" class="fab" type="primary" circle :icon="Plus" @click="onFabClick" />

    <!-- 绑定域名 -->
    <el-dialog v-model="domainVisible" :title="t('docker.bindDomainTitle')" width="480px" destroy-on-close>
      <el-form label-width="88px">
        <el-form-item :label="t('docker.bindDomain')">
          <el-input v-model="domainInput" :placeholder="t('docker.domainPlaceholder')" clearable />
        </el-form-item>
        <el-form-item v-if="domainPortOptions.length" :label="t('docker.selectPort')">
          <el-select v-model="domainHostPort" style="width: 100%">
            <el-option v-for="p in domainPortOptions" :key="p" :label="String(p)" :value="p" />
          </el-select>
        </el-form-item>
        <el-alert v-else type="warning" :closable="false" show-icon>{{ t('docker.noPortMapping') }}</el-alert>
        <p class="form-hint">{{ t('docker.bindDomainHint') }}</p>
      </el-form>
      <template #footer>
        <el-button v-if="domainContainer?.bind_domain" type="danger" plain :loading="domainSaving" @click="unbindDomain">{{ t('docker.unbindDomain') }}</el-button>
        <el-button @click="domainVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="domainSaving" :disabled="!domainPortOptions.length" @click="saveDomainBinding">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 容器设置 -->
    <el-dialog v-model="detailVisible" :title="t('docker.containerSettings')" width="720px" destroy-on-close>
      <div v-loading="detailLoading">
        <template v-if="detail">
          <el-descriptions :column="2" border size="small" class="detail-meta">
            <el-descriptions-item :label="t('common.name')">{{ detail.name }}</el-descriptions-item>
            <el-descriptions-item :label="t('docker.image')">{{ detail.image }}</el-descriptions-item>
            <el-descriptions-item :label="t('common.status')">{{ detail.status }}</el-descriptions-item>
            <el-descriptions-item :label="t('docker.restartPolicy')">{{ editRestart }}</el-descriptions-item>
          </el-descriptions>

          <el-alert type="info" :closable="false" show-icon class="recreate-hint">{{ t('docker.portChangeHint') }}</el-alert>

          <h4>{{ t('docker.portMappings') }}</h4>
          <div v-for="(p, i) in editPorts" :key="i" class="port-row">
            <el-input v-model="p.host_port" :placeholder="t('docker.hostPort')" />
            <span class="port-arrow">→</span>
            <el-input v-model="p.container_port" :placeholder="t('docker.containerPort')" />
            <el-select v-model="p.protocol" style="width: 90px">
              <el-option label="TCP" value="tcp" />
              <el-option label="UDP" value="udp" />
            </el-select>
            <el-button text type="danger" @click="removePortRow(i)">{{ t('common.delete') }}</el-button>
          </div>
          <el-button text type="primary" @click="addPortRow">+ {{ t('docker.addPort') }}</el-button>

          <h4>{{ t('docker.envVars') }}</h4>
          <el-input v-model="editEnvText" type="textarea" :rows="5" :placeholder="t('docker.envPlaceholder')" />

          <el-form-item :label="t('docker.restartPolicy')" class="restart-row">
            <el-select v-model="editRestart" style="width: 200px">
              <el-option :label="t('docker.restartNo')" value="no" />
              <el-option :label="t('docker.restartAlways')" value="always" />
              <el-option :label="t('docker.restartUnlessStopped')" value="unless-stopped" />
            </el-select>
          </el-form-item>
        </template>
      </div>
      <template #footer>
        <el-button @click="detailVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="detailSaving" @click="saveContainerDetail">{{ t('docker.saveAndRecreate') }}</el-button>
      </template>
    </el-dialog>

    <!-- 日志 -->
    <el-dialog v-model="logsVisible" :title="`${t('docker.logs')} - ${logsTitle}`" width="800px">
      <pre v-loading="logsLoading" class="log-pre">{{ logsContent }}</pre>
    </el-dialog>

    <!-- 创建容器 -->
    <el-dialog v-model="createVisible" :title="t('docker.createContainer')" width="640px" destroy-on-close>
      <el-form label-width="110px">
        <el-form-item :label="t('common.name')"><el-input v-model="createForm.name" placeholder="my-app" /></el-form-item>
        <el-form-item :label="t('docker.image')" required><el-input v-model="createForm.image" placeholder="nginx:latest" /></el-form-item>
        <el-form-item :label="t('docker.portMappings')">
          <el-input v-model="createForm.ports_text" type="textarea" :rows="3" :placeholder="t('docker.portsPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('docker.envVars')">
          <el-input v-model="createForm.env_text" type="textarea" :rows="3" :placeholder="t('docker.envPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('docker.volumeMounts')">
          <el-input v-model="createForm.mounts_text" type="textarea" :rows="3" :placeholder="t('docker.mountsPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('docker.restartPolicy')">
          <el-select v-model="createForm.restart_policy" style="width: 100%">
            <el-option :label="t('docker.restartNo')" value="no" />
            <el-option :label="t('docker.restartAlways')" value="always" />
            <el-option :label="t('docker.restartUnlessStopped')" value="unless-stopped" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('docker.command')"><el-input v-model="createForm.command" placeholder="nginx -g 'daemon off;'" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="createSaving" @click="submitCreateContainer">{{ t('docker.createContainer') }}</el-button>
      </template>
    </el-dialog>

    <!-- 拉取镜像 -->
    <el-dialog v-model="pullVisible" :title="t('docker.pullImage')" width="480px">
      <el-input v-model="pullImage" placeholder="nginx:latest" @keyup.enter="pullImageAction" />
      <template #footer>
        <el-button @click="pullVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="pullLoading" @click="pullImageAction">{{ t('docker.pullImage') }}</el-button>
      </template>
    </el-dialog>

    <!-- 创建卷 -->
    <el-dialog v-model="volumeCreateVisible" :title="t('docker.createVolume')" width="420px">
      <el-form label-width="80px">
        <el-form-item :label="t('common.name')" required><el-input v-model="volumeName" /></el-form-item>
        <el-form-item :label="t('common.type')"><el-input v-model="volumeDriver" placeholder="local" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="volumeCreateVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="volumeCreating" @click="createVolumeAction">{{ t('common.create') }}</el-button>
      </template>
    </el-dialog>

    <!-- 创建网络 -->
    <el-dialog v-model="networkCreateVisible" :title="t('docker.createNetwork')" width="480px" destroy-on-close>
      <el-form label-width="88px">
        <el-form-item :label="t('common.name')" required>
          <el-input v-model="networkName" :placeholder="t('docker.networkNamePlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('common.type')">
          <el-select v-model="networkDriver" style="width: 100%">
            <el-option label="bridge" value="bridge" />
            <el-option label="macvlan" value="macvlan" />
            <el-option label="ipvlan" value="ipvlan" />
            <el-option label="overlay" value="overlay" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('docker.subnet')">
          <el-input v-model="networkSubnet" :placeholder="t('docker.subnetPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('docker.gateway')">
          <el-input v-model="networkGateway" :placeholder="t('docker.gatewayPlaceholder')" />
        </el-form-item>
        <p class="form-hint">{{ t('docker.createNetworkHint') }}</p>
      </el-form>
      <template #footer>
        <el-button @click="networkCreateVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="networkCreating" @click="createNetworkAction">{{ t('common.create') }}</el-button>
      </template>
    </el-dialog>

    <!-- 网络详情 -->
    <el-drawer v-model="networkDrawerVisible" :title="activeNetwork?.name || t('docker.networks')" size="480px" destroy-on-close>
      <template v-if="activeNetwork">
        <div class="drawer-tags">
          <el-tag :type="activeNetwork.is_system ? 'info' : 'success'" effect="plain">{{ networkDriverLabel(activeNetwork.driver) }}</el-tag>
          <el-tag v-if="activeNetwork.is_system" type="info" effect="plain">{{ t('docker.networkSystemBadge') }}</el-tag>
          <el-tag v-if="activeNetwork.in_use" type="success" effect="plain">{{ t('docker.networkInUse') }}</el-tag>
        </div>
        <el-descriptions :column="1" border size="small" class="network-desc">
          <el-descriptions-item :label="t('docker.networkId')">{{ activeNetwork.id }}</el-descriptions-item>
          <el-descriptions-item :label="t('docker.scope')">{{ activeNetwork.scope }}</el-descriptions-item>
          <el-descriptions-item v-if="activeNetwork.subnet" :label="t('docker.subnet')">{{ activeNetwork.subnet }}</el-descriptions-item>
          <el-descriptions-item v-if="activeNetwork.gateway" :label="t('docker.gateway')">{{ activeNetwork.gateway }}</el-descriptions-item>
        </el-descriptions>
        <div v-if="activeNetwork.endpoints?.length" class="drawer-section">
          <h4>{{ t('docker.linkedContainers') }}</h4>
          <el-table :data="activeNetwork.endpoints" size="small" stripe>
            <el-table-column prop="name" :label="t('common.name')" min-width="120" />
            <el-table-column prop="ipv4" label="IPv4" width="130" />
          </el-table>
        </div>
        <el-empty v-else :description="t('docker.networkNoContainers')" :image-size="64" />
        <div class="drawer-actions">
          <el-button :icon="CopyDocument" @click="copyToClipboard(activeNetwork.id)">{{ t('docker.copyNetworkId') }}</el-button>
          <el-button v-if="dockerReady && !activeNetwork.is_system" type="danger" plain :icon="Delete" @click="removeNetwork(activeNetwork)">
            {{ t('common.delete') }}
          </el-button>
        </div>
      </template>
    </el-drawer>

    <SoftwareInstallLogDialog
      v-model="installLogVisible"
      :app-key="installLogKey"
      :app-name="installLogName"
      :trigger-install="installTrigger"
      @done="onInstallDone"
    />
  </div>
</template>

<style scoped>
.docker-page { position: relative; padding-bottom: 72px; }
.page-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 16px; }
.page-header h2 { margin: 0; }
.header-actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.status-badge { display: flex; align-items: center; gap: 8px; font-size: 13px; color: var(--el-text-color-regular); }
.status-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
.status-dot.running { background: var(--el-color-success); }
.status-dot.stopped { background: var(--el-text-color-placeholder); }
.status-version { color: var(--el-text-color-secondary); font-size: 12px; }
.hint { margin-bottom: 16px; }
.hint-sub { margin: 4px 0 0; font-size: 13px; color: var(--el-text-color-secondary); }
.tab-toolbar { margin-bottom: 12px; display: flex; gap: 8px; }
.docker-actions { display: inline-flex; align-items: center; gap: 2px; flex-wrap: nowrap; }
.docker-actions .el-button { margin-left: 0; padding: 4px 6px; }
.fab { position: fixed; right: 32px; bottom: 32px; width: 48px; height: 48px; font-size: 20px; z-index: 10; box-shadow: 0 4px 12px rgba(0,0,0,.15); }
.detail-meta { margin-bottom: 12px; }
.recreate-hint { margin: 12px 0; }
.port-row { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.port-row .el-input { flex: 1; }
.port-arrow { color: var(--el-text-color-secondary); }
.restart-row { margin-top: 12px; }
.log-pre { max-height: 60vh; overflow: auto; background: #1e1e1e; color: #d4d4d4; padding: 12px; border-radius: 6px; font-size: 12px; line-height: 1.5; white-space: pre-wrap; word-break: break-all; margin: 0; }
h4 { margin: 16px 0 8px; font-size: 14px; }
.domain-link { color: var(--el-color-primary); text-decoration: none; }
.domain-link:hover { text-decoration: underline; }
.muted-text { color: var(--el-text-color-placeholder); font-size: 13px; }
.form-hint { margin: 8px 0 0; font-size: 12px; color: var(--el-text-color-secondary); line-height: 1.5; }
.volume-stats { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; margin-bottom: 12px; }
.volume-stat { padding: 12px 14px; border-radius: 8px; background: var(--el-fill-color-light); border: 1px solid var(--el-border-color-lighter); }
.volume-stat-value { display: block; font-size: 22px; font-weight: 600; line-height: 1.2; color: var(--el-text-color-primary); }
.volume-stat-label { display: block; margin-top: 4px; font-size: 12px; color: var(--el-text-color-secondary); }
.volume-toolbar { flex-wrap: wrap; align-items: center; }
.volume-search { width: min(280px, 100%); margin-left: auto; }
.volume-view-toggle { margin-left: 0; }
.volume-kanban { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; align-items: start; min-height: 200px; }
.volume-column { display: flex; flex-direction: column; min-height: 120px; border-radius: 10px; border: 1px solid var(--el-border-color-lighter); background: var(--el-fill-color-blank); overflow: hidden; }
.volume-column-header { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 10px 12px; border-bottom: 1px solid var(--el-border-color-lighter); background: var(--el-fill-color-light); }
.volume-column-title { font-size: 13px; font-weight: 600; color: var(--el-text-color-primary); }
.volume-column-body { display: flex; flex-direction: column; gap: 10px; padding: 10px; max-height: 62vh; overflow-y: auto; }
.volume-column-empty { padding: 24px 8px; text-align: center; color: var(--el-text-color-placeholder); font-size: 13px; }
.volume-card { padding: 10px 12px; border-radius: 8px; border: 1px solid var(--el-border-color-lighter); background: var(--el-bg-color); transition: border-color .15s, box-shadow .15s; }
.volume-card:hover { border-color: var(--el-color-primary-light-5); box-shadow: 0 2px 8px rgba(0, 0, 0, .06); }
.volume-card-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 6px; }
.volume-card-name { font-size: 13px; font-weight: 600; color: var(--el-text-color-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; min-width: 0; }
.volume-card-meta { margin-bottom: 6px; }
.volume-card-path { display: flex; align-items: center; gap: 2px; margin-bottom: 6px; }
.volume-path-text { flex: 1; min-width: 0; font-size: 12px; color: var(--el-text-color-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
.volume-card-containers { margin-bottom: 8px; }
.volume-containers-label { display: block; font-size: 11px; color: var(--el-text-color-secondary); margin-bottom: 4px; }
.volume-container-tags { display: flex; flex-wrap: wrap; gap: 4px; }
.volume-card-actions { display: flex; align-items: center; gap: 2px; border-top: 1px solid var(--el-border-color-extra-light); padding-top: 6px; }
.volume-card-actions .el-button { margin-left: 0; padding: 4px 6px; }
.network-card { cursor: pointer; }
.network-card-system { border-left: 3px solid var(--el-color-info-light-3); }
.network-meta-row { display: flex; gap: 6px; margin-bottom: 4px; font-size: 12px; }
.network-meta-label { color: var(--el-text-color-secondary); flex-shrink: 0; }
.network-meta-value { color: var(--el-text-color-regular); font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.network-id-row { display: flex; align-items: center; gap: 2px; margin-bottom: 6px; }
.drawer-tags { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 16px; }
.network-desc { margin-bottom: 16px; }
.drawer-section h4 { margin: 0 0 8px; font-size: 14px; }
.drawer-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 20px; }
@media (max-width: 960px) {
  .volume-stats { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .volume-kanban { grid-template-columns: 1fr; }
  .volume-search { width: 100%; margin-left: 0; order: 3; flex: 1 1 100%; }
}
</style>
