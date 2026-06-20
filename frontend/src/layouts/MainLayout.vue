<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Moon, Monitor, Sunny } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { DARK_VARIANT_OPTIONS, THEME_MODE_OPTIONS } from '@/config/themes'
import { useThemeStore } from '@/stores/theme'
import { useLocaleStore } from '@/stores/locale'
import { LOCALE_OPTIONS, type LocaleCode } from '@/locales'
import AppSidebar from '@/components/AppSidebar.vue'
import ChangePasswordDialog from '@/components/ChangePasswordDialog.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const themeStore = useThemeStore()
const localeStore = useLocaleStore()
const { t } = useI18n()
const showChangePassword = ref(false)

const localeShort = computed(() => {
  const opt = LOCALE_OPTIONS.find((o) => o.value === localeStore.locale)
  return opt?.short ?? 'EN'
})

function setLocale(code: LocaleCode) {
  localeStore.setLocale(code)
}

onMounted(() => {
  if (auth.user?.must_change_password) showChangePassword.value = true
})

watch(
  () => auth.user?.must_change_password,
  (v) => {
    if (v) showChangePassword.value = true
  },
)

const pageTitle = computed(() => {
  const key = route.meta.titleKey as string | undefined
  return key ? t(key) : ''
})

const headerThemeIcon = computed(() => {
  if (themeStore.mode === 'dark') return Moon
  if (themeStore.mode === 'light') return Sunny
  return Monitor
})

function handleLogout() {
  auth.logout()
  router.push('/login')
}
</script>

<template>
  <el-container class="layout">
    <AppSidebar @logout="handleLogout" />

    <el-container class="main-wrap">
      <el-header class="header">
        <span class="page-title">{{ pageTitle }}</span>
        <div class="header-actions">
          <el-dropdown trigger="click" @command="setLocale">
            <el-button class="header-theme-btn header-locale-btn" text :title="t('common.language')">
              <span class="header-locale-label">{{ localeShort }}</span>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item
                  v-for="opt in LOCALE_OPTIONS"
                  :key="opt.value"
                  :command="opt.value"
                  :class="{ active: localeStore.locale === opt.value }"
                >
                  {{ opt.label }}
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-dropdown trigger="click" @command="(v: string) => themeStore.setMode(v as 'system' | 'light' | 'dark')">
            <el-button class="header-theme-btn" text circle :title="t('theme.title')">
              <el-icon><component :is="headerThemeIcon" /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item
                  v-for="opt in THEME_MODE_OPTIONS"
                  :key="opt.value"
                  :command="opt.value"
                  :class="{ active: themeStore.mode === opt.value }"
                >
                  {{ t(opt.labelKey) }}
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-popover v-if="themeStore.resolvedTheme === 'dark'" placement="bottom-end" :width="240" trigger="click">
            <template #reference>
              <el-button class="header-theme-btn" text circle :title="t('theme.darkVariant')">
                <span
                  class="header-variant-dot"
                  :style="{ background: DARK_VARIANT_OPTIONS.find((v) => v.value === themeStore.darkVariant)?.swatch }"
                />
              </el-button>
            </template>
            <div class="header-variant-panel">
              <p class="header-variant-title">{{ t('theme.darkVariant') }}</p>
              <div class="header-variant-grid">
                <button
                  v-for="opt in DARK_VARIANT_OPTIONS"
                  :key="opt.value"
                  type="button"
                  class="header-variant-chip"
                  :class="{ active: themeStore.darkVariant === opt.value }"
                  @click="themeStore.setDarkVariant(opt.value)"
                >
                  <span class="header-variant-swatch" :style="{ background: opt.swatch }" />
                  {{ t(opt.labelKey) }}
                </button>
              </div>
            </div>
          </el-popover>
        </div>
      </el-header>
      <el-main class="main">
        <RouterView />
      </el-main>
    </el-container>

    <ChangePasswordDialog v-model="showChangePassword" />
  </el-container>
</template>

<style scoped>
.layout {
  height: 100%;
  height: 100dvh;
  width: 100%;
  display: flex;
  overflow: hidden;
  background: var(--cf-bg);
}

.main-wrap {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.header {
  position: sticky;
  top: 0;
  z-index: 50;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  height: 52px;
  background: var(--apple-glass, rgba(255, 255, 255, 0.72));
  backdrop-filter: var(--apple-glass-blur, saturate(180%) blur(20px));
  -webkit-backdrop-filter: var(--apple-glass-blur, saturate(180%) blur(20px));
  border-bottom: 1px solid var(--apple-glass-border, rgba(0, 0, 0, 0.06));
  padding: 0 24px 0 32px;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.header-theme-btn {
  width: 36px;
  height: 36px;
  color: var(--cf-text-muted);
}

.header-theme-btn:hover {
  color: var(--cf-orange);
  background: rgba(246, 130, 31, 0.08);
}

.header-locale-btn {
  min-width: 36px;
  padding: 0 8px;
}

.header-locale-label {
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

:deep(.el-dropdown-menu__item.active) {
  color: var(--cf-orange);
  font-weight: 600;
}

.header-variant-dot {
  display: block;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  border: 2px solid rgba(255, 255, 255, 0.25);
}

.header-variant-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.header-variant-title {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
}

.header-variant-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}

.header-variant-chip {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid var(--cf-border);
  border-radius: 10px;
  background: var(--cf-bg);
  color: var(--cf-text);
  cursor: pointer;
  font-size: 12px;
}

.header-variant-chip.active,
.header-variant-chip:hover {
  border-color: var(--cf-orange);
}

.header-variant-swatch {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  flex-shrink: 0;
}

.page-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--cf-text-muted);
  letter-spacing: 0.02em;
  text-transform: uppercase;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.main {
  flex: 1;
  min-height: 0;
  overflow: auto;
  background: var(--cf-bg);
  width: 100%;
  max-width: none;
  padding: 24px 28px 32px;
}

.main :deep(> *) {
  width: 100%;
  max-width: none;
  box-sizing: border-box;
}

:deep(.el-dropdown-menu__item.active) {
  color: var(--cf-orange);
  font-weight: 600;
}
</style>
