import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { canAccessMenuItem, menuItemForPath } from '@/config/menu'

function routerBase(): string {
  const w = window as Window & { __OWPANEL_BASE__?: string }
  const base = w.__OWPANEL_BASE__ || import.meta.env.BASE_URL || '/'
  return base.endsWith('/') ? base : base + '/'
}

const router = createRouter({
  history: createWebHistory(routerBase()),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      meta: { public: true },
    },
    {
      path: '/',
      component: () => import('@/layouts/MainLayout.vue'),
      redirect: '/dashboard',
      children: [
        { path: 'dashboard', name: 'dashboard', component: () => import('@/views/DashboardView.vue'), meta: { titleKey: 'page.dashboard' } },
        { path: 'app-store', redirect: { path: 'software', query: { tab: 'store' } } },
        { path: 'apps', redirect: { path: 'software', query: { tab: 'store' } } },
        { path: 'software-list', redirect: { path: 'software', query: { tab: 'installed' } } },
        { path: 'software', name: 'software', component: () => import('@/views/SoftwareManageView.vue'), meta: { titleKey: 'page.software' } },
        { path: 'docker', name: 'docker', component: () => import('@/views/DockerView.vue'), meta: { titleKey: 'page.docker' } },
        { path: 'compose', name: 'compose', component: () => import('@/views/ComposeView.vue'), meta: { titleKey: 'page.compose' } },
        { path: 'websites', name: 'websites', component: () => import('@/views/WebsitesView.vue'), meta: { titleKey: 'page.websites' } },
        { path: 'product-analytics', name: 'product-analytics', component: () => import('@/views/ProductAnalyticsView.vue'), meta: { titleKey: 'page.productAnalytics' } },
        { path: 'ssl', name: 'ssl', component: () => import('@/views/SSLView.vue'), meta: { titleKey: 'page.ssl' } },
        { path: 'ftp', name: 'ftp', component: () => import('@/views/FTPView.vue'), meta: { titleKey: 'page.ftp' } },
        { path: 'nginx', redirect: { path: 'protection', query: { tab: 'nginx' } } },
        { path: 'cache', redirect: { path: 'protection', query: { tab: 'cache' } } },
        { path: 'databases', name: 'databases', component: () => import('@/views/DatabasesView.vue'), meta: { titleKey: 'page.databases' } },
        { path: 'files', name: 'files', component: () => import('@/views/FilesView.vue'), meta: { titleKey: 'page.files' } },
        { path: 'oss', name: 'oss', component: () => import('@/views/OSSView.vue'), meta: { titleKey: 'page.oss' } },
        { path: 'uptime', name: 'uptime', component: () => import('@/views/UptimeView.vue'), meta: { titleKey: 'page.uptime' } },
        { path: 'cluster', name: 'cluster', component: () => import('@/views/ClusterView.vue'), meta: { titleKey: 'page.cluster' } },
        { path: 'k8s', name: 'k8s', component: () => import('@/views/K8sView.vue'), meta: { titleKey: 'page.k8s' } },
        { path: 'enterprise', name: 'enterprise', component: () => import('@/views/EnterpriseView.vue'), meta: { titleKey: 'page.enterprise', admin: true } },
        { path: 'auto-ops', name: 'auto-ops', component: () => import('@/views/AutoOpsView.vue'), meta: { titleKey: 'page.autoOps' } },
        { path: 'devops', name: 'devops', component: () => import('@/views/DevOpsView.vue'), meta: { titleKey: 'page.devops', admin: true } },
        { path: 'protection', name: 'protection', component: () => import('@/views/ProtectionCenterView.vue'), meta: { titleKey: 'page.protection' } },
        { path: 'ai', name: 'ai', component: () => import('@/views/AIHubView.vue'), meta: { titleKey: 'page.aiHub', admin: true } },
        { path: 'firewall', redirect: { path: 'protection', query: { tab: 'firewall' } } },
        { path: 'terminal', name: 'terminal', component: () => import('@/views/TerminalView.vue'), meta: { titleKey: 'page.terminal' } },
        { path: 'bastion', redirect: { path: 'terminal', query: { tab: 'pam' } } },
        { path: 'logs', name: 'logs', component: () => import('@/views/LogsView.vue'), meta: { titleKey: 'page.logs' } },
        { path: 'cron', name: 'cron', component: () => import('@/views/CronView.vue'), meta: { titleKey: 'page.cron' } },
        { path: 'backup', name: 'backup', component: () => import('@/views/BackupView.vue'), meta: { titleKey: 'page.backup' } },
        { path: 'php', name: 'php', component: () => import('@/views/PHPView.vue'), meta: { titleKey: 'page.php' } },
        { path: 'toolbox', name: 'toolbox', component: () => import('@/views/ToolboxView.vue'), meta: { titleKey: 'page.toolbox' } },
        { path: 'extensions', name: 'extensions', component: () => import('@/views/ExtensionsView.vue'), meta: { titleKey: 'page.extensions', admin: true } },
        { path: 'ext/:id', name: 'extension-embed', component: () => import('@/views/ExtensionEmbedView.vue'), meta: { titleKey: 'page.extensionEmbed' } },
        { path: 'settings', name: 'settings', component: () => import('@/views/SettingsView.vue'), meta: { titleKey: 'page.settings', admin: true } },
        { path: 'waf', redirect: { path: 'protection', query: { tab: 'waf' } } },
        { path: 'mail', name: 'mail', component: () => import('@/views/MailView.vue'), meta: { titleKey: 'page.mail' } },
        { path: 'dns', name: 'dns', component: () => import('@/views/DNSView.vue'), meta: { titleKey: 'page.dns' } },
        { path: 'wordpress', name: 'wordpress', component: () => import('@/views/WordPressView.vue'), meta: { titleKey: 'page.wordpress' } },
        { path: 'runtimes', name: 'runtimes', component: () => import('@/views/RuntimesView.vue'), meta: { titleKey: 'page.runtimes' } },
        { path: 'nodejs', redirect: { path: 'runtimes', query: { tab: 'nodejs' } } },
        { path: 'security', redirect: { path: 'protection', query: { tab: 'security' } } },
        { path: 'cilium', redirect: { path: 'protection', query: { tab: 'cilium' } } },
        { path: 'users', name: 'users', component: () => import('@/views/UsersView.vue'), meta: { titleKey: 'page.users', admin: true } },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  if (to.meta.public) return true

  const auth = useAuthStore()
  if (!auth.token) return '/login'

  if (!auth.user) {
    try {
      await auth.fetchMe()
    } catch {
      auth.logout()
      return '/login'
    }
  }

  if (to.meta.admin && auth.user?.role !== 'admin') {
    return '/dashboard'
  }

  const menuItem = menuItemForPath(to.path)
  if (menuItem && !canAccessMenuItem(menuItem, auth.user?.role, auth.user?.permissions)) {
    return '/dashboard'
  }

  if (auth.user?.must_change_password && to.name !== 'login' && to.path !== '/settings') {
    // allow staying on dashboard while change-password dialog is shown
    if (to.name !== 'dashboard') {
      return '/dashboard'
    }
  }

  return true
})

export default router
