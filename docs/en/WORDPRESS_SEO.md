# WordPress Search Engine Push (SEO Push)

OWPanel can **automatically or manually** submit your WordPress sitemap to Google, Bing, IndexNow (Bing/Yandex/Naver, etc.), Baidu, and Yandex to speed up indexing.

> **Where:** **Apps → WordPress** → **SEO push** column or the 📣 action button.

---

## What it does

| Feature | Description |
|---------|-------------|
| **Push after deploy** | Auto-submit sitemap when WordPress goes live (on by default) |
| **Push now** | Re-submit after publishing or site changes |
| **Multi-engine** | Toggle Google / Bing / IndexNow / Baidu / Yandex independently |
| **Auto sitemap** | Detects `/wp-sitemap.xml`, then `sitemap_index.xml`, `sitemap.xml` |
| **IndexNow key** | Auto-generated; `{key}.txt` written to site root for verification |

---

## Quick start

1. **Deploy WordPress** with **SEO push** enabled on the deploy form (default: on).
2. Ensure the domain resolves, SSL is recommended, and status is **running**.
3. Open **SEO push** settings → **Push now** → review logs.

---

## Search engines

### Google

Sitemap ping is attempted but **deprecated by Google**. Always add your property and sitemap in [Google Search Console](https://search.google.com/search-console).

### Bing

Standard sitemap ping via `bing.com/ping`.

### IndexNow (recommended)

One request notifies **Bing, Yandex, Naver, Seznam**, etc. OWPanel creates the key file at `https://your-domain/{key}.txt` and POSTs to `api.indexnow.org`.

### Baidu

For China indexing. Optional push token from [Baidu Webmaster](https://ziyuan.baidu.com/).

### Yandex

Sitemap ping for Yandex Webmaster.

---

## Tips

- Override **Sitemap URL** if an SEO plugin uses a custom path.
- Turn off **Push after deploy** for manual-only pushes.
- OWPanel does **not** replace Search Console / Webmaster **site verification**.

---

## API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/wordpress/:id/seo-push` | Get settings |
| PUT | `/api/wordpress/:id/seo-push` | Update settings |
| POST | `/api/wordpress/:id/seo-push` | Push now |

---

See also: [User Guide](./USER_GUIDE.md), [Automation Guide](./AUTOMATION.md).
