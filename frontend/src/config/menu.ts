export interface MenuItem {
  path: string
  titleKey: string
  icon: string
  admin?: boolean
  perm?: string
  externalUrl?: string
}

export interface MenuGroup {
  titleKey: string
  items: MenuItem[]
}

export function menuItemForPath(path: string): MenuItem | null {
  let match: MenuItem | null = null
  let matchLen = -1
  for (const group of menuGroups) {
    for (const item of group.items) {
      if (path === item.path || path.startsWith(item.path + '/')) {
        if (item.path.length > matchLen) {
          match = item
          matchLen = item.path.length
        }
      }
    }
  }
  return match
}

export function menuGroupForPath(path: string): MenuGroup | null {
  let match: MenuGroup | null = null
  let matchLen = -1
  for (const group of menuGroups) {
    for (const item of group.items) {
      if (path === item.path || path.startsWith(item.path + '/')) {
        if (item.path.length > matchLen) {
          match = group
          matchLen = item.path.length
        }
      }
    }
  }
  return match
}

export function canAccessMenuItem(item: MenuItem, role?: string, permissions?: string): boolean {
  if (item.admin && role !== 'admin') {
    if (item.path === '/terminal' && role === 'subuser') {
      try {
        const p = JSON.parse(permissions || '{}')
        return !!p.bastion
      } catch {
        return false
      }
    }
    return false
  }
  if (!role || role === 'admin') return true
  if (role === 'user') return !item.admin
  if (role !== 'subuser') return false
  if (!item.perm) return false
  try {
    const p = JSON.parse(permissions || '{}')
    return !!p[item.perm]
  } catch {
    return false
  }
}

export const menuGroups: MenuGroup[] = [
  {
    titleKey: 'menuGroup.home',
    items: [{ path: '/dashboard', titleKey: 'menu.dashboard', icon: 'HomeFilled' }],
  },
  {
    titleKey: 'menuGroup.website',
    items: [
      { path: '/websites', titleKey: 'menu.website', icon: 'Link', perm: 'websites' },
      { path: '/wordpress', titleKey: 'menu.wpToolkit', icon: 'Reading', perm: 'websites' },
      { path: '/runtimes', titleKey: 'menu.runtimes', icon: 'Platform', perm: 'websites' },
      { path: '/ssl', titleKey: 'menu.ssl', icon: 'Lock', perm: 'websites' },
    ],
  },
  {
    titleKey: 'menuGroup.ftp',
    items: [{ path: '/ftp', titleKey: 'menu.ftp', icon: 'Upload', perm: 'ftp' }],
  },
  {
    titleKey: 'menuGroup.database',
    items: [{ path: '/databases', titleKey: 'menu.database', icon: 'Coin', perm: 'databases' }],
  },
  {
    titleKey: 'menuGroup.docker',
    items: [
      { path: '/docker', titleKey: 'menu.docker', icon: 'Box', perm: 'docker' },
      { path: '/compose', titleKey: 'menu.compose', icon: 'Grid', perm: 'docker' },
    ],
  },
  {
    titleKey: 'menuGroup.automation',
    items: [
      { path: '/auto-ops', titleKey: 'menu.autoOps', icon: 'Refresh', perm: 'monitor' },
      { path: '/product-analytics', titleKey: 'menu.abTesting', icon: 'DataAnalysis', perm: 'monitor' },
      { path: '/uptime', titleKey: 'menu.uptime', icon: 'Bell', perm: 'monitor' },
      { path: '/cron', titleKey: 'menu.cron', icon: 'Timer', perm: 'backup' },
      { path: '/backup', titleKey: 'menu.backup', icon: 'FolderOpened', perm: 'backup' },
      { path: '/devops', titleKey: 'menu.devops', icon: 'Promotion', admin: true },
      { path: '/cluster', titleKey: 'menu.cluster', icon: 'Share', perm: 'monitor' },
      { path: '/k8s', titleKey: 'menu.k8s', icon: 'Platform', perm: 'monitor' },
      { path: '/enterprise', titleKey: 'menu.enterprise', icon: 'OfficeBuilding', admin: true },
      { path: '/logs', titleKey: 'menu.logs', icon: 'Document', admin: true },
      { path: '/extensions', titleKey: 'menu.extensions', icon: 'Box', admin: true },
      { path: '/protection', titleKey: 'menu.protection', icon: 'Histogram', perm: 'websites' },
    ],
  },
  {
    titleKey: 'menuGroup.mail',
    items: [{ path: '/mail', titleKey: 'menu.mail', icon: 'Message', perm: 'mail' }],
  },
  {
    titleKey: 'menuGroup.files',
    items: [
      { path: '/files', titleKey: 'menu.files', icon: 'Folder', perm: 'files' },
      { path: '/oss', titleKey: 'menu.oss', icon: 'UploadFilled', perm: 'files' },
    ],
  },
  {
    titleKey: 'menuGroup.logs',
    items: [{ path: '/logs', titleKey: 'menu.logs', icon: 'Document', admin: true }],
  },
  {
    titleKey: 'menuGroup.domains',
    items: [{ path: '/dns', titleKey: 'menu.dns', icon: 'Compass', admin: true }],
  },
  {
    titleKey: 'menuGroup.ai',
    items: [
      { path: '/ai', titleKey: 'menu.aiHub', icon: 'MagicStick', admin: true },
    ],
  },
  {
    titleKey: 'menuGroup.appStore',
    items: [
      { path: '/software', titleKey: 'menu.software', icon: 'ShoppingCart', admin: true },
    ],
  },
  {
    titleKey: 'menuGroup.terminal',
    items: [
      { path: '/terminal', titleKey: 'menu.terminal', icon: 'Monitor', admin: true },
    ],
  },
  {
    titleKey: 'menuGroup.tools',
    items: [
      { path: '/php', titleKey: 'menu.php', icon: 'Coffee', perm: 'websites' },
      { path: '/toolbox', titleKey: 'menu.toolbox', icon: 'Tools', admin: true },
    ],
  },
  {
    titleKey: 'menuGroup.settings',
    items: [
        { path: '/settings', titleKey: 'menu.panelSettings', icon: 'Setting', admin: true },
        { path: '/users', titleKey: 'menu.users', icon: 'User', admin: true },
    ],
  },
]
