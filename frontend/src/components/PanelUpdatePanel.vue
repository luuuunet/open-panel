<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import api from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'

const { t } = useI18n()

const loading = ref(false)
const checking = ref(false)
const applying = ref(false)
const saving = ref(false)
const status = ref<any>(null)

const check = computed(() => status.value?.check)
const versionInfo = computed(() => status.value?.version || {})
const config = ref({
  enabled: false,
  schedule: '0 4 * * 0',
  auto_apply: false,
  repo: 'luuuunet/owpanel',
})

const updateAvailable = computed(() => !!check.value?.update_available)
const canApply = computed(() => !!check.value?.can_apply)

async function loadStatus() {
  loading.value = true
  try {
    const res: any = await api.get('/settings/update/status')
    status.value = res.data
    if (res.data?.config) {
      config.value = { ...config.value, ...res.data.config }
    }
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('settings.updateLoadFailed'))
  } finally {
    loading.value = false
  }
}

async function runCheck() {
  checking.value = true
  try {
    const res: any = await api.post('/settings/update/check')
    if (status.value) {
      status.value.check = res.data
    } else {
      status.value = { check: res.data }
    }
    if (res.data?.update_available) {
      ElMessage.success(t('settings.updateAvailableMsg', { version: res.data.latest_version }))
    } else {
      ElMessage.info(t('settings.updateUpToDate'))
    }
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('settings.updateCheckFailed'))
  } finally {
    checking.value = false
  }
}

async function saveConfig() {
  saving.value = true
  try {
    await api.put('/settings/update/config', config.value)
    ElMessage.success(t('settings.updateConfigSaved'))
    await loadStatus()
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('settings.updateConfigFailed'))
  } finally {
    saving.value = false
  }
}

async function applyUpdate() {
  try {
    await ElMessageBox.confirm(
      t('settings.updateApplyConfirm'),
      t('settings.updateApplyTitle'),
      { type: 'warning', confirmButtonText: t('settings.updateApplyNow'), cancelButtonText: t('common.cancel') }
    )
  } catch {
    return
  }
  applying.value = true
  try {
    const res: any = await api.post('/settings/update/apply', {}, { timeout: 900000 })
    ElMessage.success(res.data?.message || t('settings.updateApplyScheduled'))
    setTimeout(() => {
      window.location.reload()
    }, 8000)
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('settings.updateApplyFailed'))
  } finally {
    applying.value = false
  }
}

function formatTime(v?: string) {
  if (!v) return '—'
  try {
    return new Date(v).toLocaleString()
  } catch {
    return v
  }
}

onMounted(loadStatus)
</script>

<template>
  <el-card v-loading="loading" shadow="hover" class="settings-card">
    <template #header>{{ t('settings.updateSection') }}</template>

    <p class="hint">{{ t('settings.updateIntro') }}</p>

    <el-descriptions :column="2" border size="small" style="margin-bottom: 16px">
      <el-descriptions-item :label="t('settings.updateCurrentVersion')">
        <el-tag>{{ versionInfo.version || 'dev' }}</el-tag>
      </el-descriptions-item>
      <el-descriptions-item :label="t('settings.updateLatestVersion')">
        <el-tag v-if="check?.latest_version" :type="updateAvailable ? 'warning' : 'success'">
          {{ check.latest_version }}
        </el-tag>
        <span v-else>—</span>
      </el-descriptions-item>
      <el-descriptions-item :label="t('settings.updateBuildDate')">
        {{ versionInfo.build_date || '—' }}
      </el-descriptions-item>
      <el-descriptions-item :label="t('settings.updateLastCheck')">
        {{ formatTime(status?.last_check_at) }}
      </el-descriptions-item>
    </el-descriptions>

    <div class="actions">
      <el-button :loading="checking" @click="runCheck">{{ t('settings.updateCheck') }}</el-button>
      <el-button
        v-if="updateAvailable"
        type="primary"
        :loading="applying"
        :disabled="!canApply"
        @click="applyUpdate"
      >
        {{ t('settings.updateApplyNow') }}
      </el-button>
      <el-link
        v-if="check?.release_url"
        :href="check.release_url"
        target="_blank"
        type="primary"
        style="margin-left: 12px"
      >
        {{ t('settings.updateReleaseNotes') }}
      </el-link>
    </div>

    <el-alert
      v-if="check?.apply_reason"
      type="info"
      :title="check.apply_reason"
      show-icon
      :closable="false"
      style="margin: 12px 0"
    />

    <el-alert
      v-if="updateAvailable && canApply"
      type="warning"
      :title="t('settings.updateBackupHint')"
      show-icon
      :closable="false"
      style="margin: 12px 0"
    />

    <el-divider>{{ t('settings.updateAutoTitle') }}</el-divider>

    <el-form label-width="140px">
      <el-form-item :label="t('settings.updateAutoEnabled')">
        <el-switch v-model="config.enabled" />
        <div class="hint">{{ t('settings.updateAutoEnabledHint') }}</div>
      </el-form-item>
      <el-form-item :label="t('settings.updateAutoSchedule')">
        <el-input v-model="config.schedule" :placeholder="t('settings.updateAutoScheduleHint')" style="max-width: 280px" />
      </el-form-item>
      <el-form-item :label="t('settings.updateAutoApply')">
        <el-switch v-model="config.auto_apply" :disabled="!config.enabled" />
        <div class="hint">{{ t('settings.updateAutoApplyHint') }}</div>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" :loading="saving" @click="saveConfig">{{ t('settings.updateSaveConfig') }}</el-button>
      </el-form-item>
    </el-form>

    <template v-if="status?.history?.length">
      <el-divider>{{ t('settings.updateHistory') }}</el-divider>
      <el-table :data="status.history" size="small" stripe>
        <el-table-column prop="created_at" :label="t('settings.updateHistoryTime')" width="180">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column prop="from_version" :label="t('settings.updateHistoryFrom')" width="120" />
        <el-table-column prop="to_version" :label="t('settings.updateHistoryTo')" width="120" />
        <el-table-column prop="status" :label="t('settings.updateHistoryStatus')" width="120" />
        <el-table-column prop="trigger" :label="t('settings.updateHistoryTrigger')" width="100" />
        <el-table-column prop="error_msg" :label="t('settings.updateHistoryError')" show-overflow-tooltip />
      </el-table>
    </template>
  </el-card>
</template>

<style scoped>
.hint {
  margin-top: 6px;
  color: var(--cf-text-muted);
  font-size: 12px;
}
.actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.settings-card {
  width: 100%;
  margin-bottom: 16px;
}
</style>
