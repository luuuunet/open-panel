<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import api, { resolveApiError } from '@/api'
import { ElMessage } from 'element-plus'

const props = defineProps<{
  modelValue: boolean
  siteId: number | null
  domain: string
}>()

const emit = defineEmits<{ 'update:modelValue': [boolean]; updated: [] }>()

const { t } = useI18n()
const loading = ref(false)
const pushing = ref(false)
const lastResult = ref<any>(null)

const form = ref({
  enabled: true,
  push_on_deploy: true,
  google: true,
  bing: true,
  indexnow: true,
  baidu: false,
  yandex: false,
  indexnow_key: '',
  sitemap_url: '',
  baidu_token: '',
  last_seo_push_at: null as string | null,
  last_seo_push_status: '',
  last_seo_push_log: '',
})

watch(() => props.modelValue, (v) => {
  if (v && props.siteId) load()
})

async function load() {
  if (!props.siteId) return
  loading.value = true
  try {
    const res: any = await api.get(`/wordpress/${props.siteId}/seo-push`)
    form.value = { ...form.value, ...(res.data || {}) }
    lastResult.value = null
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!props.siteId) return
  loading.value = true
  try {
    const payload = {
      enabled: form.value.enabled,
      push_on_deploy: form.value.push_on_deploy,
      google: form.value.google,
      bing: form.value.bing,
      indexnow: form.value.indexnow,
      baidu: form.value.baidu,
      yandex: form.value.yandex,
      indexnow_key: form.value.indexnow_key,
      sitemap_url: form.value.sitemap_url,
      baidu_token: form.value.baidu_token,
    }
    const res: any = await api.put(`/wordpress/${props.siteId}/seo-push`, payload)
    form.value = { ...form.value, ...(res.data || {}) }
    ElMessage.success(t('wp.seoPushSaved'))
    emit('updated')
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    loading.value = false
  }
}

async function pushNow() {
  if (!props.siteId) return
  pushing.value = true
  try {
    await save()
    const res: any = await api.post(`/wordpress/${props.siteId}/seo-push`)
    lastResult.value = res.data
    await load()
    ElMessage.success(t('wp.seoPushDone'))
    emit('updated')
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    pushing.value = false
  }
}

function genKey() {
  form.value.indexnow_key = crypto.randomUUID().replace(/-/g, '')
}

function close() {
  emit('update:modelValue', false)
}
</script>

<template>
  <el-dialog
    :model-value="modelValue"
    :title="t('wp.seoPushTitle', { domain })"
    width="640px"
    destroy-on-close
    @update:model-value="emit('update:modelValue', $event)"
  >
    <el-alert type="info" :closable="false" show-icon class="seo-intro">
      <template #title>{{ t('wp.seoPushIntroTitle') }}</template>
      <template #default>{{ t('wp.seoPushIntroBody') }}</template>
    </el-alert>

    <div v-loading="loading">
      <el-form label-width="140px" class="seo-form">
        <el-form-item :label="t('wp.seoPushEnabled')">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item :label="t('wp.seoPushOnDeploy')">
          <el-switch v-model="form.push_on_deploy" :disabled="!form.enabled" />
          <p class="form-hint">{{ t('wp.seoPushOnDeployHint') }}</p>
        </el-form-item>
        <el-divider content-position="left">{{ t('wp.seoPushEngines') }}</el-divider>
        <el-form-item :label="t('wp.seoPushGoogle')">
          <el-switch v-model="form.google" :disabled="!form.enabled" />
          <span class="form-hint inline">{{ t('wp.seoPushGoogleHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('wp.seoPushBing')">
          <el-switch v-model="form.bing" :disabled="!form.enabled" />
        </el-form-item>
        <el-form-item :label="t('wp.seoPushIndexNow')">
          <el-switch v-model="form.indexnow" :disabled="!form.enabled" />
          <span class="form-hint inline">{{ t('wp.seoPushIndexNowHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('wp.seoPushBaidu')">
          <el-switch v-model="form.baidu" :disabled="!form.enabled" />
        </el-form-item>
        <el-form-item :label="t('wp.seoPushYandex')">
          <el-switch v-model="form.yandex" :disabled="!form.enabled" />
        </el-form-item>
        <el-form-item :label="t('wp.seoPushIndexNowKey')">
          <div class="key-row">
            <el-input v-model="form.indexnow_key" :placeholder="t('wp.seoPushIndexNowKeyHint')" />
            <el-button @click="genKey">{{ t('wp.seoPushGenKey') }}</el-button>
          </div>
        </el-form-item>
        <el-form-item :label="t('wp.seoPushSitemap')">
          <el-input v-model="form.sitemap_url" :placeholder="t('wp.seoPushSitemapHint')" />
        </el-form-item>
        <el-form-item v-if="form.baidu" :label="t('wp.seoPushBaiduToken')">
          <el-input v-model="form.baidu_token" :placeholder="t('wp.seoPushBaiduTokenHint')" />
        </el-form-item>
      </el-form>

      <div v-if="form.last_seo_push_log" class="last-log">
        <div class="last-log-head">
          {{ t('wp.seoPushLastLog') }}
          <el-tag v-if="form.last_seo_push_status" size="small" :type="form.last_seo_push_status === 'success' ? 'success' : 'warning'">
            {{ form.last_seo_push_status }}
          </el-tag>
        </div>
        <pre>{{ form.last_seo_push_log }}</pre>
      </div>

      <div v-if="lastResult?.results?.length" class="last-log">
        <div class="last-log-head">{{ t('wp.seoPushResult') }}</div>
        <ul class="result-list">
          <li v-for="r in lastResult.results" :key="r.engine">
            <el-tag size="small" :type="r.ok ? 'success' : 'danger'">{{ r.engine }}</el-tag>
            {{ r.message }}
          </li>
        </ul>
      </div>
    </div>

    <template #footer>
      <el-button @click="close">{{ t('common.cancel') }}</el-button>
      <el-button @click="save">{{ t('common.save') }}</el-button>
      <el-button type="primary" :loading="pushing" :disabled="!form.enabled" @click="pushNow">
        {{ t('wp.seoPushNow') }}
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.seo-intro { margin-bottom: 16px; }
.seo-form { margin-top: 8px; }
.form-hint { margin: 4px 0 0; font-size: 12px; color: var(--el-text-color-secondary); line-height: 1.5; }
.form-hint.inline { margin: 0 0 0 8px; display: inline; }
.key-row { display: flex; gap: 8px; width: 100%; }
.last-log { margin-top: 12px; padding: 10px 12px; background: var(--el-fill-color-light); border-radius: 8px; }
.last-log-head { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; font-weight: 600; font-size: 13px; }
.last-log pre { margin: 0; font-size: 12px; white-space: pre-wrap; word-break: break-all; }
.result-list { margin: 0; padding-left: 18px; font-size: 13px; line-height: 1.8; }
</style>
