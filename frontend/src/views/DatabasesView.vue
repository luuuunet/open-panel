<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import api, { resolveApiError } from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CopyDocument, View, Hide, Odometer, DataBoard, RefreshRight } from '@element-plus/icons-vue'
import DatabaseBackupDialog from '@/components/DatabaseBackupDialog.vue'
import PostgreSQLExtensionDialog from '@/components/PostgreSQLExtensionDialog.vue'
import SoftwareInstallLogDialog from '@/components/SoftwareInstallLogDialog.vue'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const auth = useAuthStore()
const isAdmin = computed(() => !auth.user?.role || auth.user.role === 'admin')

const dbTab = ref('mysql')
const instances = ref<any[]>([])
const mysqlStatus = ref<any>(null)
const mongodbStatus = ref<any>(null)
const pgStatus = ref<any>(null)
const dialogVisible = ref(false)
const form = ref({
  name: '',
  type: 'mysql',
  host: '127.0.0.1',
  port: 3306,
  username: '',
  password: '',
  remark: '',
  charset: 'utf8mb4',
  access_mode: 'local' as 'local' | 'remote' | 'both',
  serverTarget: 'local' as 'local' | 'remote',
  force_ssl: false,
})
const mysqlCharsets = [
  { value: 'utf8mb4', label: 'utf8mb4' },
  { value: 'utf8', label: 'utf-8' },
  { value: 'gbk', label: 'gbk' },
  { value: 'big5', label: 'big5' },
]
const pma = ref<any>(null)
const pmaLoading = ref(false)

const backupDialogVisible = ref(false)
const backupDbId = ref<number | null>(null)
const backupDbName = ref('')
const backupDbType = ref('mysql')
const backupInitialTab = ref('backups')

const editDialogVisible = ref(false)
const editSaving = ref(false)
const editingId = ref<number | null>(null)
const editForm = ref({
  name: '',
  type: 'mysql',
  host: '127.0.0.1',
  port: 3306,
  username: '',
  password: '',
  has_password: false,
  remark: '',
  allow_remote: false,
  access_mode: 'local' as 'local' | 'remote' | 'both',
})

const rootDialogVisible = ref(false)
const rootPassword = ref('')
const rootSaving = ref(false)

const revealedIds = ref<Record<number, string>>({})
const revealingId = ref<number | null>(null)

const kafkaAccelLoading = ref(false)
const kafkaAccelEnabled = ref(false)
const acceleratedIds = ref<number[]>([])
const installDialogVisible = ref(false)
const installTrigger = ref(false)
const accelDbLoadingId = ref<number | null>(null)
const pendingAccelDbId = ref<number | null>(null)

const remarkDialogVisible = ref(false)
const remarkSaving = ref(false)
const remarkEditRow = ref<any>(null)
const remarkEditValue = ref('')

const pgExtensionsVisible = ref(false)

interface EngineStatus {
  type: string
  software_key: string
  name: string
  installed: boolean
  installing: boolean
  running: boolean
  version?: string
}

const engineStatuses = ref<Record<string, EngineStatus>>({})
const engineInstallVisible = ref(false)
const engineInstallTrigger = ref(false)
const engineInstalling = ref(false)

const currentEngine = computed(() => engineStatuses.value[dbTab.value] || null)
const currentEngineInstalled = computed(() => !!currentEngine.value?.installed)
const currentEngineName = computed(() => currentEngine.value?.name || dbTab.value.toUpperCase())
const currentEngineKey = computed(() => currentEngine.value?.software_key || dbTab.value)

const acceleratedSet = computed(() => new Set(acceleratedIds.value))

const sqlEngineLabel = computed(() => {
  if (mysqlStatus.value?.engine === 'mariadb') return 'MariaDB'
  return 'MySQL'
})

const sqlVersionLabel = computed(() => {
  const st = mysqlStatus.value
  if (!st) return ''
  return st.server_version || st.version || ''
})

const filteredInstances = computed(() =>
  instances.value.filter((row) => {
    const tpe = (row.type || 'mysql').toLowerCase()
    if (dbTab.value === 'mysql') return tpe === 'mysql' || tpe === 'mariadb' || !row.type
    if (dbTab.value === 'mongodb') return tpe === 'mongodb'
    return tpe === dbTab.value
  }),
)

function randomPassword(len = 16) {
  const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789'
  let out = ''
  for (let i = 0; i < len; i++) out += chars[Math.floor(Math.random() * chars.length)]
  return out
}

function regeneratePassword() {
  form.value.password = randomPassword()
}

function defaultCreateForm(tab = dbTab.value) {
  return {
    name: '',
    type: dbTypeForTab(tab),
    host: tab === 'remote' ? '' : '127.0.0.1',
    port: defaultPortForTab(tab),
    username: '',
    password: randomPassword(),
    remark: '',
    charset: 'utf8mb4',
    access_mode: 'local' as 'local' | 'remote' | 'both',
    serverTarget: 'local' as 'local' | 'remote',
    force_ssl: false,
  }
}

const isLocalMysqlCreate = computed(
  () => dbTab.value === 'mysql' && form.value.serverTarget === 'local',
)

function dbTypeForTab(tab = dbTab.value) {
  if (tab === 'postgresql') return 'postgresql'
  if (tab === 'redis') return 'redis'
  if (tab === 'mongodb') return 'mongodb'
  if (tab === 'mysql' && mysqlStatus.value?.engine === 'mariadb') return 'mariadb'
  return 'mysql'
}

function defaultPortForTab(tab = dbTab.value) {
  if (tab === 'postgresql') return 5432
  if (tab === 'redis') return 6379
  if (tab === 'mongodb') return 27017
  return 3306
}

async function loadKafkaAccel() {
  if (!isAdmin.value) return
  try {
    const res: any = await api.get('/kafka-accel/config')
    const cfg = res.data?.config || {}
    kafkaAccelEnabled.value = !!cfg.enabled
    acceleratedIds.value = res.data?.linked_database_ids || []
  } catch {
    kafkaAccelEnabled.value = false
    acceleratedIds.value = []
  }
}

async function autoEnableKafkaAccel() {
  await ElMessageBox.confirm(t('databases.kafkaAccelHint'), t('databases.kafkaAccel'), { type: 'info' })
  kafkaAccelLoading.value = true
  try {
    let res: any = await api.post('/kafka-accel/auto-enable', { install_kafka: true }, { timeout: 1200000 })
    const data = res.data || {}
    if (data.needs_kafka_install) {
      installTrigger.value = true
      installDialogVisible.value = true
      return
    }
    ElMessage.success(data.message || t('databases.kafkaAccelSuccess'))
    await loadKafkaAccel()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('databases.kafkaAccelFailed')))
  } finally {
    kafkaAccelLoading.value = false
  }
}

async function onKafkaInstallDone(payload: { success: boolean }) {
  installTrigger.value = false
  if (!payload.success) {
    pendingAccelDbId.value = null
    return
  }
  kafkaAccelLoading.value = true
  try {
    const dbId = pendingAccelDbId.value
    pendingAccelDbId.value = null
    const url = dbId ? `/kafka-accel/databases/${dbId}/auto-enable` : '/kafka-accel/auto-enable'
    const res: any = await api.post(url, { install_kafka: false }, { timeout: 600000 })
    const data = res.data || {}
    if (data.enabled) {
      ElMessage.success(data.message || t('databases.kafkaAccelSuccess'))
      await loadKafkaAccel()
    }
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('databases.kafkaAccelFailed')))
  } finally {
    kafkaAccelLoading.value = false
    accelDbLoadingId.value = null
  }
}

function canShowAccelLink(row: any) {
  const tpe = (row.type || 'mysql').toLowerCase()
  if (dbTab.value === 'mysql') return tpe === 'mysql' || tpe === 'mariadb' || !row.type
  if (dbTab.value === 'postgresql') return tpe === 'postgresql' || tpe === 'postgres'
  return false
}

function dbAccelTooltip(row: any) {
  if (acceleratedSet.value.has(row.id)) return t('databases.kafkaAccelDisconnectTooltip')
  if (!canAccelDatabase(row)) return t('databases.kafkaAccelLocalOnly')
  return t('databases.kafkaAccelTooltip')
}

async function loadEngineStatuses() {
  try {
    const res: any = await api.get('/databases/engines/status')
    engineStatuses.value = res.data || {}
  } catch {
    engineStatuses.value = {}
  }
}

async function installCurrentEngine() {
  engineInstalling.value = true
  try {
    await api.post(`/software/${currentEngineKey.value}/install`, { version: '' }, { timeout: 120000 })
    ElMessage.success(t('databases.engineInstallStarted'))
    engineInstallTrigger.value = true
    engineInstallVisible.value = true
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('software.installFailed')))
  } finally {
    engineInstalling.value = false
  }
}

async function onEngineInstallDone(payload: { success: boolean }) {
  engineInstallTrigger.value = false
  await load()
  if (payload.success) ElMessage.success(t('software.installSuccessShort'))
}

function openCreateDialog() {
  if (!currentEngineInstalled.value) {
    ElMessage.warning(t('databases.engineNotInstalled', { name: currentEngineName.value }))
    return
  }
  form.value = defaultCreateForm()
  dialogVisible.value = true
}

function canAccelDatabase(row: any) {
  return canShowAccelLink(row) && isLocalHost(row)
}

async function disconnectDbAccel(row: any) {
  await ElMessageBox.confirm(
    t('databases.kafkaAccelDisconnectHint', { name: row.name }),
    t('databases.kafkaAccelDisconnectBtn'),
    { type: 'warning' },
  )
  accelDbLoadingId.value = row.id
  try {
    const newIds = acceleratedIds.value.filter((id) => id !== row.id)
    await api.patch('/kafka-accel/config', { linked_database_ids: newIds })
    ElMessage.success(t('databases.kafkaAccelDisconnectSuccess'))
    await loadKafkaAccel()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('databases.kafkaAccelDisconnectFailed')))
  } finally {
    accelDbLoadingId.value = null
  }
}

async function connectDbAccel(row: any) {
  await ElMessageBox.confirm(
    t('databases.kafkaAccelDbHint', { name: row.name }),
    t('databases.kafkaAccelRowBtn'),
    { type: 'info' },
  )
  accelDbLoadingId.value = row.id
  pendingAccelDbId.value = row.id
  try {
    const res: any = await api.post(`/kafka-accel/databases/${row.id}/auto-enable`, { install_kafka: true }, { timeout: 1200000 })
    const data = res.data || {}
    if (data.needs_kafka_install) {
      installTrigger.value = true
      installDialogVisible.value = true
      return
    }
    ElMessage.success(data.message || t('databases.kafkaAccelSuccess'))
    await loadKafkaAccel()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('databases.kafkaAccelFailed')))
    pendingAccelDbId.value = null
  } finally {
    if (!installDialogVisible.value) {
      accelDbLoadingId.value = null
    }
  }
}

async function toggleDbKafkaAccel(row: any) {
  if (!isAdmin.value) {
    ElMessage.warning(t('databases.kafkaAccelAdminOnly'))
    return
  }
  if (acceleratedSet.value.has(row.id)) {
    await disconnectDbAccel(row)
    return
  }
  if (!canAccelDatabase(row)) {
    ElMessage.warning(t('databases.kafkaAccelLocalOnly'))
    return
  }
  await connectDbAccel(row)
}

async function saveRemarkEdit() {
  if (!remarkEditRow.value) return
  remarkSaving.value = true
  try {
    await api.patch(`/databases/${remarkEditRow.value.id}`, { remark: remarkEditValue.value })
    remarkEditRow.value.remark = remarkEditValue.value.trim()
    ElMessage.success(t('common.saved'))
    remarkDialogVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    remarkSaving.value = false
  }
}

function openRemarkEdit(row: any) {
  remarkEditRow.value = row
  remarkEditValue.value = row.remark || ''
  remarkDialogVisible.value = true
}

async function load() {
  const [listRes, statusRes, mongoRes, pgRes]: any[] = await Promise.all([
    api.get('/databases'),
    api.get('/databases/mysql/status').catch(() => ({ data: null })),
    api.get('/databases/mongodb/status').catch(() => ({ data: null })),
    api.get('/databases/pgsql/status').catch(() => ({ data: null })),
  ])
  instances.value = listRes.data || []
  mysqlStatus.value = statusRes.data
  mongodbStatus.value = mongoRes.data
  pgStatus.value = pgRes.data
  await Promise.all([loadPma(), loadKafkaAccel(), loadEngineStatuses()])
}

async function loadPma() {
  try {
    const res: any = await api.get('/phpmyadmin/access')
    pma.value = res.data
  } catch {
    pma.value = null
  }
}

async function openPhpMyAdmin() {
  pmaLoading.value = true
  try {
    let res: any = await api.get('/phpmyadmin/access')
    pma.value = res.data
    if (!pma.value?.installed) {
      ElMessage.warning(t('databases.pmaNotInstalled'))
      return
    }
    if (!pma.value?.url) {
      res = await api.post('/phpmyadmin/setup')
      pma.value = res.data
    }
    if (!pma.value?.url) {
      ElMessage.warning(pma.value?.setup_error || t('databases.pmaNoUrl'))
      return
    }
    window.open(pma.value.url, '_blank', 'noopener,noreferrer')
  } catch (e: any) {
    ElMessage.error(e?.error || t('common.failed'))
  } finally {
    pmaLoading.value = false
  }
}

async function handleCreate() {
  if (!form.value.name.trim()) {
    ElMessage.warning(t('databases.nameRequired'))
    return
  }
  if (!form.value.username.trim()) {
    form.value.username = form.value.name
  }
  form.value.type = dbTypeForTab()
  const payload = {
    name: form.value.name.trim(),
    type: form.value.type,
    host: form.value.host,
    port: form.value.port,
    username: form.value.username.trim(),
    password: form.value.password,
    remark: form.value.remark,
    charset: form.value.charset,
    allow_remote: form.value.access_mode !== 'local',
    access_mode: form.value.access_mode,
    force_ssl: form.value.force_ssl,
  }
  if (isLocalMysqlCreate.value) {
    if (!form.value.password.trim()) {
      ElMessage.warning(t('databases.rootPasswordRequired'))
      return
    }
    await api.post('/databases/provision', {
      ...payload,
      host: '127.0.0.1',
      port: 3306,
    })
    ElMessage.success(t('databases.provisionCreated'))
  } else {
    await api.post('/databases', payload)
    ElMessage.success(t('databases.created'))
  }
  dialogVisible.value = false
  form.value = defaultCreateForm()
  load()
}

async function handleDelete(id: number) {
  await ElMessageBox.confirm(t('databases.deleteConfirm'), t('common.warning'), { type: 'warning' })
  await api.delete(`/databases/${id}`)
  ElMessage.success(t('common.deleted'))
  load()
}

function backupLabel(row: any) {
  if (row.backup_status && row.backup_status !== 'none') return row.backup_status
  if (row.backup_count > 0) return `${row.backup_count}${t('databases.backupUnit')}`
  return t('databases.backupNone')
}

function sourceLabel(row: any) {
  const h = (row.host || '').toLowerCase()
  if (h === '127.0.0.1' || h === 'localhost' || h === '::1') return t('databases.sourceLocal')
  return row.host || '—'
}

function isMySQLRow(row: any) {
  const tpe = (row.type || 'mysql').toLowerCase()
  return tpe === 'mysql' || tpe === 'mariadb' || !row.type
}

function isLocalHost(row: any) {
  const h = (row.host || '').toLowerCase()
  return h === '127.0.0.1' || h === 'localhost' || h === '::1' || h === ''
}

function resolveAccessMode(row: any): 'local' | 'remote' | 'both' {
  if (row.access_mode === 'remote' || row.access_mode === 'both' || row.access_mode === 'local') {
    return row.access_mode
  }
  return row.allow_remote ? 'both' : 'local'
}

function accessModeLabel(mode: 'local' | 'remote' | 'both') {
  if (mode === 'remote') return t('databases.accessRemoteOnly')
  if (mode === 'both') return t('databases.accessBoth')
  return t('databases.accessLocalOnly')
}

function accessModeTagType(mode: 'local' | 'remote' | 'both'): 'success' | 'warning' | 'info' {
  if (mode === 'both') return 'warning'
  if (mode === 'remote') return 'info'
  return 'success'
}

function remoteAccessLabel(row: any) {
  if (isMySQLRow(row)) return accessModeLabel(resolveAccessMode(row))
  return isLocalHost(row) ? t('databases.permRW') : t('databases.permRemote')
}

function openBackup(row: any, tab = 'backups') {
  backupDbId.value = row.id
  backupDbName.value = row.name
  backupDbType.value = row.type
  backupInitialTab.value = tab
  backupDialogVisible.value = true
}

async function openEdit(row: any) {
  editingId.value = row.id
  editForm.value = {
    name: row.name,
    type: row.type,
    host: row.host || '127.0.0.1',
    port: row.port || 3306,
    username: row.username || row.name,
    password: '',
    has_password: !!row.has_password,
    remark: row.remark || '',
    allow_remote: resolveAccessMode(row) !== 'local',
    access_mode: resolveAccessMode(row),
  }
  editDialogVisible.value = true
}

async function saveEdit() {
  if (!editingId.value) return
  if (!editForm.value.username.trim()) {
    ElMessage.warning(t('databases.usernameRequired'))
    return
  }
  editSaving.value = true
  try {
    const payload: Record<string, unknown> = {
      host: editForm.value.host,
      port: editForm.value.port,
      username: editForm.value.username,
      password: editForm.value.password,
      remark: editForm.value.remark,
    }
    if (isMySQLRow(editForm.value)) {
      payload.access_mode = editForm.value.access_mode
      payload.allow_remote = editForm.value.access_mode !== 'local'
    }
    await api.patch(`/databases/${editingId.value}`, payload)
    ElMessage.success(t('databases.credentialsSaved'))
    delete revealedIds.value[editingId.value]
    editDialogVisible.value = false
    load()
  } catch (e: any) {
    ElMessage.error(e?.error || e?.response?.data?.error || t('dbBackup.saveFailed'))
  } finally {
    editSaving.value = false
  }
}

async function togglePassword(row: any) {
  if (revealedIds.value[row.id] !== undefined) {
    delete revealedIds.value[row.id]
    return
  }
  revealingId.value = row.id
  try {
    const res: any = await api.get(`/databases/${row.id}/credentials`)
    if (!res.data?.password) {
      ElMessage.warning(t('databases.noPassword'))
      openEdit(row)
      return
    }
    revealedIds.value[row.id] = res.data.password
  } catch (e: any) {
    ElMessage.error(e?.error || t('common.failed'))
  } finally {
    revealingId.value = null
  }
}

function passwordDisplay(row: any) {
  if (revealedIds.value[row.id] !== undefined) return revealedIds.value[row.id]
  if (row.has_password) return '••••••••'
  return '—'
}

async function copyPassword(row: any) {
  let pwd = revealedIds.value[row.id]
  if (!pwd) {
    try {
      const res: any = await api.get(`/databases/${row.id}/credentials`)
      pwd = res.data?.password
    } catch {
      ElMessage.error(t('common.failed'))
      return
    }
  }
  if (!pwd) {
    ElMessage.warning(t('databases.noPassword'))
    return
  }
  await navigator.clipboard.writeText(pwd)
  ElMessage.success(t('databases.passwordCopied'))
}

async function saveRootPassword() {
  if (!rootPassword.value.trim()) {
    ElMessage.warning(t('databases.rootPasswordRequired'))
    return
  }
  rootSaving.value = true
  try {
    await api.post('/databases/mysql/root-password', { password: rootPassword.value })
    ElMessage.success(t('databases.rootPasswordSaved'))
    rootDialogVisible.value = false
    rootPassword.value = ''
    load()
  } catch (e: any) {
    ElMessage.error(e?.error || e?.response?.data?.error || t('common.failed'))
  } finally {
    rootSaving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="databases-page">
    <div class="page-header">
      <h2>{{ t('page.databases') }}</h2>
    </div>

    <el-tabs v-model="dbTab" class="db-tabs">
      <el-tab-pane :label="t('databases.tabMysql')" name="mysql" />
      <el-tab-pane :label="t('databases.tabPgsql')" name="postgresql" />
      <el-tab-pane :label="t('databases.tabMongo')" name="mongodb" />
      <el-tab-pane :label="t('databases.tabRedis')" name="redis" />
    </el-tabs>

    <el-alert
      v-if="!currentEngineInstalled"
      type="warning"
      :closable="false"
      show-icon
      class="engine-alert"
    >
      {{ t('databases.engineNotInstalled', { name: currentEngineName }) }}
    </el-alert>

    <div class="toolbar">
      <div class="toolbar-left">
        <el-button
          v-if="!currentEngineInstalled"
          type="primary"
          :loading="engineInstalling || currentEngine?.installing"
          @click="installCurrentEngine"
        >
          {{ t('databases.installEngine', { name: currentEngineName }) }}
        </el-button>
        <template v-else>
        <el-button type="success" @click="openCreateDialog">{{ t('databases.add') }}</el-button>
        <el-button v-if="dbTab === 'mysql'" @click="rootDialogVisible = true">{{ t('databases.rootPassword') }}</el-button>
        <el-button v-if="dbTab === 'mysql'" type="warning" :icon="DataBoard" :loading="pmaLoading" @click="openPhpMyAdmin">
          {{ t('databases.openPhpMyAdmin') }}
        </el-button>
        <el-button
          v-if="dbTab === 'postgresql'"
          type="primary"
          plain
          @click="pgExtensionsVisible = true"
        >
          {{ t('databases.extensions') }}
        </el-button>
        <el-button
          v-if="isAdmin && (dbTab === 'mysql' || dbTab === 'postgresql')"
          type="primary"
          plain
          :loading="kafkaAccelLoading"
          @click="autoEnableKafkaAccel"
        >
          {{ t('databases.kafkaAccel') }}
        </el-button>
        </template>
        <div v-if="kafkaAccelEnabled && isAdmin" class="mysql-badge kafka-badge">
          <span class="dot running" />
          {{ t('databases.kafkaAccelBadge') }}
        </div>
        <div v-if="currentEngineInstalled && dbTab === 'mysql' && mysqlStatus?.installed" class="mysql-badge">
          <span class="dot running" />
          {{ sqlEngineLabel }} {{ sqlVersionLabel }}
        </div>
        <div v-if="currentEngineInstalled && dbTab === 'mongodb' && mongodbStatus?.installed" class="mysql-badge">
          <span class="dot" :class="{ running: mongodbStatus.running }" />
          MongoDB {{ mongodbStatus.version || '' }}
        </div>
        <div v-if="currentEngineInstalled && dbTab === 'postgresql' && pgStatus?.installed" class="mysql-badge">
          <span class="dot running" />
          PostgreSQL {{ pgStatus.version || '' }}
        </div>
      </div>
    </div>

    <el-alert
      v-if="dbTab === 'mysql' && mysqlStatus?.legacy_mysql57"
      type="info"
      :closable="false"
      show-icon
      class="legacy-hint"
    >
      {{ t('databases.mysql57LegacyHint') }}
    </el-alert>

    <el-table v-if="currentEngineInstalled" :data="filteredInstances" stripe class="db-table">
      <el-table-column prop="name" :label="t('databases.dbName')" min-width="140" />
      <el-table-column prop="remark" :label="t('databases.remark')" min-width="120" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="remark-link" :title="t('common.remarkClickHint')" @click="openRemarkEdit(row)">
            {{ row.remark || '—' }}
          </span>
          <el-tag v-if="acceleratedSet.has(row.id)" size="small" type="success" effect="plain" class="accel-tag">
            {{ t('databases.kafkaAccelRow') }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="username" :label="t('databases.username')" min-width="130">
        <template #default="{ row }">{{ row.username || row.name }}</template>
      </el-table-column>
      <el-table-column :label="t('dbBackup.password')" min-width="180">
        <template #default="{ row }">
          <div class="pwd-cell">
            <span class="pwd-text">{{ passwordDisplay(row) }}</span>
            <el-button
              v-if="row.has_password"
              text
              size="small"
              :icon="revealedIds[row.id] !== undefined ? Hide : View"
              :loading="revealingId === row.id"
              @click="togglePassword(row)"
            />
            <el-button v-if="row.has_password" text size="small" :icon="CopyDocument" @click="copyPassword(row)" />
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="t('databases.permission')" width="90" align="center">
        <template #default="{ row }">
          <el-tag size="small" :type="isMySQLRow(row) ? accessModeTagType(resolveAccessMode(row)) : 'success'" effect="plain">
            {{ remoteAccessLabel(row) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('databases.backup')" width="130" align="center">
        <template #default="{ row }">
          <el-link type="warning" @click="openBackup(row, 'backups')">{{ backupLabel(row) }}</el-link>
          <span class="link-sep">|</span>
          <el-link type="primary" @click="openBackup(row, 'import')">{{ t('databases.import') }}</el-link>
        </template>
      </el-table-column>
      <el-table-column :label="t('databases.tools')" width="112" align="center">
        <template #default="{ row }">
          <div class="tool-actions">
            <el-tooltip v-if="dbTab === 'mysql'" :content="t('databases.openPhpMyAdmin')" placement="top">
              <el-button text type="warning" :icon="DataBoard" :loading="pmaLoading" @click="openPhpMyAdmin" />
            </el-tooltip>
            <el-tooltip v-if="canShowAccelLink(row) && isAdmin" :content="dbAccelTooltip(row)" placement="top">
              <el-button
                text
                :type="acceleratedSet.has(row.id) ? 'success' : 'primary'"
                :icon="Odometer"
                :loading="accelDbLoadingId === row.id"
                :disabled="!canAccelDatabase(row) && !acceleratedSet.has(row.id)"
                @click="toggleDbKafkaAccel(row)"
              />
            </el-tooltip>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="t('databases.source')" width="100">
        <template #default="{ row }">{{ sourceLabel(row) }}</template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" width="220" fixed="right">
        <template #default="{ row }">
          <el-button text type="primary" size="small" @click="openEdit(row)">{{ t('databases.permission') }}</el-button>
          <el-button text type="primary" size="small" @click="openBackup(row)">{{ t('databases.tools') }}</el-button>
          <el-button text type="danger" size="small" @click="handleDelete(row.id)">{{ t('common.delete') }}</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-empty
      v-else
      :description="t('databases.engineNotInstalled', { name: currentEngineName })"
      :image-size="72"
    >
      <el-button type="primary" :loading="engineInstalling" @click="installCurrentEngine">
        {{ t('databases.installEngine', { name: currentEngineName }) }}
      </el-button>
    </el-empty>

    <el-empty v-if="currentEngineInstalled && !filteredInstances.length" :description="t('databases.empty')" />

    <el-dialog v-model="dialogVisible" :title="t('databases.addTitle')" width="560px">
      <el-form :model="form" label-width="110px">
        <el-form-item :label="t('databases.dbName')" required>
          <div class="name-charset-row">
            <el-input v-model="form.name" :placeholder="t('databases.dbNamePlaceholder')" class="name-input" />
            <el-select
              v-if="isLocalMysqlCreate"
              v-model="form.charset"
              class="charset-select"
              :placeholder="t('databases.charset')"
            >
              <el-option v-for="c in mysqlCharsets" :key="c.value" :value="c.value" :label="c.label" />
            </el-select>
          </div>
        </el-form-item>
        <el-form-item :label="t('databases.username')">
          <el-input v-model="form.username" :placeholder="t('databases.usernameSameAsDb')" />
        </el-form-item>
        <el-form-item :label="t('dbBackup.password')">
          <div class="pwd-gen-row">
            <el-input v-model="form.password" type="text" class="pwd-input" />
            <el-button :icon="RefreshRight" @click="regeneratePassword" />
          </div>
        </el-form-item>
        <el-form-item v-if="isLocalMysqlCreate" :label="t('databases.accessPermission')">
          <el-select v-model="form.access_mode" style="width: 100%">
            <el-option value="local" :label="t('databases.accessLocalOnly')" />
            <el-option value="remote" :label="t('databases.accessRemoteOnly')" />
            <el-option value="both" :label="t('databases.accessBoth')" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="dbTab === 'mysql'" :label="t('databases.addTo')">
          <div class="server-target-row">
            <el-select v-model="form.serverTarget" style="flex: 1">
              <el-option value="local" :label="t('databases.localServer')" />
              <el-option value="remote" :label="t('databases.remoteServer')" />
            </el-select>
          </div>
          <div v-if="isLocalMysqlCreate" class="form-hint">{{ t('databases.localServerHint') }}</div>
        </el-form-item>
        <el-form-item v-if="isLocalMysqlCreate" :label="t('databases.forceSSL')">
          <el-switch v-model="form.force_ssl" />
          <span class="inline-hint">{{ t('databases.forceSSLHint') }}</span>
        </el-form-item>
        <template v-if="!isLocalMysqlCreate || form.serverTarget === 'remote'">
          <el-form-item :label="t('databases.host')">
            <el-input v-model="form.host" :placeholder="form.serverTarget === 'remote' ? t('databases.remoteHostPlaceholder') : '127.0.0.1'" />
          </el-form-item>
          <el-form-item :label="t('common.port')">
            <el-input-number v-model="form.port" :min="1" :max="65535" />
          </el-form-item>
        </template>
        <el-form-item :label="t('databases.remark')">
          <el-input v-model="form.remark" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="handleCreate">{{ t('databases.create') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="editDialogVisible" :title="t('databases.editTitle', { name: editForm.name })" width="520px">
      <el-form label-width="90px">
        <el-form-item :label="t('databases.dbName')">
          <el-input :model-value="editForm.name" disabled />
        </el-form-item>
        <el-form-item :label="t('databases.username')" required>
          <el-input v-model="editForm.username" />
        </el-form-item>
        <el-form-item :label="t('dbBackup.password')">
          <el-input v-model="editForm.password" type="password" show-password :placeholder="t('dbBackup.passwordHint')" />
          <div class="form-hint">{{ t('databases.passwordHint') }}</div>
        </el-form-item>
        <el-form-item :label="t('databases.remark')">
          <el-input v-model="editForm.remark" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item v-if="isMySQLRow(editForm)" :label="t('databases.accessPermission')">
          <el-radio-group v-model="editForm.access_mode">
            <el-radio value="local">{{ t('databases.accessLocalOnly') }}</el-radio>
            <el-radio value="remote">{{ t('databases.accessRemoteOnly') }}</el-radio>
            <el-radio value="both">{{ t('databases.accessBoth') }}</el-radio>
          </el-radio-group>
          <div class="form-hint">{{ t('databases.accessModeHint') }}</div>
        </el-form-item>
        <el-form-item :label="t('databases.host')">
          <el-input v-model="editForm.host" />
        </el-form-item>
        <el-form-item :label="t('common.port')">
          <el-input-number v-model="editForm.port" :min="1" :max="65535" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editDialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="editSaving" @click="saveEdit">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="rootDialogVisible" :title="t('databases.rootPassword')" width="440px">
      <el-alert type="warning" :closable="false" show-icon class="root-alert">
        {{ t('databases.rootPasswordHint') }}
      </el-alert>
      <el-form label-width="90px" style="margin-top: 16px">
        <el-form-item :label="t('dbBackup.password')" required>
          <el-input v-model="rootPassword" type="password" show-password />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="rootDialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="rootSaving" @click="saveRootPassword">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="remarkDialogVisible" :title="t('common.editRemark')" width="480px">
      <el-input v-model="remarkEditValue" type="textarea" :rows="3" :placeholder="t('common.remarkPlaceholder')" />
      <template #footer>
        <el-button @click="remarkDialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="remarkSaving" @click="saveRemarkEdit">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <DatabaseBackupDialog
      v-model:visible="backupDialogVisible"
      :database-id="backupDbId"
      :db-name="backupDbName"
      :db-type="backupDbType"
      :initial-tab="backupInitialTab"
      @updated="load"
    />

    <SoftwareInstallLogDialog
      v-model="engineInstallVisible"
      :app-key="currentEngineKey"
      :app-name="currentEngineName"
      :trigger-install="engineInstallTrigger"
      @done="onEngineInstallDone"
    />

    <SoftwareInstallLogDialog
      v-model="installDialogVisible"
      app-key="kafka"
      :app-name="'Kafka'"
      :trigger-install="installTrigger"
      @done="onKafkaInstallDone"
    />

    <PostgreSQLExtensionDialog
      v-model="pgExtensionsVisible"
      :databases="instances"
    />
  </div>
</template>

<style scoped>
.databases-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.db-tabs :deep(.el-tabs__header) {
  margin-bottom: 0;
}
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 10px;
}
.toolbar-left {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.mysql-badge {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 4px 12px;
  border-radius: 6px;
  background: var(--el-fill-color-light);
  font-size: 13px;
}
.mysql-badge .dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--el-color-info);
}
.mysql-badge .dot.running {
  background: var(--el-color-success);
}
.accel-tag {
  margin-left: 6px;
  vertical-align: middle;
}
.remark-link {
  color: var(--el-color-primary);
  cursor: pointer;
}
.remark-link:hover {
  text-decoration: underline;
}
.tool-actions {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 2px;
  flex-wrap: nowrap;
}
.tool-actions :deep(.el-button) {
  padding: 4px 6px;
  margin: 0;
}
.pwd-cell {
  display: flex;
  align-items: center;
  gap: 2px;
}
.pwd-text {
  font-family: Consolas, Monaco, monospace;
  font-size: 13px;
  min-width: 72px;
}
.link-sep {
  margin: 0 4px;
  color: var(--el-border-color);
}
.form-hint {
  margin-top: 4px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.legacy-hint {
  margin-bottom: 12px;
}
.engine-alert {
  margin-bottom: 12px;
}
.root-alert {
  margin-bottom: 0;
}
.name-charset-row {
  display: flex;
  gap: 8px;
  width: 100%;
}
.name-charset-row .name-input {
  flex: 1;
}
.name-charset-row .charset-select {
  width: 120px;
  flex-shrink: 0;
}
.pwd-gen-row {
  display: flex;
  gap: 8px;
  width: 100%;
}
.pwd-gen-row .pwd-input {
  flex: 1;
}
.server-target-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}
.inline-hint {
  margin-left: 10px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
