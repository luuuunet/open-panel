<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import api, { resolveApiError } from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'
import { cfTheme } from '@/config/theme'

interface StatusInfo {
  k3s_running: boolean
  k3s_installed: boolean
  nodes_ready: number
  nodes_total: number
  system_pods_ready: number
  system_pods_total: number
  k8s_ready: boolean
  hint?: string
  linux_only: boolean
}

interface SetupStep {
  key: string
  title: string
  description: string
  done: boolean
  current: boolean
  action?: string
}

interface ChecklistItem {
  key: string
  label: string
  pass: boolean
  level: string
  hint?: string
}

interface JoinInfo {
  server_url: string
  token: string
  commands: Record<string, string>
  script: string
}

const { t } = useI18n()
const activeTab = ref('dashboard')
const loading = ref(false)
const wizardLoading = ref(false)
const installLoading = ref(false)
const workloadsLoading = ref(false)

const dashboard = ref<{
  status: StatusInfo | null
  health_score: number
  setup_steps: SetupStep[]
  checklist: ChecklistItem[]
} | null>(null)

const joinInfo = ref<JoinInfo | null>(null)
const pods = ref<any[]>([])
const deployments = ref<any[]>([])
const nodes = ref<any[]>([])
const namespaces = ref<any[]>([])
const workloadTab = ref('pods')

const status = computed(() => dashboard.value?.status || null)
const setupSteps = computed(() => dashboard.value?.setup_steps || [])
const checklist = computed(() => dashboard.value?.checklist || [])
const healthScore = computed(() => dashboard.value?.health_score ?? 0)
const healthColor = computed(() => {
  if (healthScore.value >= 80) return cfTheme.success
  if (healthScore.value >= 50) return cfTheme.warning
  return cfTheme.danger
})

const statusCards = computed(() => {
  const s = status.value
  if (!s) return []
  return [
    { key: 'k3s', label: t('k8s.cardK3s'), value: s.k3s_running ? t('k8s.running') : t('k8s.stopped'), type: s.k3s_running ? 'success' : 'info' },
    { key: 'nodes', label: t('k8s.cardNodes'), value: `${s.nodes_ready}/${s.nodes_total}`, type: s.nodes_total > 0 && s.nodes_ready >= s.nodes_total ? 'success' : 'warning' },
    { key: 'pods', label: t('k8s.cardSystemPods'), value: `${s.system_pods_ready}/${s.system_pods_total}`, type: s.system_pods_ready >= s.system_pods_total && s.system_pods_total > 0 ? 'success' : 'warning' },
    { key: 'ready', label: t('k8s.cardReady'), value: s.k8s_ready ? t('k8s.ready') : t('k8s.notReady'), type: s.k8s_ready ? 'success' : 'warning' },
  ] as const
})

async function loadDashboard() {
  const res: any = await api.get('/k8s/dashboard')
  dashboard.value = res.data || null
}

async function loadJoinInfo() {
  try {
    const res: any = await api.get('/k8s/join-info')
    joinInfo.value = res.data || null
  } catch {
    joinInfo.value = null
  }
}

async function loadWorkloads() {
  workloadsLoading.value = true
  try {
    const [p, d, n, ns]: any[] = await Promise.all([
      api.get('/k8s/pods'),
      api.get('/k8s/deployments'),
      api.get('/k8s/nodes'),
      api.get('/k8s/namespaces'),
    ])
    pods.value = p.data || []
    deployments.value = d.data || []
    nodes.value = n.data || []
    namespaces.value = ns.data || []
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    workloadsLoading.value = false
  }
}

async function loadAll() {
  loading.value = true
  try {
    await Promise.all([loadDashboard(), loadJoinInfo()])
    if (activeTab.value === 'workloads') await loadWorkloads()
  } finally {
    loading.value = false
  }
}

async function runWizard() {
  await ElMessageBox.confirm(t('k8s.wizardConfirm'), t('common.confirm'), { type: 'info' })
  wizardLoading.value = true
  try {
    const res: any = await api.post('/k8s/wizard', { deploy_sample: true })
    const steps = (res.data?.steps || []).join('\n')
    ElMessage.success(res.data?.message || t('k8s.wizardDone'))
    if (steps) ElMessageBox.alert(steps, t('k8s.wizardTitle'), { type: 'success' })
    await loadAll()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    wizardLoading.value = false
  }
}

async function installK3s() {
  await ElMessageBox.confirm(t('k8s.installConfirm'), t('common.confirm'), { type: 'info' })
  installLoading.value = true
  try {
    const res: any = await api.post('/k8s/install')
    ElMessage.success(res.data?.message || t('k8s.installDone'))
    await loadAll()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    installLoading.value = false
  }
}

async function deploySample() {
  wizardLoading.value = true
  try {
    const res: any = await api.post('/k8s/wizard', { deploy_sample: true })
    ElMessage.success(res.data?.message || t('k8s.sampleDone'))
    await loadAll()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    wizardLoading.value = false
  }
}

async function runStepAction(step: SetupStep) {
  if (step.done) return
  switch (step.action) {
    case 'install_k3s':
      await installK3s()
      break
    case 'refresh':
      await loadAll()
      break
    case 'deploy_sample':
      await deploySample()
      break
    default:
      break
  }
}

async function copyText(text: string) {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(t('k8s.copied'))
  } catch {
    ElMessage.error(t('common.failed'))
  }
}

function onTabChange(name: string | number) {
  if (name === 'workloads' && pods.value.length === 0) loadWorkloads()
  if (name === 'join') loadJoinInfo()
}

onMounted(loadAll)
</script>

<template>
  <div class="k8s-view" v-loading="loading">
    <div class="k8s-header">
      <div>
        <h3 class="view-title">{{ t('k8s.title') }}</h3>
        <p class="view-sub">{{ t('k8s.subtitle') }}</p>
      </div>
      <div class="header-actions">
        <el-button :loading="loading" @click="loadAll">{{ t('common.refresh') }}</el-button>
        <el-button type="primary" :loading="wizardLoading" :disabled="status?.linux_only" @click="runWizard">
          {{ t('k8s.wizardOneClick') }}
        </el-button>
      </div>
    </div>

    <el-tabs v-model="activeTab" type="border-card" class="k8s-tabs" @tab-change="onTabChange">
      <el-tab-pane :label="t('k8s.tabDashboard')" name="dashboard">
        <el-row :gutter="16" class="dashboard-top">
          <el-col :xs="24" :md="6">
            <el-card shadow="never" class="health-card">
              <div class="health-ring">
                <el-progress type="dashboard" :percentage="healthScore" :color="healthColor" :width="120">
                  <template #default>
                    <span class="health-num">{{ healthScore }}</span>
                    <span class="health-label">{{ t('k8s.healthScore') }}</span>
                  </template>
                </el-progress>
              </div>
              <p v-if="status?.hint" class="health-hint">{{ status.hint }}</p>
            </el-card>
          </el-col>
          <el-col :xs="24" :md="18">
            <el-card shadow="never" class="status-card">
              <template #header><span>{{ t('k8s.statusOverview') }}</span></template>
              <el-row :gutter="10">
                <el-col v-for="c in statusCards" :key="c.key" :xs="12" :sm="6">
                  <div class="stat-pill">
                    <div class="stat-label">{{ c.label }}</div>
                    <el-tag :type="c.type" size="small" round>{{ c.value }}</el-tag>
                  </div>
                </el-col>
              </el-row>
            </el-card>
          </el-col>
        </el-row>

        <el-card shadow="never" class="section-card">
          <template #header><span>{{ t('k8s.setupWizard') }}</span></template>
          <el-steps :active="setupSteps.findIndex(s => s.current)" finish-status="success" align-center class="setup-steps">
            <el-step
              v-for="step in setupSteps"
              :key="step.key"
              :title="step.title"
              :description="step.description"
              :status="step.done ? 'success' : step.current ? 'process' : 'wait'"
            />
          </el-steps>
          <div class="step-actions">
            <el-button
              v-for="step in setupSteps.filter(s => s.current && !s.done)"
              :key="step.key"
              type="primary"
              size="small"
              :loading="installLoading || wizardLoading"
              @click="runStepAction(step)"
            >
              {{ t('k8s.doStep', { step: step.title }) }}
            </el-button>
          </div>
        </el-card>

        <el-card shadow="never" class="section-card">
          <template #header><span>{{ t('k8s.checklist') }}</span></template>
          <el-table :data="checklist" size="small" stripe>
            <el-table-column prop="label" :label="t('k8s.checkItem')" min-width="160" />
            <el-table-column :label="t('k8s.checkStatus')" width="100">
              <template #default="{ row }">
                <el-tag :type="row.pass ? 'success' : 'warning'" size="small">{{ row.pass ? t('k8s.pass') : t('k8s.pending') }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="hint" :label="t('k8s.checkHint')" min-width="200" show-overflow-tooltip />
          </el-table>
        </el-card>
      </el-tab-pane>

      <el-tab-pane :label="t('k8s.tabWorkloads')" name="workloads">
        <div v-loading="workloadsLoading">
          <el-tabs v-model="workloadTab" type="card">
            <el-tab-pane :label="t('k8s.pods')" name="pods">
              <el-table :data="pods" size="small" stripe max-height="520">
                <el-table-column prop="namespace" :label="t('k8s.namespace')" width="140" />
                <el-table-column prop="name" :label="t('common.name')" min-width="180" />
                <el-table-column prop="status" :label="t('common.status')" width="100" />
                <el-table-column prop="ready" label="Ready" width="80" />
                <el-table-column prop="restarts" :label="t('k8s.restarts')" width="80" />
                <el-table-column prop="node" :label="t('k8s.node')" width="140" />
                <el-table-column prop="age" :label="t('k8s.age')" width="80" />
              </el-table>
            </el-tab-pane>
            <el-tab-pane :label="t('k8s.deployments')" name="deployments">
              <el-table :data="deployments" size="small" stripe max-height="520">
                <el-table-column prop="namespace" :label="t('k8s.namespace')" width="140" />
                <el-table-column prop="name" :label="t('common.name')" min-width="180" />
                <el-table-column prop="ready" label="Ready" width="80" />
                <el-table-column prop="available" :label="t('k8s.available')" width="90" />
                <el-table-column prop="age" :label="t('k8s.age')" width="80" />
              </el-table>
            </el-tab-pane>
            <el-tab-pane :label="t('k8s.nodes')" name="nodes">
              <el-table :data="nodes" size="small" stripe max-height="520">
                <el-table-column prop="name" :label="t('common.name')" min-width="160" />
                <el-table-column prop="status" :label="t('common.status')" width="100" />
                <el-table-column prop="roles" :label="t('k8s.roles')" width="160" />
                <el-table-column prop="version" :label="t('common.version')" min-width="120" />
                <el-table-column prop="age" :label="t('k8s.age')" width="80" />
              </el-table>
            </el-tab-pane>
            <el-tab-pane :label="t('k8s.namespaces')" name="namespaces">
              <el-table :data="namespaces" size="small" stripe max-height="520">
                <el-table-column prop="name" :label="t('common.name')" min-width="180" />
                <el-table-column prop="status" :label="t('common.status')" width="100" />
                <el-table-column prop="age" :label="t('k8s.age')" width="80" />
              </el-table>
            </el-tab-pane>
          </el-tabs>
          <p v-if="!status?.k3s_running" class="empty-hint">{{ t('k8s.workloadsNeedK3s') }}</p>
        </div>
      </el-tab-pane>

      <el-tab-pane :label="t('k8s.tabJoin')" name="join">
        <el-card v-if="joinInfo" shadow="never" class="join-card">
          <template #header><strong>{{ t('k8s.joinTitle') }}</strong></template>
          <p class="join-intro">{{ t('k8s.joinIntro') }}</p>
          <div class="join-meta">
            <div class="join-meta-row">
              <span class="join-label">{{ t('k8s.serverUrl') }}</span>
              <code>{{ joinInfo.server_url }}</code>
              <el-button size="small" @click="copyText(joinInfo.server_url)">{{ t('k8s.copy') }}</el-button>
            </div>
            <div class="join-meta-row">
              <span class="join-label">{{ t('k8s.token') }}</span>
              <code class="token-preview">{{ joinInfo.token ? joinInfo.token.slice(0, 16) + '…' : '—' }}</code>
              <el-button size="small" @click="copyText(joinInfo.token)">{{ t('k8s.copy') }}</el-button>
            </div>
          </div>
          <div class="join-row join-row-main">
            <div class="join-label">{{ t('k8s.joinWorker') }}</div>
            <code class="join-cmd">{{ joinInfo.commands?.worker || '—' }}</code>
            <el-button type="primary" size="small" @click="copyText(joinInfo.commands?.worker || '')">{{ t('k8s.copyCmd') }}</el-button>
          </div>
        </el-card>
        <el-empty v-else :description="t('k8s.joinUnavailable')" />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<style scoped>
.k8s-view { padding: 0 4px 24px; }
.k8s-header { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; margin-bottom: 16px; flex-wrap: wrap; }
.view-title { margin: 0 0 4px; font-size: 20px; font-weight: 600; }
.view-sub { margin: 0; color: var(--el-text-color-secondary); font-size: 13px; }
.header-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.dashboard-top { margin-bottom: 16px; }
.health-card, .status-card, .section-card { margin-bottom: 16px; }
.health-ring { display: flex; justify-content: center; padding: 8px 0; }
.health-num { display: block; font-size: 22px; font-weight: 700; line-height: 1.1; }
.health-label { display: block; font-size: 11px; color: var(--el-text-color-secondary); }
.health-hint { margin: 12px 0 0; font-size: 12px; color: var(--el-text-color-secondary); text-align: center; }
.stat-pill { padding: 10px 8px; border: 1px solid var(--el-border-color-lighter); border-radius: 8px; margin-bottom: 8px; }
.stat-label { font-size: 12px; color: var(--el-text-color-secondary); margin-bottom: 6px; }
.setup-steps { margin: 16px 0; }
.step-actions { display: flex; gap: 8px; flex-wrap: wrap; justify-content: center; margin-top: 12px; }
.join-card { margin-top: 8px; }
.join-intro { margin: 0 0 16px; color: var(--el-text-color-secondary); font-size: 13px; }
.join-meta { margin-bottom: 16px; }
.join-meta-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-bottom: 8px; }
.join-row { display: flex; align-items: flex-start; gap: 12px; flex-wrap: wrap; margin-bottom: 12px; }
.join-row-main { padding: 12px; background: var(--el-fill-color-light); border-radius: 8px; }
.join-label { min-width: 100px; font-size: 13px; font-weight: 500; }
.join-cmd { flex: 1; min-width: 200px; font-size: 12px; word-break: break-all; padding: 8px; background: var(--el-fill-color); border-radius: 4px; }
.token-preview { font-size: 12px; }
.empty-hint { margin-top: 12px; color: var(--el-text-color-secondary); font-size: 13px; }
</style>
