<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Setting, Star, VideoPause, VideoPlay } from '@element-plus/icons-vue'
import api, { resolveApiError } from '@/api'
import SoftwareConfigDialog from '@/components/SoftwareConfigDialog.vue'
import { ElMessage } from 'element-plus'

const { t } = useI18n()
const router = useRouter()

const versions = ref<any[]>([])
const loading = ref(false)
const acting = ref<string | null>(null)
const configDialog = ref(false)
const configApp = ref<{ key: string; name: string } | null>(null)

async function load() {
  loading.value = true
  try {
    const res: any = await api.get('/php/versions')
    versions.value = res.data || []
  } finally {
    loading.value = false
  }
}

async function setDefault(ver: string) {
  ElMessage.success(t('runtime.phpSetDefaultOk', { ver }))
  load()
}

async function toggle(row: any) {
  const action = row.status === 'running' ? 'stop' : 'start'
  acting.value = row.key
  try {
    await api.post(`/php/${row.key}/${action}`)
    ElMessage.success(action === 'start' ? t('runtime.phpStarted') : t('runtime.phpStopped'))
    await load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    acting.value = null
  }
}

function openConfig(row: any) {
  configApp.value = { key: row.key, name: `PHP ${row.version}` }
  configDialog.value = true
}

onMounted(load)
</script>

<template>
  <div>
    <div class="page-header"><h2>{{ t('nav.php') }}</h2></div>
    <el-table v-if="versions.length" v-loading="loading" :data="versions" stripe>
      <el-table-column prop="version" :label="t('common.version')" width="100" />
      <el-table-column prop="status" :label="t('common.status')" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 'running' ? 'success' : 'info'">
            {{ row.status === 'running' ? t('common.running') : t('common.stopped') }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="port" :label="t('common.port')" width="90" />
      <el-table-column prop="mode" :label="t('runtime.phpMode')" width="120" />
      <el-table-column prop="binary" :label="t('common.path')" min-width="200" show-overflow-tooltip />
      <el-table-column prop="default" :label="t('runtime.phpDefault')" width="80">
        <template #default="{ row }">
          <el-tag v-if="row.default" type="warning">{{ t('runtime.phpDefaultTag') }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" width="140" align="center">
        <template #default="{ row }">
          <div class="php-actions">
            <el-tooltip :content="t('common.config')" placement="top">
              <el-button text type="primary" size="small" :icon="Setting" @click="openConfig(row)" />
            </el-tooltip>
            <el-tooltip v-if="!row.default" :content="t('runtime.phpSetDefault')" placement="top">
              <el-button text type="primary" size="small" :icon="Star" @click="setDefault(row.version)" />
            </el-tooltip>
            <el-tooltip
              :content="row.status === 'running' ? t('common.stop') : t('common.start')"
              placement="top"
            >
              <el-button
                text
                size="small"
                :type="row.status === 'running' ? 'danger' : 'success'"
                :icon="row.status === 'running' ? VideoPause : VideoPlay"
                :loading="acting === row.key"
                @click="toggle(row)"
              />
            </el-tooltip>
          </div>
        </template>
      </el-table-column>
    </el-table>
    <el-empty
      v-else-if="!loading"
      :description="t('runtime.phpEmpty')"
      :image-size="72"
    >
      <el-button type="primary" @click="router.push('/software')">
        {{ t('nav.software') }}
      </el-button>
    </el-empty>
    <p v-if="versions.some(v => v.message)" class="hint">
      {{ versions.find(v => v.message)?.message }}
    </p>

    <SoftwareConfigDialog v-model="configDialog" :app="configApp" />
  </div>
</template>

<style scoped>
.hint {
  margin-top: 12px;
  color: var(--el-color-warning);
  font-size: 13px;
}
.php-actions {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  flex-wrap: nowrap;
}
.php-actions .el-button {
  margin-left: 0;
  padding: 4px 6px;
}
</style>
