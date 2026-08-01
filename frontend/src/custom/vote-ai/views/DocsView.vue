<template>
  <div class="docs-page" :class="{ 'theme-dark': isDark }">
    <header class="site-header">
      <nav class="site-nav" aria-label="文档页导航">
        <router-link to="/home" class="brand"><span class="brand-logo"><img :src="VOTE_AI_LOGO_URL" alt="" /></span><span>Vote AI</span></router-link>
        <div class="nav-links">
          <router-link to="/home">{{ copy.nav.home }}</router-link>
          <router-link to="/pricing">{{ copy.nav.pricing }}</router-link>
          <router-link to="/home#faq">{{ copy.nav.faq }}</router-link>
          <router-link to="/docs" class="active">{{ copy.nav.docs }}</router-link>
        </div>
        <div class="nav-actions">
          <LocaleSwitcher />
          <button type="button" class="icon-button" :title="isDark ? copy.light : copy.dark" @click="toggleTheme"><Icon :name="isDark ? 'sun' : 'moon'" size="md" /></button>
          <router-link v-if="isAuthenticated" :to="dashboardPath" class="text-action">{{ copy.console }}</router-link>
          <template v-else><router-link to="/login" class="text-action">{{ copy.login }}</router-link><router-link to="/register" class="primary-action">{{ copy.register }}</router-link></template>
          <button type="button" class="mobile-menu-button" :aria-label="copy.menu" @click="mobileMenuOpen = !mobileMenuOpen"><Icon :name="mobileMenuOpen ? 'x' : 'menu'" size="md" /></button>
        </div>
      </nav>
      <div v-if="mobileMenuOpen" class="mobile-menu">
        <router-link to="/home" @click="mobileMenuOpen = false">{{ copy.nav.home }}</router-link><router-link to="/pricing" @click="mobileMenuOpen = false">{{ copy.nav.pricing }}</router-link><router-link to="/home#faq" @click="mobileMenuOpen = false">{{ copy.nav.faq }}</router-link><router-link to="/docs" class="active" @click="mobileMenuOpen = false">{{ copy.nav.docs }}</router-link>
        <div class="mobile-tools"><LocaleSwitcher /><button type="button" @click="toggleTheme"><Icon :name="isDark ? 'sun' : 'moon'" size="md" />{{ isDark ? copy.light : copy.dark }}</button></div>
        <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="mobile-primary">{{ isAuthenticated ? copy.console : copy.login }}</router-link>
      </div>
    </header>

    <main class="docs-layout">
      <aside class="docs-sidebar" :class="{ open: directoryOpen }">
        <button type="button" class="directory-toggle" @click="directoryOpen = !directoryOpen"><span>{{ copy.directory }}</span><Icon :name="directoryOpen ? 'chevronUp' : 'chevronDown'" size="sm" /></button>
        <div class="directory-body">
          <div class="sidebar-heading"><strong>{{ copy.directory }}</strong><button v-if="isAdmin" type="button" class="add-button" :disabled="loading || saving" @click="openNew"><Icon name="plus" size="sm" />{{ copy.add }}</button></div>
          <nav class="article-list">
            <div v-for="(doc, index) in visibleDocs" :key="doc.id" class="article-row" :class="{ active: activeDoc?.id === doc.id }">
              <router-link :to="`/docs/${doc.slug}`" @click="directoryOpen = false"><span>{{ localized(doc.title) }}</span><small v-if="isAdmin && !doc.published">{{ copy.draft }}</small></router-link>
              <div v-if="isAdmin" class="row-actions">
                <button type="button" :title="copy.up" :disabled="saving || index === 0" @click="moveDoc(doc.id, -1)"><Icon name="arrowUp" size="xs" /></button>
                <button type="button" :title="copy.down" :disabled="saving || index === visibleDocs.length - 1" @click="moveDoc(doc.id, 1)"><Icon name="arrowDown" size="xs" /></button>
                <button type="button" :title="copy.edit" :disabled="saving" @click="openEdit(doc)"><Icon name="edit" size="xs" /></button>
                <button type="button" :title="copy.remove" :disabled="saving" @click="requestDelete(doc)"><Icon name="trash" size="xs" /></button>
              </div>
            </div>
          </nav>
        </div>
      </aside>

      <article class="docs-article">
        <div v-if="loading" class="empty-state">{{ copy.loading }}</div>
        <div v-else-if="activeDoc" class="article-meta"><span>{{ copy.updated }}</span><span v-if="isAdmin" :class="['status-badge', { draft: !activeDoc.published }]">{{ activeDoc.published ? copy.published : copy.draft }}</span><button v-if="isAdmin" type="button" :disabled="saving" @click="openEdit(activeDoc)"><Icon name="edit" size="sm" />{{ copy.edit }}</button></div>
        <MarkdownContent v-if="!loading && activeDoc" :content="localized(activeDoc.content)" :copy-label="copy.copyCode" :copied-label="copy.copied" />
        <div v-else-if="!loading" class="empty-state">{{ copy.empty }}</div>
      </article>
    </main>

    <BaseDialog :show="editorOpen" :title="editingId ? copy.editDoc : copy.addDoc" width="full" @close="editorOpen = false">
      <div class="editor-grid">
        <div class="editor-form">
          <Input v-model="form.slug" label="Slug" required :error="errors.slug" placeholder="quick-start" />
          <div class="two-columns"><Input v-model="form.title.zh" :label="copy.zhTitle" required :error="errors.zhTitle" /><Input v-model="form.title.en" :label="copy.enTitle" required :error="errors.enTitle" /></div>
          <TextArea v-model="form.content.zh" :label="copy.zhContent" required :rows="12" :error="errors.zhContent" />
          <TextArea v-model="form.content.en" :label="copy.enContent" required :rows="12" :error="errors.enContent" />
          <label class="publish-toggle"><Toggle v-model="form.published" /><span>{{ copy.publish }}</span></label>
        </div>
        <div class="editor-preview"><div class="preview-label">{{ copy.preview }}</div><MarkdownContent :content="localized(form.content)" :copy-label="copy.copyCode" :copied-label="copy.copied" /></div>
      </div>
      <template #footer><div class="dialog-actions"><button type="button" class="cancel-button" :disabled="saving" @click="editorOpen = false">{{ copy.cancel }}</button><button type="button" class="save-button" :disabled="saving" @click="saveDoc">{{ saving ? copy.saving : copy.save }}</button></div></template>
    </BaseDialog>

    <ConfirmDialog v-if="!saving" :show="deleteOpen" :title="copy.deleteTitle" :message="copy.deleteMessage" :confirm-text="copy.remove" :cancel-text="copy.cancel" danger @confirm="deleteDoc" @cancel="deleteOpen = false" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore, useAuthStore } from '@/stores'
import { getAdminDocs, getPublishedDocs, saveAdminDocs } from '../api/docs'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Input from '@/components/common/Input.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import TextArea from '@/components/common/TextArea.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import MarkdownContent from '../components/MarkdownContent.vue'
import { VOTE_AI_LOGO_URL } from '../branding'

type LocalizedText = { zh: string; en: string }
interface DocArticle { id: string; slug: string; published: boolean; title: LocalizedText; content: LocalizedText }

const zhCopy = { nav: { home: '首页', pricing: '价格', faq: '常见问题', docs: '文档' }, light: '日间模式', dark: '夜间模式', login: '登录', register: '注册', console: '控制台', menu: '打开导航', directory: '文章目录', add: '新增', draft: '草稿', published: '已发布', up: '上移', down: '下移', edit: '编辑', remove: '删除', updated: 'Vote AI 接入文档', loading: '正在加载文档…', empty: '暂无可查看的文档', addDoc: '新增文档', editDoc: '编辑文档', zhTitle: '中文标题', enTitle: '英文标题', zhContent: '中文 Markdown 正文', enContent: '英文 Markdown 正文', publish: '发布此文档', preview: '实时预览', cancel: '取消', save: '保存', saving: '保存中…', saved: '文档已保存', loadFailed: '文档加载失败', saveFailed: '文档保存失败，请重试', deleteTitle: '删除文档', deleteMessage: '确定删除这篇文档吗？', copyCode: '复制', copied: '已复制', required: '此项必填', slugInvalid: '仅支持小写字母、数字和连字符', slugExists: 'Slug 已存在' }
const enCopy = { nav: { home: 'Home', pricing: 'Pricing', faq: 'FAQ', docs: 'Docs' }, light: 'Light mode', dark: 'Dark mode', login: 'Sign in', register: 'Register', console: 'Console', menu: 'Open navigation', directory: 'Articles', add: 'Add', draft: 'Draft', published: 'Published', up: 'Move up', down: 'Move down', edit: 'Edit', remove: 'Delete', updated: 'Vote AI integration docs', loading: 'Loading documentation…', empty: 'No documentation is available.', addDoc: 'Add document', editDoc: 'Edit document', zhTitle: 'Chinese title', enTitle: 'English title', zhContent: 'Chinese Markdown content', enContent: 'English Markdown content', publish: 'Publish this document', preview: 'Live preview', cancel: 'Cancel', save: 'Save', saving: 'Saving…', saved: 'Documentation saved', loadFailed: 'Failed to load documentation', saveFailed: 'Failed to save documentation. Please retry.', deleteTitle: 'Delete document', deleteMessage: 'Delete this document?', copyCode: 'Copy', copied: 'Copied', required: 'Required', slugInvalid: 'Use lowercase letters, numbers, and hyphens only', slugExists: 'Slug already exists' }

const { locale } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()
const docs = ref<DocArticle[]>([])
const loading = ref(true)
const saving = ref(false)
let loadSequence = 0
const isDark = ref(document.documentElement.classList.contains('dark'))
const mobileMenuOpen = ref(false)
const directoryOpen = ref(false)
const editorOpen = ref(false)
const deleteOpen = ref(false)
const editingId = ref('')
const deletingId = ref('')
const form = reactive<DocArticle>(emptyDoc())
const errors = reactive({ slug: '', zhTitle: '', enTitle: '', zhContent: '', enContent: '' })

const copy = computed(() => locale.value.startsWith('zh') ? zhCopy : enCopy)
const isAdmin = computed(() => authStore.isAuthenticated && authStore.isAdmin)
const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() => authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
const visibleDocs = computed(() => isAdmin.value ? docs.value : docs.value.filter(doc => doc.published))
const activeDoc = computed(() => visibleDocs.value.find(doc => doc.slug === route.params.slug) || visibleDocs.value[0])

function localized(value: LocalizedText) { return locale.value.startsWith('zh') ? value.zh : value.en }
function emptyDoc(): DocArticle { return { id: '', slug: '', published: false, title: { zh: '', en: '' }, content: { zh: '', en: '' } } }
function cloneDoc(doc: DocArticle): DocArticle { return JSON.parse(JSON.stringify(doc)) }
function cloneDocList(value: DocArticle[]) { return value.map(cloneDoc) }
function errorMessage(error: unknown, fallback: string) {
  return typeof error === 'object' && error && 'message' in error && typeof error.message === 'string' ? error.message : fallback
}
async function loadDocs(admin = isAdmin.value) {
  const sequence = ++loadSequence
  loading.value = true
  try {
    const loaded = admin ? await getAdminDocs() : await getPublishedDocs()
    if (sequence === loadSequence) docs.value = cloneDocList(loaded)
  } catch (error) {
    if (sequence === loadSequence) appStore.showError(errorMessage(error, copy.value.loadFailed))
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}
async function persist(nextDocs: DocArticle[], previousDocs: DocArticle[]) {
  if (saving.value) return false
  saving.value = true
  docs.value = nextDocs
  try {
    const savedDocs = await saveAdminDocs(cloneDocList(nextDocs))
    if (Array.isArray(savedDocs)) docs.value = cloneDocList(savedDocs)
    appStore.showSuccess(copy.value.saved)
    return true
  } catch (error) {
    docs.value = previousDocs
    appStore.showError(errorMessage(error, copy.value.saveFailed))
    return false
  } finally {
    saving.value = false
  }
}
function syncRoute() {
  if (!activeDoc.value) return
  const slug = typeof route.params.slug === 'string' ? route.params.slug : ''
  if (slug !== activeDoc.value.slug) router.replace(`/docs/${activeDoc.value.slug}`)
}
function toggleTheme() { isDark.value = !isDark.value; document.documentElement.classList.toggle('dark', isDark.value); localStorage.setItem('theme', isDark.value ? 'dark' : 'light') }
function openNew() { Object.assign(form, emptyDoc()); editingId.value = ''; clearErrors(); editorOpen.value = true }
function openEdit(doc: DocArticle) { Object.assign(form, cloneDoc(doc)); editingId.value = doc.id; clearErrors(); editorOpen.value = true }
function clearErrors() { Object.keys(errors).forEach(key => { errors[key as keyof typeof errors] = '' }) }
function validate() {
  clearErrors()
  if (!form.slug.trim()) errors.slug = copy.value.required
  else if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(form.slug)) errors.slug = copy.value.slugInvalid
  else if (docs.value.some(doc => doc.slug === form.slug && doc.id !== editingId.value)) errors.slug = copy.value.slugExists
  if (!form.title.zh.trim()) errors.zhTitle = copy.value.required
  if (!form.title.en.trim()) errors.enTitle = copy.value.required
  if (!form.content.zh.trim()) errors.zhContent = copy.value.required
  if (!form.content.en.trim()) errors.enContent = copy.value.required
  return !Object.values(errors).some(Boolean)
}
async function saveDoc() {
  if (saving.value) return
  form.slug = form.slug.trim()
  if (!validate()) return
  const previousDocs = cloneDocList(docs.value)
  const nextDocs = cloneDocList(docs.value)
  if (editingId.value) {
    const index = nextDocs.findIndex(doc => doc.id === editingId.value)
    if (index >= 0) nextDocs[index] = cloneDoc(form)
  } else {
    form.id = `doc-${Date.now()}`
    nextDocs.push(cloneDoc(form))
  }
  if (await persist(nextDocs, previousDocs)) {
    editorOpen.value = false
    router.replace(`/docs/${form.slug}`)
  }
}
function requestDelete(doc: DocArticle) { deletingId.value = doc.id; deleteOpen.value = true }
async function deleteDoc() {
  if (saving.value) return
  const previousDocs = cloneDocList(docs.value)
  const nextDocs = docs.value.filter(doc => doc.id !== deletingId.value)
  if (await persist(nextDocs, previousDocs)) {
    deleteOpen.value = false
    syncRoute()
  }
}
async function moveDoc(id: string, direction: number) {
  if (saving.value) return
  const previousDocs = cloneDocList(docs.value)
  const nextDocs = cloneDocList(docs.value)
  const from = nextDocs.findIndex(doc => doc.id === id)
  const to = from + direction
  if (from < 0 || to < 0 || to >= nextDocs.length) return
  const [item] = nextDocs.splice(from, 1)
  nextDocs.splice(to, 0, item)
  await persist(nextDocs, previousDocs)
}

watch([() => route.params.slug, visibleDocs], syncRoute, { immediate: true })
watch(isAdmin, admin => { if (!saving.value) loadDocs(admin) }, { immediate: true })
onMounted(() => {
  authStore.checkAuth()
  const saved = localStorage.getItem('theme')
  if (saved === 'dark' || (!saved && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
  if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings()
})
</script>

<style scoped>
.docs-page { min-height: 100vh; color: #211b17; background: #fcf9f4; }
.docs-page.theme-dark { color-scheme: dark; color: #f8f2ed; background: #1c1c19; }
:global(html.dark body) { background: #1c1c19; }
a { text-decoration: none; }
.site-header { position: sticky; z-index: 40; top: 0; border-bottom: 1px solid rgba(88,66,56,.1); background: rgba(252,249,244,.94); backdrop-filter: blur(18px); }
.theme-dark .site-header { border-color: #4d4843; background: rgba(28,28,25,.94); }
.site-nav { display: grid; grid-template-columns: auto 1fr auto; align-items: center; width: min(100% - 48px,1280px); min-height: 76px; margin: auto; gap: 36px; }
.brand { display: inline-flex; align-items: center; gap: 10px; color: inherit; font-size: 18px; font-weight: 750; }
.brand-logo { width: 34px; height: 34px; overflow: hidden; border-radius: 10px; background: white; box-shadow: 0 5px 16px rgba(62,22,0,.1); }.brand-logo img { width: 100%; height: 100%; object-fit: contain; }
.nav-links { display: flex; justify-content: center; gap: 30px; }.nav-links a { color: #6f655e; font-size: 14px; font-weight: 600; }.nav-links a:hover,.nav-links .active { color: #9c3f00; }.nav-links .active { font-weight: 800; }.theme-dark .nav-links a { color: #c4beb7; }.theme-dark .nav-links a:hover,.theme-dark .nav-links .active { color: #ffb594; }
.nav-actions { display: flex; align-items: center; gap: 10px; }.icon-button,.mobile-menu-button { display: grid; width: 38px; height: 38px; place-items: center; border: 0; border-radius: 10px; color: inherit; background: transparent; cursor: pointer; }.text-action { padding: 9px; color: inherit; font-size: 14px; font-weight: 650; }.primary-action { padding: 10px 18px; border-radius: 8px; color: white; background: #9c3f00; font-size: 14px; font-weight: 750; }.mobile-menu-button,.mobile-menu { display: none; }
.local-notice { display: flex; align-items: center; justify-content: center; gap: 8px; min-height: 40px; padding: 8px 20px; color: #81421f; background: #fff0e5; font-size: 13px; }.theme-dark .local-notice { color: #ffd1b5; background: #482719; }
.docs-layout { display: grid; grid-template-columns: 276px minmax(0,1fr); width: min(100% - 48px,1180px); margin: 0 auto; }
.docs-sidebar { position: sticky; top: 76px; height: calc(100vh - 76px); padding: 36px 24px 36px 0; border-right: 1px solid #e5dbd4; overflow-y: auto; }.theme-dark .docs-sidebar { border-color: #48433f; }.sidebar-heading { display: flex; align-items: center; justify-content: space-between; margin-bottom: 15px; font-size: 13px; text-transform: uppercase; letter-spacing: .07em; }.add-button { display: inline-flex; align-items: center; gap: 4px; border: 0; color: #9c3f00; background: none; font-size: 12px; cursor: pointer; }.directory-toggle { display: none; }
.article-list { display: grid; gap: 6px; }.article-row { border-radius: 8px; }.article-row > a { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 10px 12px; color: #665a53; font-size: 14px; font-weight: 600; }.article-row > a small { padding: 2px 7px; border-radius: 99px; color: #98522b; background: #f8e2d5; font-size: 10px; }.article-row:hover,.article-row.active { background: #f5ebe4; }.article-row.active > a { color: #9c3f00; }.theme-dark .article-row > a { color: #c7bdb6; }.theme-dark .article-row:hover,.theme-dark .article-row.active { background: #342b26; }.theme-dark .article-row.active > a { color: #ffb594; }
.row-actions { display: flex; justify-content: flex-end; gap: 2px; padding: 0 8px 6px; }.row-actions button { display: grid; width: 26px; height: 24px; place-items: center; border: 0; border-radius: 5px; color: #7d7068; background: transparent; cursor: pointer; }.row-actions button:hover:not(:disabled) { color: #9c3f00; background: #fff; }.row-actions button:disabled { opacity: .3; cursor: default; }
.docs-article { width: min(100%,820px); min-width: 0; padding: 58px 32px 100px 72px; }.article-meta { display: flex; align-items: center; gap: 10px; margin-bottom: 16px; color: #887b73; font-size: 12px; }.article-meta button { display: inline-flex; align-items: center; gap: 5px; margin-left: auto; border: 0; color: #9c3f00; background: none; cursor: pointer; }.status-badge { padding: 3px 8px; border-radius: 99px; color: #287247; background: #e5f4ea; }.status-badge.draft { color: #8c532d; background: #f6e6da; }.empty-state { padding: 100px 0; color: #887b73; text-align: center; }
.editor-grid { display: grid; grid-template-columns: minmax(0,1fr) minmax(0,1fr); gap: 22px; }.editor-form { display: grid; gap: 15px; }.two-columns { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }.publish-toggle { display: flex; align-items: center; gap: 10px; font-size: 14px; }.editor-preview { min-width: 0; max-height: 70vh; padding: 20px; border: 1px solid #e2d8d1; border-radius: 10px; overflow-y: auto; }.preview-label { margin-bottom: 18px; color: #8b7e76; font-size: 12px; font-weight: 750; text-transform: uppercase; letter-spacing: .08em; }.dialog-actions { display: flex; justify-content: flex-end; gap: 10px; }.cancel-button,.save-button { padding: 9px 18px; border-radius: 7px; font-size: 14px; font-weight: 650; cursor: pointer; }.cancel-button { border: 1px solid #d7cbc3; background: white; }.save-button { border: 1px solid #9c3f00; color: white; background: #9c3f00; }
@media (max-width: 900px) { .site-nav { gap: 18px; }.nav-links { gap: 16px; }.primary-action { display: none; }.docs-article { padding-left: 42px; }.editor-grid { grid-template-columns: 1fr; }.editor-preview { max-height: none; } }
@media (max-width: 767px) {
  .site-nav { display: flex; justify-content: space-between; width: min(100% - 32px,1280px); min-height: 68px; }.nav-links,.nav-actions > :not(.mobile-menu-button) { display: none; }.mobile-menu-button { display: grid; }.mobile-menu { display: flex; flex-direction: column; gap: 3px; padding: 16px 20px 22px; border-top: 1px solid #e5dbd4; }.mobile-menu > a { padding: 10px 4px; color: inherit; font-size: 16px; font-weight: 700; }.mobile-menu .active { color: #9c3f00; }.mobile-tools { display: flex; align-items: center; justify-content: space-between; padding-top: 12px; border-top: 1px solid #e5dbd4; }.mobile-tools button { display: inline-flex; align-items: center; gap: 7px; border: 0; color: inherit; background: none; }.mobile-primary { margin-top: 8px; padding: 12px !important; border-radius: 8px; color: white !important; background: #9c3f00; text-align: center; }
  .docs-layout { display: block; width: 100%; }.docs-sidebar { position: sticky; z-index: 20; top: 68px; width: 100%; height: auto; padding: 0; border: 0; border-bottom: 1px solid #e5dbd4; background: #fcf9f4; overflow: visible; }.theme-dark .docs-sidebar { background: #1c1c19; }.directory-toggle { display: flex; align-items: center; justify-content: space-between; width: 100%; padding: 13px 18px; border: 0; color: inherit; background: transparent; font-weight: 700; }.directory-body { display: none; max-height: 56vh; padding: 8px 14px 18px; overflow-y: auto; }.docs-sidebar.open .directory-body { display: block; }.docs-article { width: 100%; padding: 36px 20px 76px; }.local-notice { justify-content: flex-start; }.two-columns { grid-template-columns: 1fr; }
}
@media (max-width: 390px) { .site-nav { width: calc(100% - 24px); }.brand { font-size: 16px; }.brand-logo { width: 31px; height: 31px; }.docs-article { padding-inline: 16px; }.article-meta { flex-wrap: wrap; } }
</style>
