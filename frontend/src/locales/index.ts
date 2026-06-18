import { createI18n } from 'vue-i18n'

import zhCN from './zh-CN'

import zhTW from './zh-TW'

import en from './en'



export type LocaleCode = 'zh-CN' | 'zh-TW' | 'en'



export const LOCALE_OPTIONS: { label: string; short: string; value: LocaleCode }[] = [

  { label: 'English', short: 'EN', value: 'en' },

  { label: '简体中文', short: '简', value: 'zh-CN' },

  { label: '繁體中文', short: '繁', value: 'zh-TW' },

]



const STORAGE_KEY = 'owpanel-locale'



/** Map browser / OS language tags to supported panel locales. */

export function detectBrowserLocale(): LocaleCode {

  const candidates = [...(navigator.languages || []), navigator.language].filter(Boolean)

  for (const raw of candidates) {

    const tag = raw.toLowerCase().replace(/_/g, '-')

    if (tag.startsWith('zh')) {

      if (

        tag.includes('-tw') ||

        tag.includes('-hk') ||

        tag.includes('-mo') ||

        tag.includes('-hant') ||

        tag === 'zh-hant'

      ) {

        return 'zh-TW'

      }

      if (

        tag.includes('-cn') ||

        tag.includes('-sg') ||

        tag.includes('-hans') ||

        tag === 'zh-hans' ||

        tag === 'zh'

      ) {

        return 'zh-CN'

      }

      // Generic Chinese (e.g. zh) — default to Simplified

      return 'zh-CN'

    }

    if (tag.startsWith('en')) {

      return 'en'

    }

  }

  return 'en'

}



export function getSavedLocale(): LocaleCode {

  const saved = localStorage.getItem(STORAGE_KEY) as LocaleCode | null

  if (saved === 'zh-CN' || saved === 'zh-TW' || saved === 'en') return saved

  return detectBrowserLocale()

}



export function saveLocale(locale: LocaleCode) {

  localStorage.setItem(STORAGE_KEY, locale)

}



export function isChineseLocale(code: string): boolean {

  return code === 'zh-CN' || code === 'zh-TW'

}



/** Map UI locale to backend content lang (toolbox health/snippets, etc.). */
export function apiContentLang(code: string): 'en' | 'zh' {
  if (code.startsWith('en')) return 'en'
  return 'zh'
}



export function htmlLang(code: LocaleCode): string {

  return code

}



const i18n = createI18n({

  legacy: false,

  locale: getSavedLocale(),

  fallbackLocale: 'en',

  messages: {

    'zh-CN': zhCN,

    'zh-TW': zhTW,

    en,

  },

})



export default i18n



export function categoryLabel(category: string, t: (key: string) => string): string {

  const map: Record<string, string> = {

    'Web服务器': 'software.category.web',

    'Web Server': 'software.category.web',

    'Web伺服器': 'software.category.web',

    '数据库': 'software.category.database',

    'Database': 'software.category.database',

    '資料庫': 'software.category.database',

    '运行环境': 'software.category.runtime',

    'Runtime': 'software.category.runtime',

    '執行環境': 'software.category.runtime',

    'FTP': 'software.category.ftp',

    '系统工具': 'software.category.tools',

    'System Tools': 'software.category.tools',

    '系統工具': 'software.category.tools',

    '容器': 'software.category.container',

    'Container': 'software.category.container',

    '安全': 'software.category.security',

    'Security': 'software.category.security',

    '人工智能': 'software.category.ai',

    'Artificial Intelligence': 'software.category.ai',

    'AI': 'software.category.ai',

    '建站': 'software.category.website',

    '网站': 'software.category.website',

    '網站': 'software.category.website',

    '工具': 'software.category.tools',

    '邮件服务': 'software.category.email',

    '郵件服務': 'software.category.email',

    '云存储': 'software.category.storage',

    '雲儲存': 'software.category.storage',

    'DevOps': 'software.category.devops',

    '中间件': 'software.category.middleware',

    '中間件': 'software.category.middleware',

    'BI': 'software.category.bi',

    '开发工具': 'software.category.devtools',

    '開發工具': 'software.category.devtools',

    '多媒体': 'software.category.media',

    '多媒體': 'software.category.media',

    '图形处理': 'software.category.imageProcessing',

    '圖形處理': 'software.category.imageProcessing',

    'Image Processing': 'software.category.imageProcessing',

    '视频处理': 'software.category.videoProcessing',

    '視頻處理': 'software.category.videoProcessing',

    'Video Processing': 'software.category.videoProcessing',

    '生活': 'software.category.lifestyle',

    '邮件': 'software.category.email',

    '郵件': 'software.category.email',

    '服务器': 'software.category.server',

    '伺服器': 'software.category.server',

    '游戏': 'software.category.game',

    '遊戲': 'software.category.game',

    'CRM': 'software.category.crm',

    '存储': 'software.category.storage',

    '儲存': 'software.category.storage',

    '应用': 'software.category.apps',

    '應用': 'software.category.apps',

  }

  const key = map[category]

  return key ? t(key) : category

}


