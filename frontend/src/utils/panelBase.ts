/** Panel URL prefix when security entrance is enabled (e.g. `/bb276bbd/`). */
export function panelBase(): string {
  const w = window as Window & { __OWPANEL_BASE__?: string }
  const base = w.__OWPANEL_BASE__ || '/'
  return base.endsWith('/') ? base : base + '/'
}

/** Resolve a root-relative static path (e.g. `/software/nginx.svg`). */
export function panelStaticPath(path: string): string {
  const normalized = path.startsWith('/') ? path : '/' + path
  const prefix = panelBase().replace(/\/$/, '')
  if (prefix === '') {
    return normalized
  }
  return prefix + normalized
}
