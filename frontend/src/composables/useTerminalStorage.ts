import type { TerminalSession } from '@/types/terminal'
import { uniqueId } from '@/utils/uniqueId'

const SAVED_KEY = 'owpanel-terminal-saved-v1'
const PREFS_KEY = 'owpanel-terminal-prefs-v1'

export interface SavedConnection {
  id: string
  name: string
  host: string
  port: number
  user: string
  selectedTarget: string
  authMethod: 'password' | 'key'
  keyId: number | null
}

export interface TerminalPrefs {
  fontSize: number
  sidebarCollapsed: boolean
  connBarCollapsed: boolean
}

const defaultPrefs: TerminalPrefs = {
  fontSize: 14,
  sidebarCollapsed: false,
  connBarCollapsed: false,
}

export function loadSavedConnections(): SavedConnection[] {
  try {
    const raw = localStorage.getItem(SAVED_KEY)
    return raw ? JSON.parse(raw) : []
  } catch {
    return []
  }
}

export function persistSavedConnections(list: SavedConnection[]) {
  localStorage.setItem(SAVED_KEY, JSON.stringify(list))
}

export function loadPrefs(): TerminalPrefs {
  try {
    const raw = localStorage.getItem(PREFS_KEY)
    return raw ? { ...defaultPrefs, ...JSON.parse(raw) } : { ...defaultPrefs }
  } catch {
    return { ...defaultPrefs }
  }
}

export function persistPrefs(prefs: TerminalPrefs) {
  localStorage.setItem(PREFS_KEY, JSON.stringify(prefs))
}

export function sessionToSaved(s: TerminalSession, name?: string): SavedConnection {
  return {
    id: uniqueId(),
    name: name || s.title || `${s.user}@${s.host}`,
    host: s.host,
    port: s.port,
    user: s.user,
    selectedTarget: s.selectedTarget,
    authMethod: s.authMethod,
    keyId: s.keyId,
  }
}

export function applySaved(s: TerminalSession, saved: SavedConnection) {
  s.title = saved.name
  s.host = saved.host
  s.port = saved.port
  s.user = saved.user
  s.selectedTarget = saved.selectedTarget
  s.authMethod = saved.authMethod
  s.keyId = saved.keyId
  s.password = ''
  s.privateKeyPaste = ''
  s.keyPassphrase = ''
}
