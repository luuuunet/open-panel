<script setup lang="ts">

import { computed, onMounted, reactive, ref } from 'vue'

import { useI18n } from 'vue-i18n'

import api, { AI_REQUEST_TIMEOUT, resolveApiError } from '@/api'

import { ElMessage } from 'element-plus'

import LanguageSwitcher from '@/components/LanguageSwitcher.vue'
import PanelMigrationPanel from '@/components/PanelMigrationPanel.vue'
import PanelUpdatePanel from '@/components/PanelUpdatePanel.vue'
import PerformanceModePanel from '@/components/PerformanceModePanel.vue'

import { DARK_VARIANT_OPTIONS, THEME_MODE_OPTIONS } from '@/config/themes'

import type { LocaleCode } from '@/locales'

import { useLocaleStore } from '@/stores/locale'

import { useThemeStore } from '@/stores/theme'



const { t } = useI18n()

const localeStore = useLocaleStore()

const themeStore = useThemeStore()

const themeMode = computed({
  get: () => themeStore.mode,
  set: (v) => themeStore.setMode(v),
})



const aiProviders = [

  { value: 'openai', labelKey: 'settings.aiProviderOpenAI' },

  { value: 'claude', labelKey: 'settings.aiProviderClaude' },

  { value: 'cursor', labelKey: 'settings.aiProviderCursor' },

  { value: 'deepseek', labelKey: 'settings.aiProviderDeepSeek' },

  { value: 'ollama', labelKey: 'settings.aiProviderOllama' },

  { value: 'huggingface', labelKey: 'settings.aiProviderHuggingFace' },

  { value: 'custom', labelKey: 'settings.aiProviderCustom' },

]



const aiDefaults: Record<string, { base: string; model: string }> = {

  openai: { base: 'https://api.openai.com/v1', model: 'gpt-4o-mini' },

  claude: { base: 'https://api.anthropic.com/v1', model: 'claude-sonnet-4-6' },

  cursor: { base: 'https://api.cursor.com', model: 'composer-2.5' },

  deepseek: { base: 'https://api.deepseek.com/v1', model: 'deepseek-chat' },

  ollama: { base: 'http://127.0.0.1:11434/v1', model: 'llama3.2' },

  huggingface: { base: 'http://127.0.0.1:8095/v1', model: 'Qwen2.5-0.5B-Instruct' },

  custom: { base: '', model: '' },

}



const providerHintKeys: Record<string, string> = {
  claude: 'settings.aiProviderClaudeHint',
  cursor: 'settings.aiProviderCursorHint',
}

const providerHint = computed(() => {
  const key = providerHintKeys[form.ai_provider]
  return key ? t(key) : ''
})

const baseUrlPlaceholder = computed(() => {
  if (form.ai_provider === 'cursor') return t('settings.aiBaseUrlCursorPlaceholder')
  return t('settings.aiBaseUrlPlaceholder')
})



const form = reactive({

  panel_name: '',

  panel_port: '',

  panel_ssl: 'false',

  panel_safe_path: '',

  login_captcha: 'false',

  session_timeout: '',

  backup_path: '',

  website_path: '',

  ai_enabled: 'false',

  ai_provider: 'openai',

  ai_api_key: '',

  ai_api_key_set: 'false',

  ai_base_url: '',

  ai_model: '',

})

const loading = ref(false)

const totpEnabled = ref(false)
const totpSetup = ref<{ qr_data?: string; secret?: string } | null>(null)
const totpVerifyCode = ref('')

async function loadTotpStatus() {
  try {
    const me: any = await api.get('/auth/me')
    totpEnabled.value = !!me.data?.totp_enabled
  } catch { /* ignore */ }
}

async function setupTotp() {
  const res: any = await api.post('/auth/totp/setup')
  totpSetup.value = res.data
}

async function verifyTotp() {
  await api.post('/auth/totp/verify', { code: totpVerifyCode.value })
  ElMessage.success(t('settings.totpEnabled'))
  totpEnabled.value = true
  totpSetup.value = null
}

async function disableTotp() {
  await api.post('/auth/totp/disable', { password: '' })
  totpEnabled.value = false
  ElMessage.success(t('common.success'))
}
const platform = ref<any>(null)

const uiLocale = ref<LocaleCode>(localeStore.locale)

type AIModel = { id: string; display_name: string; description?: string }

const aiModels = ref<AIModel[]>([])

const aiModelsLoading = ref(false)

const supportsModelSync = computed(() => {
  if (form.ai_provider === 'custom') return !!form.ai_base_url.trim()
  return !!form.ai_provider
})

const needsApiKeyForSync = computed(() => !['ollama', 'huggingface'].includes(form.ai_provider))

function aiModelLabel(m: AIModel) {
  return m.display_name && m.display_name !== m.id ? `${m.display_name} (${m.id})` : m.id
}

async function syncAIModels(showToast = true) {
  if (!supportsModelSync.value) {
    if (showToast) ElMessage.warning(t('settings.aiModelsNeedBaseUrl'))
    return
  }
  if (needsApiKeyForSync.value && !form.ai_api_key && form.ai_api_key_set !== 'true') {
    if (showToast) ElMessage.warning(t('settings.aiModelsNeedKey'))
    return
  }
  aiModelsLoading.value = true
  try {
    const payload: Record<string, string> = { ai_provider: form.ai_provider }
    const key = form.ai_api_key.trim()
    if (key) {
      payload.ai_api_key = key
      await api.put('/settings', { ai_api_key: key })
      form.ai_api_key_set = 'true'
      form.ai_api_key = ''
    }
    if (form.ai_base_url) payload.ai_base_url = form.ai_base_url
    const res: any = await api.post('/settings/ai-models/sync', payload, { timeout: AI_REQUEST_TIMEOUT })
    aiModels.value = res.data?.models || []
    if (!form.ai_model && aiModels.value.length) {
      form.ai_model = aiModels.value[0].id
    }
    if (showToast) {
      ElMessage.success(t('settings.aiModelsSynced', { n: aiModels.value.length }))
    }
  } catch (e: any) {
    if (showToast) {
      ElMessage.error(resolveApiError(e, t('settings.aiModelsSyncFailed'), t('common.requestTimeout')))
    }
  } finally {
    aiModelsLoading.value = false
  }
}

function onProviderChange(provider: string) {
  const preset = aiDefaults[provider]
  if (!preset || provider === 'custom') {
    aiModels.value = []
    return
  }
  form.ai_base_url = preset.base
  form.ai_model = preset.model
  syncAIModels(false)
}

async function loadPlatform() {
  try {
    const res: any = await api.get('/system/platform')
    platform.value = res.data || null
  } catch {
    platform.value = null
  }
}

async function load() {

  const res: any = await api.get('/settings')

  Object.assign(form, res.data || {})

  if (!form.ai_model || (form.ai_provider === 'cursor' && form.ai_model === 'default')) {
    form.ai_model = aiDefaults[form.ai_provider]?.model || 'gpt-4o-mini'
  }

  if (!form.ai_base_url && form.ai_provider !== 'custom') {

    form.ai_base_url = aiDefaults[form.ai_provider]?.base || ''

  }

  if (form.ai_provider === 'custom' && !form.ai_base_url) {
    aiModels.value = []
  } else if (supportsModelSync.value) {
    await syncAIModels(false)
  }

}



async function save() {

  loading.value = true

  try {

    const payload: Record<string, string> = { ...form }
    const newApiKey = payload.ai_api_key?.trim() || ''

    if (!newApiKey) {

      delete payload.ai_api_key

    } else {

      payload.ai_api_key = newApiKey

    }

    delete payload.ai_api_key_set

    await api.put('/settings', payload)

    localeStore.setLocale(uiLocale.value)

    if (newApiKey) {
      form.ai_api_key_set = 'true'
      form.ai_api_key = ''
      ElMessage.success(t('settings.aiApiKeySaved'))
    } else {
      ElMessage.success(t('settings.saved'))
    }

    await load()

  } finally {

    loading.value = false

  }

}



onMounted(() => {
  loadTotpStatus()
  load()
  loadPlatform()
})

</script>



<template>

  <div class="settings-page">

    <div class="page-header"><h2>{{ t('settings.title') }}</h2></div>

    <el-card v-if="platform" shadow="hover" class="settings-card">
      <template #header>{{ t('settings.platformSection') }}</template>
      <el-descriptions :column="1" border size="small">
        <el-descriptions-item :label="t('settings.platformOs')">{{ platform.os_name || platform.goos }} ({{ platform.goarch }})</el-descriptions-item>
        <el-descriptions-item :label="t('settings.platformPkgMgr')">{{ platform.package_manager || '—' }}</el-descriptions-item>
        <el-descriptions-item :label="t('settings.platformNote')">{{ platform.recommended_note }}</el-descriptions-item>
      </el-descriptions>
      <el-alert v-if="platform.goos === 'windows'" type="warning" show-icon :closable="false" :title="t('settings.platformWindowsLimit')" style="margin-top: 12px" />
    </el-card>

    <el-card shadow="hover" class="settings-card">

      <template #header>{{ t('settings.basicSection') }}</template>

      <el-form :model="form" label-width="120px">

        <el-form-item :label="t('settings.panelName')"><el-input v-model="form.panel_name" /></el-form-item>

        <el-form-item :label="t('settings.panelPort')"><el-input v-model="form.panel_port" /></el-form-item>

        <el-form-item :label="t('settings.safePath')">
          <el-input v-model="form.panel_safe_path" :placeholder="t('settings.safePathPlaceholder')" readonly />
          <div class="hint">{{ t('settings.safePathHint') }}</div>
        </el-form-item>

        <el-form-item :label="t('settings.enableSsl')">

          <el-switch v-model="form.panel_ssl" active-value="true" inactive-value="false" />

        </el-form-item>

        <el-form-item :label="t('settings.loginCaptcha')">

          <el-switch v-model="form.login_captcha" active-value="true" inactive-value="false" />

        </el-form-item>

        <el-form-item :label="t('settings.sessionTimeout')"><el-input v-model="form.session_timeout" /></el-form-item>

        <el-form-item :label="t('settings.backupPath')"><el-input v-model="form.backup_path" /></el-form-item>

        <el-form-item :label="t('settings.websitePath')"><el-input v-model="form.website_path" /></el-form-item>

        <el-form-item :label="t('settings.language')">

          <LanguageSwitcher v-model="uiLocale" />

          <div class="hint">{{ t('settings.languageHint') }}</div>

        </el-form-item>

      </el-form>

    </el-card>

    <el-card shadow="hover" class="settings-card">
      <template #header>{{ t('settings.appearanceSection') }}</template>
      <el-form label-width="120px">
        <el-form-item :label="t('theme.title')">
          <el-radio-group v-model="themeMode">
            <el-radio v-for="opt in THEME_MODE_OPTIONS" :key="opt.value" :value="opt.value">
              {{ t(opt.labelKey) }}
            </el-radio>
          </el-radio-group>
          <div class="hint">{{ t('settings.themeHint') }}</div>
        </el-form-item>
        <el-form-item v-if="themeStore.resolvedTheme === 'dark'" :label="t('theme.darkVariant')">
          <div class="theme-variant-row">
            <button
              v-for="opt in DARK_VARIANT_OPTIONS"
              :key="opt.value"
              type="button"
              class="theme-variant-btn"
              :class="{ active: themeStore.darkVariant === opt.value }"
              @click="themeStore.setDarkVariant(opt.value)"
            >
              <span class="theme-variant-swatch" :style="{ background: opt.swatch }" />
              {{ t(opt.labelKey) }}
            </button>
          </div>
        </el-form-item>
      </el-form>
    </el-card>



    <el-card shadow="hover" class="settings-card">

      <template #header>{{ t('settings.aiSection') }}</template>

      <el-form :model="form" label-width="120px">

        <el-form-item :label="t('settings.aiEnabled')">

          <el-switch v-model="form.ai_enabled" active-value="true" inactive-value="false" />

          <div class="hint">{{ t('settings.aiEnabledHint') }}</div>

        </el-form-item>

        <el-form-item :label="t('settings.aiProvider')">

          <el-select v-model="form.ai_provider" style="width: 100%" @change="onProviderChange">

            <el-option

              v-for="p in aiProviders"

              :key="p.value"

              :label="t(p.labelKey)"

              :value="p.value"

            />

          </el-select>

        </el-form-item>

        <el-form-item v-if="form.ai_provider !== 'ollama'" :label="t('settings.aiApiKey')">

          <el-input

            v-model="form.ai_api_key"

            type="password"

            show-password

            :placeholder="form.ai_api_key_set === 'true' ? t('settings.aiApiKeyKeep') : t('settings.aiApiKeyPlaceholder')"

          />

          <div class="key-status">
            <el-tag v-if="form.ai_api_key_set === 'true'" type="success" size="small">{{ t('settings.aiApiKeyConfigured') }}</el-tag>
            <el-tag v-else type="info" size="small">{{ t('settings.aiApiKeyNotConfigured') }}</el-tag>
          </div>
          <div v-if="providerHint" class="hint">{{ providerHint }}</div>
          <div class="hint">{{ t('settings.aiApiKeySaveHint') }}</div>

        </el-form-item>

        <el-form-item :label="t('settings.aiBaseUrl')">

          <el-input v-model="form.ai_base_url" :placeholder="baseUrlPlaceholder" />

        </el-form-item>

        <el-form-item :label="t('settings.aiModel')">
          <div v-if="supportsModelSync" class="model-row">
            <el-select
              v-model="form.ai_model"
              filterable
              style="flex: 1"
              :loading="aiModelsLoading"
              :placeholder="t('settings.aiModelSelectPlaceholder')"
            >
              <el-option
                v-if="form.ai_model && !aiModels.some((m) => m.id === form.ai_model)"
                :label="form.ai_model"
                :value="form.ai_model"
              />
              <el-option
                v-for="m in aiModels"
                :key="m.id"
                :label="aiModelLabel(m)"
                :value="m.id"
              />
            </el-select>
            <el-button :loading="aiModelsLoading" @click="syncAIModels(true)">
              {{ t('settings.syncAIModels') }}
            </el-button>
          </div>
          <el-input v-else v-model="form.ai_model" :placeholder="t('settings.aiModelPlaceholder')" />
          <div v-if="supportsModelSync" class="hint">{{ t('settings.aiModelSyncHint') }}</div>
        </el-form-item>

      </el-form>

    </el-card>

    <el-card class="settings-card" :header="t('settings.totpTitle')">
      <p class="hint">{{ t('settings.totpHint') }}</p>
      <el-tag v-if="totpEnabled" type="success">{{ t('settings.totpOn') }}</el-tag>
      <el-tag v-else type="info">{{ t('settings.totpOff') }}</el-tag>
      <div style="margin-top:12px">
        <el-button v-if="!totpEnabled && !totpSetup" @click="setupTotp">{{ t('settings.totpSetup') }}</el-button>
        <template v-if="totpSetup">
          <img v-if="totpSetup.qr_data" :src="`data:image/png;base64,${totpSetup.qr_data}`" alt="QR" style="max-width:200px;display:block;margin-bottom:8px" />
          <el-input v-model="totpVerifyCode" :placeholder="t('login.totpCodeHint')" style="max-width:200px;margin-bottom:8px" />
          <el-button type="primary" @click="verifyTotp">{{ t('settings.totpConfirm') }}</el-button>
        </template>
        <el-button v-if="totpEnabled" type="danger" plain @click="disableTotp">{{ t('settings.totpDisable') }}</el-button>
      </div>
    </el-card>

    <PerformanceModePanel />

    <PanelMigrationPanel />

    <PanelUpdatePanel />

    <el-button type="primary" :loading="loading" @click="save">{{ t('settings.saveSettings') }}</el-button>

  </div>

</template>



<style scoped>

.hint {

  margin-top: 6px;

  color: var(--cf-text-muted);

  font-size: 12px;

}

.settings-card {
  width: 100%;
  margin-bottom: 16px;
}

.theme-variant-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.theme-variant-btn {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border: 1px solid var(--cf-border);
  border-radius: 10px;
  background: var(--cf-bg);
  color: var(--cf-text);
  cursor: pointer;
  font-size: 13px;
}

.theme-variant-btn.active,
.theme-variant-btn:hover {
  border-color: var(--cf-orange);
  background: rgba(246, 130, 31, 0.08);
}

.theme-variant-swatch {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 1px solid rgba(255, 255, 255, 0.15);
}

.model-row {
  display: flex;
  gap: 8px;
  width: 100%;
}

.key-status {
  margin-top: 8px;
}

</style>

