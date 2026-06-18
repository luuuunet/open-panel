<script setup lang="ts">

import { computed, onMounted, ref, watch } from 'vue'

import { useRoute } from 'vue-router'

import { useI18n } from 'vue-i18n'

import api from '@/api'

import { ElMessage, ElMessageBox } from 'element-plus'

import {
  Folder, Document, Upload, FolderAdd, DocumentAdd, Refresh, EditPen, Delete,
  Search, Star, StarFilled, Download, CopyDocument, Rank, Picture, DocumentCopy, Link,
} from '@element-plus/icons-vue'

import FileEditorWithChat from '@/components/FileEditorWithChat.vue'
import { resolveSiteForPath } from '@/utils/siteFromPath'



const { t } = useI18n()

const route = useRoute()



const dirPath = ref('')

const pathInput = ref('')

const entries = ref<any[]>([])

const roots = ref<any[]>([])

const editorVisible = ref(false)

const editingPath = ref('')

const fileContent = ref('')

const websites = ref<Array<{ id: number; root_path?: string; domain?: string }>>([])

const editingSiteContext = computed(() => {
  if (!editingPath.value) return null
  return resolveSiteForPath(editingPath.value, websites.value)
})

async function loadWebsitesForEditor() {
  if (websites.value.length) return
  try {
    const res: any = await api.get('/websites')
    websites.value = res.data || []
  } catch {
    /* ignore */
  }
}



const renameVisible = ref(false)

const renameTarget = ref<any>(null)

const renameName = ref('')



const createVisible = ref(false)

const createName = ref('')

const createIsDir = ref(false)



const permVisible = ref(false)

const permTarget = ref<any>(null)

const permMode = ref('0644')
const permRecursive = ref(false)

const selectedRows = ref<any[]>([])
const compressVisible = ref(false)
const compressFormat = ref('zip')
const compressDest = ref('')

const viewMode = ref<'files' | 'trash'>('files')
const trashItems = ref<any[]>([])
const trashLoading = ref(false)

const searchQuery = ref('')
const searchActive = ref(false)
const searchLoading = ref(false)
const searchResults = ref<any[]>([])

const FAV_STORAGE_KEY = 'owpanel-file-favorites'
const favorites = ref<{ label: string; path: string }[]>([])

const transferVisible = ref(false)
const transferMode = ref<'copy' | 'move'>('copy')
const transferDest = ref('')

const sizeVisible = ref(false)
const sizeTarget = ref<any>(null)
const sizeValue = ref<number | null>(null)
const sizeLoading = ref(false)

const previewVisible = ref(false)
const previewName = ref('')
const previewUrl = ref('')

const urlDownloadVisible = ref(false)
const urlDownloadLoading = ref(false)
const urlDownloadForm = ref({ url: '', filename: '' })

const displayedEntries = computed(() => {
  if (searchActive.value) return searchResults.value
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return entries.value
  return entries.value.filter((e) => e.name.toLowerCase().includes(q))
})

const isCurrentFavorite = computed(() =>
  favorites.value.some((f) => f.path === dirPath.value),
)

function loadFavorites() {
  try {
    const raw = localStorage.getItem(FAV_STORAGE_KEY)
    favorites.value = raw ? JSON.parse(raw) : []
  } catch {
    favorites.value = []
  }
}

function saveFavorites() {
  localStorage.setItem(FAV_STORAGE_KEY, JSON.stringify(favorites.value))
}

function toggleFavorite() {
  if (!dirPath.value) return
  const idx = favorites.value.findIndex((f) => f.path === dirPath.value)
  if (idx >= 0) {
    favorites.value.splice(idx, 1)
    ElMessage.success(t('files.favoriteRemoved'))
  } else {
    const label = breadcrumbs.value.length ? breadcrumbs.value[breadcrumbs.value.length - 1].label : dirPath.value
    favorites.value.push({ label, path: dirPath.value })
    ElMessage.success(t('files.favoriteAdded'))
  }
  saveFavorites()
}

function isImageName(name: string) {
  const n = name.toLowerCase()
  return ['.jpg', '.jpeg', '.png', '.gif', '.webp', '.svg', '.bmp', '.ico'].some((ext) => n.endsWith(ext))
}

function apiDownloadUrl(subpath: string) {
  const token = localStorage.getItem('token')
  const w = window as Window & { __OWPANEL_BASE__?: string }
  const base = w.__OWPANEL_BASE__ || '/'
  const prefix = base.endsWith('/') ? base : base + '/'
  return { url: `${prefix}api/v1${subpath}`, token }
}

async function downloadFile(path: string, name?: string) {
  const { url, token } = apiDownloadUrl(`/files/download?path=${encodeURIComponent(path)}`)
  const res = await fetch(url, { headers: token ? { Authorization: `Bearer ${token}` } : {} })
  if (!res.ok) {
    ElMessage.error(t('files.downloadFailed'))
    return
  }
  const blob = await res.blob()
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = name || path.split(/[/\\]/).pop() || 'download'
  a.click()
  URL.revokeObjectURL(a.href)
}

async function downloadBatch() {
  if (selectedRows.value.length === 0) {
    ElMessage.warning(t('files.selectFirst'))
    return
  }
  const paths = selectedRows.value.map((r) => r.path)
  if (paths.length === 1 && !selectedRows.value[0].is_dir) {
    await downloadFile(paths[0], selectedRows.value[0].name)
    return
  }
  try {
    const { url, token } = apiDownloadUrl('/files/download-batch')
    const res = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify({ paths }),
    })
    if (!res.ok) {
      ElMessage.error(t('files.downloadFailed'))
      return
    }
    const blob = await res.blob()
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = 'download.zip'
    a.click()
    URL.revokeObjectURL(a.href)
  } catch {
    ElMessage.error(t('files.downloadFailed'))
  }
}

async function runRecursiveSearch() {
  const q = searchQuery.value.trim()
  if (!q) {
    searchActive.value = false
    searchResults.value = []
    return
  }
  searchLoading.value = true
  try {
    const res: any = await api.get('/files/search', { params: { path: dirPath.value, q } })
    searchResults.value = res.data || []
    searchActive.value = true
  } catch (e: any) {
    ElMessage.error(e?.error || t('files.loadFailed'))
  } finally {
    searchLoading.value = false
  }
}

function clearSearch() {
  searchQuery.value = ''
  searchActive.value = false
  searchResults.value = []
}

function openTransfer(mode: 'copy' | 'move') {
  if (selectedRows.value.length === 0) {
    ElMessage.warning(t('files.selectFirst'))
    return
  }
  transferMode.value = mode
  transferDest.value = dirPath.value
  transferVisible.value = true
}

async function confirmTransfer() {
  const paths = selectedRows.value.map((r) => r.path)
  const dest = transferDest.value.trim()
  if (!dest) return
  const endpoint = transferMode.value === 'copy' ? '/files/copy' : '/files/move'
  await api.post(endpoint, { paths, dest })
  ElMessage.success(transferMode.value === 'copy' ? t('files.transferCopied') : t('files.transferMoved'))
  transferVisible.value = false
  loadDir(dirPath.value, editorVisible.value)
}

async function duplicateEntry(row: any) {
  await api.post('/files/duplicate', { path: row.path })
  ElMessage.success(t('files.duplicated'))
  loadDir(dirPath.value, editorVisible.value)
}

async function openFolderSize(row: any) {
  sizeTarget.value = row
  sizeValue.value = null
  sizeVisible.value = true
  sizeLoading.value = true
  try {
    const res: any = await api.get('/files/size', { params: { path: row.path } })
    sizeValue.value = res.data?.size ?? 0
  } catch (e: any) {
    ElMessage.error(e?.error || t('files.loadFailed'))
    sizeVisible.value = false
  } finally {
    sizeLoading.value = false
  }
}

async function openImagePreview(row: any) {
  previewName.value = row.name
  if (previewUrl.value) URL.revokeObjectURL(previewUrl.value)
  previewUrl.value = ''
  previewVisible.value = true
  try {
    const { url, token } = apiDownloadUrl(`/files/download?path=${encodeURIComponent(row.path)}`)
    const res = await fetch(url, { headers: token ? { Authorization: `Bearer ${token}` } : {} })
    if (!res.ok) {
      ElMessage.error(t('files.openFailed'))
      previewVisible.value = false
      return
    }
    const blob = await res.blob()
    previewUrl.value = URL.createObjectURL(blob)
  } catch {
    ElMessage.error(t('files.openFailed'))
    previewVisible.value = false
  }
}

function closePreview() {
  previewVisible.value = false
  if (previewUrl.value) {
    URL.revokeObjectURL(previewUrl.value)
    previewUrl.value = ''
  }
}

function isArchiveName(name: string) {
  const n = name.toLowerCase()
  return n.endsWith('.zip') || n.endsWith('.tar.gz') || n.endsWith('.tgz')
}

function onSelectionChange(rows: any[]) {
  selectedRows.value = rows
}

function openCompress() {
  if (selectedRows.value.length === 0) {
    ElMessage.warning(t('files.selectFirst'))
    return
  }
  const sep = dirPath.value.includes('\\') ? '\\' : '/'
  const base = selectedRows.value.length === 1 ? selectedRows.value[0].name : 'archive'
  compressDest.value = dirPath.value.replace(/[\\/]+$/, '') + sep + base + (compressFormat.value === 'zip' ? '.zip' : '.tar.gz')
  compressVisible.value = true
}

async function confirmCompress() {
  const paths = selectedRows.value.map((r) => r.path)
  await api.post('/files/compress', { paths, format: compressFormat.value, dest: compressDest.value })
  ElMessage.success(t('files.compressed'))
  compressVisible.value = false
  loadDir(dirPath.value, editorVisible.value)
}

async function extractEntry(row: any) {
  await api.post('/files/extract', { path: row.path, dest_dir: dirPath.value })
  ElMessage.success(t('files.extracted'))
  loadDir(dirPath.value, editorVisible.value)
}



const editorTitle = computed(() => {

  if (!editingPath.value) return t('files.editFile')

  const parts = editingPath.value.replace(/\\/g, '/').split('/')

  return parts[parts.length - 1] || t('files.editFile')

})



const breadcrumbs = computed(() => {

  const p = dirPath.value.replace(/\\/g, '/')

  if (!p) return []

  const parts = p.split('/').filter(Boolean)

  if (p.match(/^[A-Za-z]:/)) {

    const drive = parts[0]

    const crumbs = [{ label: drive, path: drive + '\\' }]

    let acc = drive + '\\'

    for (let i = 1; i < parts.length; i++) {

      acc = acc.endsWith('\\') ? acc + parts[i] : acc + '\\' + parts[i]

      crumbs.push({ label: parts[i], path: acc })

    }

    return crumbs

  }

  const crumbs = [{ label: '/', path: '/' }]

  let acc = ''

  for (const part of parts) {

    acc += '/' + part

    crumbs.push({ label: part, path: acc })

  }

  return crumbs

})



function formatSize(n: number) {

  if (n == null) return '-'

  if (n < 1024) return `${n} B`

  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`

  return `${(n / 1024 / 1024).toFixed(1)} MB`

}



function formatTime(ts: number) {

  if (!ts) return '-'

  return new Date(ts * 1000).toLocaleString()

}



async function loadRoots() {

  const res: any = await api.get('/files/roots')

  roots.value = res.data?.roots || []

  if (!dirPath.value && res.data?.default_root) {

    dirPath.value = res.data.default_root

    pathInput.value = dirPath.value

  }

}



async function loadDir(path?: string, keepEditor = false) {

  const target = path ?? dirPath.value

  try {

    const res: any = await api.get('/files', { params: { path: target } })

    entries.value = res.data || []

    dirPath.value = target

    pathInput.value = target

    searchActive.value = false

    searchResults.value = []

    if (!keepEditor) {

      editorVisible.value = false

      editingPath.value = ''

      fileContent.value = ''

    }

  } catch (e: any) {

    ElMessage.error(e?.error || e?.message || t('files.loadFailed'))

  }

}



function goPath() {

  loadDir(pathInput.value.trim())

}



async function openFile(path: string) {
  try {
    void loadWebsitesForEditor()
    const info: any = await api.get('/files/info', { params: { path } })
    if (info.data?.is_dir) {
      ElMessage.warning(t('files.isDirectory'))
      await loadDir(path, editorVisible.value)
      return
    }
    const res: any = await api.get('/files/content', { params: { path } })
    fileContent.value = res.data?.content ?? ''
    editingPath.value = path
    editorVisible.value = true
  } catch (e: any) {
    ElMessage.error(e?.error || t('files.openFailed'))
  }
}



function closeEditor() {

  editorVisible.value = false

  editingPath.value = ''

  fileContent.value = ''

}



async function saveFile() {

  await api.put('/files/content', { path: editingPath.value, content: fileContent.value })

  ElMessage.success(t('files.saved'))

  loadDir(dirPath.value, true)

}


async function loadTrash() {
  trashLoading.value = true
  try {
    const res: any = await api.get('/files/trash')
    trashItems.value = res.data || []
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('files.loadFailed'))
  } finally {
    trashLoading.value = false
  }
}

function switchView(mode: 'files' | 'trash') {
  viewMode.value = mode
  if (mode === 'trash') {
    loadTrash()
  } else {
    loadDir(dirPath.value, editorVisible.value)
  }
}

async function restoreTrash(row: any) {
  await ElMessageBox.confirm(
    t('files.restoreConfirm', { name: row.name, path: row.original_path }),
    t('common.warning'),
    { type: 'info' },
  )
  await api.post(`/files/trash/${row.id}/restore`)
  ElMessage.success(t('files.restoreSuccess'))
  loadTrash()
}

async function deleteTrashPermanent(row: any) {
  await ElMessageBox.confirm(
    t('files.deletePermanentConfirm', { name: row.name }),
    t('common.warning'),
    { type: 'warning' },
  )
  await api.delete(`/files/trash/${row.id}`)
  ElMessage.success(t('files.deletePermanentSuccess'))
  loadTrash()
}

async function emptyTrash() {
  if (trashItems.value.length === 0) return
  await ElMessageBox.confirm(t('files.emptyTrashConfirm'), t('common.warning'), { type: 'warning' })
  await api.post('/files/trash/empty')
  ElMessage.success(t('files.emptyTrashSuccess'))
  loadTrash()
}


async function deleteEntry(row: any) {

  await ElMessageBox.confirm(t('files.deleteConfirmTrash', { name: row.name }), t('common.warning'), { type: 'warning' })

  await api.delete('/files', { params: { path: row.path } })

  ElMessage.success(t('files.movedToTrash'))

  if (editingPath.value === row.path) {

    closeEditor()

  }

  loadDir(dirPath.value)

}

async function deleteBatch() {
  if (selectedRows.value.length === 0) return
  const n = selectedRows.value.length
  await ElMessageBox.confirm(t('files.deleteBatchConfirm', { n }), t('common.warning'), { type: 'warning' })
  const paths = selectedRows.value.map((r) => r.path)
  const { data } = await api.post('/files/delete-batch', { paths })
  const moved = data?.moved ?? 0
  const failed = data?.failed ?? 0
  if (failed > 0) {
    ElMessage.warning(t('files.deleteBatchPartial', { moved, failed }))
  } else {
    ElMessage.success(t('files.deleteBatchSuccess', { n: moved }))
  }
  selectedRows.value = []
  loadDir(dirPath.value, editorVisible.value)
}



function goUp() {

  const sep = dirPath.value.includes('\\') ? '\\' : '/'

  const parts = dirPath.value.replace(/[\\/]+$/, '').split(/[\\/]/)

  parts.pop()

  let parent = parts.join(sep) || sep

  if (dirPath.value.match(/^[A-Za-z]:\\/) && parts.length === 1) {

    parent = parts[0] + '\\'

  }

  loadDir(parent, editorVisible.value)

}



function openRename(row: any) {

  renameTarget.value = row

  renameName.value = row.name

  renameVisible.value = true

}



async function confirmRename() {

  if (!renameTarget.value || !renameName.value.trim()) return

  await api.post('/files/rename', { path: renameTarget.value.path, new_name: renameName.value.trim() })

  ElMessage.success(t('files.renamed'))

  renameVisible.value = false

  loadDir(dirPath.value, editorVisible.value)

}



function openCreate(isDir: boolean) {

  createIsDir.value = isDir

  createName.value = ''

  createVisible.value = true

}



async function confirmCreate() {

  const name = createName.value.trim()

  if (!name) return

  const sep = dirPath.value.includes('\\') ? '\\' : '/'

  const full = dirPath.value.replace(/[\\/]+$/, '') + sep + name

  await api.post('/files/create', { path: full, is_dir: createIsDir.value, content: '' })

  ElMessage.success(t('files.created'))

  createVisible.value = false

  const createdFile = !createIsDir.value

  await loadDir(dirPath.value, editorVisible.value)

  if (createdFile) {

    await openFile(full)

  }

}



function openPerm(row: any) {

  permTarget.value = row

  permMode.value = row.mode || '0644'

  permRecursive.value = false

  permVisible.value = true

}



async function confirmPerm() {

  if (!permTarget.value) return

  const res: any = await api.patch('/files/permissions', {
    path: permTarget.value.path,
    mode: permMode.value,
    recursive: permRecursive.value,
  })

  if (permRecursive.value && res?.data) {
    const { updated = 0, failed = 0 } = res.data
    ElMessage.success(t('files.permUpdatedRecursive', { updated, failed }))
  } else {
    ElMessage.success(t('files.permUpdated'))
  }

  permVisible.value = false

  loadDir(dirPath.value, editorVisible.value)

}



async function uploadRequest(opt: any) {

  const fd = new FormData()

  fd.append('file', opt.file)

  fd.append('path', dirPath.value)

  try {

    await api.post('/files/upload', fd, { headers: { 'Content-Type': 'multipart/form-data' } })

    ElMessage.success(t('files.uploaded'))

    loadDir(dirPath.value, editorVisible.value)

    opt.onSuccess?.({})

  } catch (e: any) {

    ElMessage.error(e?.error || t('files.uploadFailed'))

    opt.onError?.(e)

  }

}

function openUrlDownload() {
  urlDownloadForm.value = { url: '', filename: '' }
  urlDownloadVisible.value = true
}

async function confirmUrlDownload() {
  const url = urlDownloadForm.value.url.trim()
  if (!url) {
    ElMessage.warning(t('files.downloadUrlRequired'))
    return
  }
  if (!dirPath.value) {
    ElMessage.warning(t('files.downloadUrlNoDir'))
    return
  }
  urlDownloadLoading.value = true
  try {
    await api.post('/files/download-url', {
      url,
      path: dirPath.value,
      filename: urlDownloadForm.value.filename.trim(),
    })
    ElMessage.success(t('files.downloadFromUrlSuccess'))
    urlDownloadVisible.value = false
    loadDir(dirPath.value, editorVisible.value)
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('files.downloadFromUrlFailed'))
  } finally {
    urlDownloadLoading.value = false
  }
}



async function navigateToQueryPath() {
  const q = route.query.path
  if (typeof q !== 'string' || !q.trim()) return
  try {
    const info: any = await api.get('/files/info', { params: { path: q } })
    if (info.data?.is_dir) {
      await loadDir(q)
      return
    }
    const parent = q.replace(/[\\/][^\\/]+$/, '')
    await loadDir(parent || dirPath.value)
    await openFile(q)
  } catch {
    await loadDir(q)
  }
}

onMounted(async () => {
  loadFavorites()
  void loadWebsitesForEditor()
  await loadRoots()
  if (route.query.path) {
    await navigateToQueryPath()
  } else {
    await loadDir(dirPath.value)
  }
})

watch(
  () => route.query.path,
  async (path) => {
    if (typeof path === 'string' && path.trim()) {
      await navigateToQueryPath()
    }
  },
)

</script>



<template>

  <div class="files-page">

    <div class="page-header">

      <h2>{{ t('files.title') }}</h2>

      <el-radio-group v-model="viewMode" class="view-tabs" @change="switchView">
        <el-radio-button :value="'files'">{{ t('files.tabFiles') }}</el-radio-button>
        <el-radio-button :value="'trash'">{{ t('files.tabTrash') }}</el-radio-button>
      </el-radio-group>

    </div>



    <template v-if="viewMode === 'files'">

    <el-card class="toolbar-card">

      <div class="toolbar">

        <el-select :placeholder="t('files.quickRoot')" style="width: 160px" @change="loadDir">

          <el-option v-for="r in roots" :key="r.path" :label="r.label" :value="r.path" />

        </el-select>

        <el-input v-model="pathInput" class="path-input" :placeholder="t('files.pathPlaceholder')" @keyup.enter="goPath">

          <template #append>

            <el-button @click="goPath">{{ t('files.go') }}</el-button>

          </template>

        </el-input>

        <el-button :icon="Refresh" @click="loadDir(undefined, editorVisible)">{{ t('common.refresh') }}</el-button>

        <el-upload :show-file-list="false" :http-request="uploadRequest" multiple>

          <el-button type="primary" :icon="Upload">{{ t('files.upload') }}</el-button>

        </el-upload>

        <el-button :icon="Link" @click="openUrlDownload">{{ t('files.downloadFromUrl') }}</el-button>

        <el-button :icon="DocumentAdd" @click="openCreate(false)">{{ t('files.newFile') }}</el-button>

        <el-button :icon="FolderAdd" @click="openCreate(true)">{{ t('files.newFolder') }}</el-button>

        <el-button :disabled="selectedRows.length === 0" @click="openCompress">{{ t('files.compress') }}</el-button>

        <el-button :disabled="selectedRows.length === 0" :icon="Download" @click="downloadBatch">{{ t('files.downloadBatch') }}</el-button>

        <el-button :disabled="selectedRows.length === 0" :icon="CopyDocument" @click="openTransfer('copy')">{{ t('files.copy') }}</el-button>

        <el-button :disabled="selectedRows.length === 0" :icon="Rank" @click="openTransfer('move')">{{ t('files.move') }}</el-button>

        <el-button type="danger" :disabled="selectedRows.length === 0" :icon="Delete" @click="deleteBatch">{{ t('files.deleteBatch') }}</el-button>

        <el-input
          v-model="searchQuery"
          class="search-input"
          clearable
          :placeholder="t('files.searchPlaceholder')"
          @clear="clearSearch"
          @keyup.enter="runRecursiveSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
          <template #append>
            <el-button :loading="searchLoading" @click="runRecursiveSearch">{{ t('files.searchRecursive') }}</el-button>
          </template>
        </el-input>

        <el-button :icon="isCurrentFavorite ? StarFilled : Star" :type="isCurrentFavorite ? 'warning' : 'default'" @click="toggleFavorite">
          {{ isCurrentFavorite ? t('files.removeFavorite') : t('files.addFavorite') }}
        </el-button>

        <el-dropdown v-if="favorites.length" trigger="click">
          <el-button>{{ t('files.favorites') }}</el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item v-for="f in favorites" :key="f.path" @click="loadDir(f.path, editorVisible)">
                <span class="fav-label">{{ f.label }}</span>
                <span class="fav-path mono">{{ f.path }}</span>
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>

      </div>

      <div v-if="searchActive" class="search-banner">
        <span>{{ t('files.searchResults', { n: searchResults.length }) }}</span>
        <el-button link type="primary" @click="clearSearch">{{ t('files.clearSearch') }}</el-button>
      </div>

      <el-breadcrumb separator="/" class="crumb">

        <el-breadcrumb-item v-for="(c, i) in breadcrumbs" :key="i">

          <a @click="loadDir(c.path, editorVisible)">{{ c.label }}</a>

        </el-breadcrumb-item>

      </el-breadcrumb>

    </el-card>



    <el-card>

      <div class="list-actions">

        <el-button size="small" @click="goUp">{{ t('files.parent') }}</el-button>

      </div>

      <el-table

        :data="displayedEntries"

        stripe

        highlight-current-row

        max-height="620"

        @row-dblclick="(row: any) => row.is_dir ? loadDir(row.path, editorVisible) : (isImageName(row.name) ? openImagePreview(row) : openFile(row.path))"

        @selection-change="onSelectionChange"

      >

        <el-table-column type="selection" width="48" />

        <el-table-column :label="t('files.name')" min-width="220">

          <template #default="{ row }">

            <el-icon class="file-icon"><Folder v-if="row.is_dir" /><Document v-else /></el-icon>

            <span>{{ row.name }}</span>

          </template>

        </el-table-column>

        <el-table-column :label="t('files.size')" width="100">

          <template #default="{ row }">{{ row.is_dir ? '-' : formatSize(row.size) }}</template>

        </el-table-column>

        <el-table-column prop="mode" :label="t('files.permission')" width="90" />

        <el-table-column :label="t('files.modified')" width="170">

          <template #default="{ row }">{{ formatTime(row.mod_time) }}</template>

        </el-table-column>

        <el-table-column :label="t('common.actions')" width="520" fixed="right">

          <template #default="{ row }">

            <el-button v-if="!row.is_dir && isImageName(row.name)" size="small" :icon="Picture" @click="openImagePreview(row)">{{ t('files.preview') }}</el-button>

            <el-button v-if="!row.is_dir" size="small" :icon="Download" @click="downloadFile(row.path, row.name)">{{ t('files.download') }}</el-button>

            <el-button v-if="!row.is_dir && isArchiveName(row.name)" size="small" @click="extractEntry(row)">{{ t('files.extract') }}</el-button>

            <el-button

              v-if="!row.is_dir"

              type="primary"

              size="small"

              :icon="EditPen"

              @click="openFile(row.path)"

            >

              {{ t('files.editFile') }}

            </el-button>

            <el-button v-else size="small" @click="loadDir(row.path, editorVisible)">{{ t('files.openDir') }}</el-button>

            <el-button v-if="row.is_dir" size="small" @click="openFolderSize(row)">{{ t('files.folderSize') }}</el-button>

            <el-button size="small" :icon="DocumentCopy" @click="duplicateEntry(row)">{{ t('files.duplicate') }}</el-button>

            <el-button size="small" @click="openRename(row)">{{ t('files.rename') }}</el-button>

            <el-button size="small" @click="openPerm(row)">{{ t('files.permission') }}</el-button>

            <el-button size="small" type="danger" @click="deleteEntry(row)">{{ t('common.delete') }}</el-button>

          </template>

        </el-table-column>

      </el-table>

    </el-card>



    <el-drawer

      v-model="editorVisible"

      :title="editorTitle"

      size="92%"

      direction="rtl"

      destroy-on-close

      class="file-editor-drawer"

      @close="closeEditor"

    >

      <FileEditorWithChat

        v-if="editingPath"

        v-model="fileContent"

        :path="editingPath"

        :site-id="editingSiteContext?.siteId"

        :site-root="editingSiteContext?.siteRoot"

        :site-domain="editingSiteContext?.domain"

        @save="saveFile"

      />

    </el-drawer>



    <el-dialog v-model="renameVisible" :title="t('files.rename')" width="420px">

      <el-input v-model="renameName" :placeholder="t('files.newName')" />

      <template #footer>

        <el-button @click="renameVisible = false">{{ t('common.cancel') }}</el-button>

        <el-button type="primary" @click="confirmRename">{{ t('common.confirm') }}</el-button>

      </template>

    </el-dialog>



    <el-dialog v-model="createVisible" :title="createIsDir ? t('files.newFolder') : t('files.newFile')" width="420px">

      <el-input v-model="createName" :placeholder="t('files.newName')" />

      <template #footer>

        <el-button @click="createVisible = false">{{ t('common.cancel') }}</el-button>

        <el-button type="primary" @click="confirmCreate">{{ t('common.confirm') }}</el-button>

      </template>

    </el-dialog>



    <el-dialog v-model="compressVisible" :title="t('files.compressTitle')" width="480px">

      <el-form label-width="100px">

        <el-form-item :label="t('files.compressFormat')">

          <el-select v-model="compressFormat" style="width: 100%">

            <el-option label="ZIP" value="zip" />

            <el-option label="tar.gz" value="tar.gz" />

          </el-select>

        </el-form-item>

        <el-form-item :label="t('files.compressDest')">

          <el-input v-model="compressDest" />

        </el-form-item>

      </el-form>

      <template #footer>

        <el-button @click="compressVisible = false">{{ t('common.cancel') }}</el-button>

        <el-button type="primary" @click="confirmCompress">{{ t('files.compress') }}</el-button>

      </template>

    </el-dialog>



    <el-dialog v-model="urlDownloadVisible" :title="t('files.downloadFromUrlTitle')" width="520px">
      <el-form label-width="100px" @submit.prevent="confirmUrlDownload">
        <el-form-item :label="t('files.path')">
          <span class="mono">{{ dirPath || '—' }}</span>
        </el-form-item>
        <el-form-item :label="t('files.downloadUrl')" required>
          <el-input
            v-model="urlDownloadForm.url"
            :placeholder="t('files.downloadUrlPlaceholder')"
            clearable
          />
        </el-form-item>
        <el-form-item :label="t('files.downloadUrlFilename')">
          <el-input
            v-model="urlDownloadForm.filename"
            :placeholder="t('files.downloadUrlFilenamePlaceholder')"
            clearable
          />
        </el-form-item>
        <p class="form-hint">{{ t('files.downloadUrlHint') }}</p>
      </el-form>
      <template #footer>
        <el-button @click="urlDownloadVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="urlDownloadLoading" @click="confirmUrlDownload">
          {{ t('files.downloadFromUrl') }}
        </el-button>
      </template>
    </el-dialog>



    <el-dialog
      v-model="transferVisible"
      :title="t('files.transferTitle', { action: transferMode === 'copy' ? t('files.copy') : t('files.move') })"
      width="480px"
    >
      <el-form label-width="100px">
        <el-form-item :label="t('files.transferDest')">
          <el-input v-model="transferDest" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="transferVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="confirmTransfer">{{ t('common.confirm') }}</el-button>
      </template>
    </el-dialog>



    <el-dialog v-model="sizeVisible" :title="t('files.folderSizeTitle')" width="420px">
      <p class="mono">{{ sizeTarget?.path }}</p>
      <p v-if="sizeLoading">{{ t('files.calculating') }}</p>
      <p v-else-if="sizeValue != null">{{ t('files.folderSizeResult', { size: formatSize(sizeValue) }) }}</p>
      <template #footer>
        <el-button type="primary" @click="sizeVisible = false">{{ t('common.confirm') }}</el-button>
      </template>
    </el-dialog>



    <el-dialog v-model="previewVisible" :title="previewName || t('files.previewImage')" width="720px" @close="closePreview">
      <div class="preview-wrap">
        <img v-if="previewUrl" :src="previewUrl" :alt="previewName" class="preview-img" />
        <el-empty v-else :description="t('files.calculating')" />
      </div>
    </el-dialog>



    <el-dialog v-model="permVisible" :title="t('files.editPermission')" width="420px">

      <el-form label-width="80px">

        <el-form-item :label="t('files.path')">

          <span class="mono">{{ permTarget?.path }}</span>

        </el-form-item>

        <el-form-item :label="t('files.permission')">

          <el-input v-model="permMode" placeholder="0644 / 0755" />

          <div class="hint">{{ t('files.permHint') }}</div>

        </el-form-item>

        <el-form-item v-if="permTarget?.is_dir">

          <el-checkbox v-model="permRecursive">{{ t('files.permRecursive') }}</el-checkbox>

        </el-form-item>

      </el-form>

      <template #footer>

        <el-button @click="permVisible = false">{{ t('common.cancel') }}</el-button>

        <el-button type="primary" @click="confirmPerm">{{ t('common.confirm') }}</el-button>

      </template>

    </el-dialog>

    </template>



    <el-card v-else v-loading="trashLoading">

      <div class="list-actions">

        <el-button size="small" :icon="Refresh" @click="loadTrash">{{ t('common.refresh') }}</el-button>

        <el-button size="small" type="danger" :icon="Delete" :disabled="trashItems.length === 0" @click="emptyTrash">

          {{ t('files.emptyTrash') }}

        </el-button>

      </div>

      <el-empty v-if="!trashLoading && trashItems.length === 0" :description="t('files.trashEmpty')" />

      <el-table v-else :data="trashItems" stripe max-height="620">

        <el-table-column :label="t('files.name')" min-width="160">

          <template #default="{ row }">

            <el-icon class="file-icon"><Folder v-if="row.is_dir" /><Document v-else /></el-icon>

            <span>{{ row.name }}</span>

          </template>

        </el-table-column>

        <el-table-column :label="t('files.originalPath')" min-width="280">

          <template #default="{ row }">

            <span class="mono">{{ row.original_path }}</span>

          </template>

        </el-table-column>

        <el-table-column :label="t('files.size')" width="100">

          <template #default="{ row }">{{ row.is_dir ? '-' : formatSize(row.size) }}</template>

        </el-table-column>

        <el-table-column :label="t('files.deletedAt')" width="170">

          <template #default="{ row }">{{ formatTime(row.deleted_at) }}</template>

        </el-table-column>

        <el-table-column prop="deleted_by" :label="t('files.deletedBy')" width="120" />

        <el-table-column :label="t('common.actions')" width="260" fixed="right">

          <template #default="{ row }">

            <el-button type="primary" size="small" @click="restoreTrash(row)">{{ t('files.restore') }}</el-button>

            <el-button type="danger" size="small" @click="deleteTrashPermanent(row)">{{ t('files.deletePermanent') }}</el-button>

          </template>

        </el-table-column>

      </el-table>

    </el-card>

  </div>

</template>



<style scoped>

.files-page { display: flex; flex-direction: column; gap: 16px; }

.page-header { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; }

.page-header h2 { margin: 0; }

.view-tabs { flex-shrink: 0; }

.toolbar-card :deep(.el-card__body) { padding-bottom: 12px; }

.toolbar { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; margin-bottom: 10px; }

.path-input { flex: 1; min-width: 240px; }

.search-input { flex: 1; min-width: 200px; max-width: 360px; }

.search-banner { display: flex; align-items: center; gap: 12px; font-size: 13px; margin-bottom: 8px; color: var(--el-text-color-secondary); }

.fav-label { display: block; font-weight: 500; }

.fav-path { display: block; font-size: 11px; color: var(--el-text-color-secondary); max-width: 280px; overflow: hidden; text-overflow: ellipsis; }

.preview-wrap { display: flex; justify-content: center; align-items: center; min-height: 200px; }

.preview-img { max-width: 100%; max-height: 70vh; object-fit: contain; }

.crumb { font-size: 13px; }

.file-icon { margin-right: 6px; vertical-align: middle; }

.list-actions { margin-bottom: 10px; }

.mono { font-family: monospace; font-size: 13px; word-break: break-all; }

.hint { font-size: 12px; color: var(--el-text-color-secondary); margin-top: 4px; }

:deep(.file-editor-drawer .el-drawer__body) {
  padding: 0 12px 12px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  height: calc(100% - 55px);
}

:deep(.file-editor-drawer .editor-with-chat) {
  flex: 1;
  min-height: 0;
}

</style>

