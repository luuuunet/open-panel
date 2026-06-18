export type ThemeMode = 'system' | 'light' | 'dark'
export type DarkThemeVariant = 'navy' | 'charcoal' | 'midnight' | 'slate'

export interface ThemePrefs {
  mode: ThemeMode
  darkVariant: DarkThemeVariant
}

export const THEME_STORAGE_KEY = 'owpanel-theme'

export const DEFAULT_THEME_PREFS: ThemePrefs = {
  mode: 'system',
  darkVariant: 'navy',
}

export const THEME_MODE_OPTIONS: { value: ThemeMode; labelKey: string }[] = [
  { value: 'system', labelKey: 'theme.modeSystem' },
  { value: 'light', labelKey: 'theme.modeLight' },
  { value: 'dark', labelKey: 'theme.modeDark' },
]

export const DARK_VARIANT_OPTIONS: { value: DarkThemeVariant; labelKey: string; swatch: string }[] = [
  { value: 'navy', labelKey: 'theme.variantCloudflare', swatch: '#161616' },
  { value: 'charcoal', labelKey: 'theme.variantCharcoal', swatch: '#141414' },
  { value: 'midnight', labelKey: 'theme.variantMidnight', swatch: '#121418' },
  { value: 'slate', labelKey: 'theme.variantSlate', swatch: '#151a21' },
]

export function loadThemePrefs(): ThemePrefs {
  try {
    const raw = localStorage.getItem(THEME_STORAGE_KEY)
    if (!raw) return { ...DEFAULT_THEME_PREFS }
    const parsed = JSON.parse(raw) as Partial<ThemePrefs>
    return {
      mode: parsed.mode === 'light' || parsed.mode === 'dark' || parsed.mode === 'system' ? parsed.mode : DEFAULT_THEME_PREFS.mode,
      darkVariant:
        parsed.darkVariant === 'charcoal' ||
        parsed.darkVariant === 'midnight' ||
        parsed.darkVariant === 'slate' ||
        parsed.darkVariant === 'navy'
          ? parsed.darkVariant
          : DEFAULT_THEME_PREFS.darkVariant,
    }
  } catch {
    return { ...DEFAULT_THEME_PREFS }
  }
}

export function saveThemePrefs(prefs: ThemePrefs) {
  localStorage.setItem(THEME_STORAGE_KEY, JSON.stringify(prefs))
}

export function resolveTheme(mode: ThemeMode, systemDark: boolean): 'light' | 'dark' {
  if (mode === 'dark') return 'dark'
  if (mode === 'light') return 'light'
  return systemDark ? 'dark' : 'light'
}

export function applyThemeToDOM(resolved: 'light' | 'dark', darkVariant: DarkThemeVariant) {
  const root = document.documentElement
  root.setAttribute('data-theme', resolved)
  root.setAttribute('data-theme-variant', darkVariant)
  root.classList.toggle('dark', resolved === 'dark')
  root.style.colorScheme = resolved
}
