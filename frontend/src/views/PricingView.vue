<template>
  <div class="pricing-page" :class="{ 'theme-dark': isDark }">
    <header class="site-header">
      <nav class="site-nav" aria-label="价格页导航">
        <router-link to="/home" class="brand" aria-label="返回 Vote AI 首页">
          <span class="brand-logo">
            <img :src="siteLogo || '/logo.png'" alt="" />
          </span>
          <span>Vote AI</span>
        </router-link>

        <div class="nav-links">
          <router-link to="/home">{{ copy.nav.home }}</router-link>
          <router-link to="/pricing" class="active">{{ copy.nav.pricing }}</router-link>
          <router-link to="/docs">{{ copy.nav.docs }}</router-link>
          <router-link to="/home#faq">{{ copy.nav.faq }}</router-link>
        </div>

        <div class="nav-actions">
          <LocaleSwitcher />
          <button type="button" class="icon-button" :title="isDark ? copy.actions.light : copy.actions.dark" @click="toggleTheme">
            <Icon :name="isDark ? 'sun' : 'moon'" size="md" />
          </button>
          <router-link v-if="isAuthenticated" :to="dashboardPath" class="text-action">{{ copy.actions.console }}</router-link>
          <template v-else>
            <router-link to="/login" class="text-action">{{ copy.actions.login }}</router-link>
            <router-link to="/register" class="primary-action">{{ copy.actions.start }}</router-link>
          </template>
          <button type="button" class="mobile-menu-button" :aria-label="copy.actions.menu" @click="mobileMenuOpen = !mobileMenuOpen">
            <Icon :name="mobileMenuOpen ? 'x' : 'menu'" size="md" />
          </button>
        </div>
      </nav>

      <div v-if="mobileMenuOpen" class="mobile-menu">
        <router-link to="/home" @click="mobileMenuOpen = false">{{ copy.nav.home }}</router-link>
        <router-link to="/pricing" class="active" @click="mobileMenuOpen = false">{{ copy.nav.pricing }}</router-link>
        <router-link to="/docs" @click="mobileMenuOpen = false">{{ copy.nav.docs }}</router-link>
        <router-link to="/home#faq" @click="mobileMenuOpen = false">{{ copy.nav.faq }}</router-link>
        <div class="mobile-tools">
          <LocaleSwitcher />
          <button type="button" class="mobile-theme" @click="toggleTheme">
            <Icon :name="isDark ? 'sun' : 'moon'" size="md" />
            {{ isDark ? copy.actions.light : copy.actions.dark }}
          </button>
        </div>
        <router-link v-if="isAuthenticated" :to="dashboardPath" class="mobile-primary">{{ copy.actions.console }}</router-link>
        <template v-else>
          <router-link to="/login" class="mobile-primary">{{ copy.actions.login }}</router-link>
          <router-link to="/register" class="mobile-secondary">{{ copy.actions.create }}</router-link>
        </template>
      </div>
    </header>

    <main class="pricing-main">
      <section class="page-heading">
        <h1>{{ copy.title }}</h1>
        <p>{{ copy.intro }}</p>
      </section>

      <section class="family-switcher" :aria-label="copy.familyLabel">
        <button
          v-for="family in families"
          :key="family.id"
          type="button"
          :class="{ active: activeFamilyId === family.id }"
          @click="activeFamilyId = family.id"
        >
          <span class="family-mark" :class="family.id">{{ family.mark }}</span>
          {{ family.name }}
        </button>
      </section>

      <section class="rules-card">
        <div class="rules-title">
          <Icon name="calculator" size="md" />
          <strong>{{ copy.rulesTitle }}</strong>
        </div>
        <p>
          {{ copy.rulesPrefix }} <b>{{ activeGroup.multiplier }}</b>{{ copy.rulesMiddle }} <b>{{ formatPercent(activeGroup.multiplier) }}</b>
        </p>
      </section>

      <section class="price-panel">
        <header class="panel-header">
          <div>
            <Icon name="badge" size="md" />
            <h2>{{ copy.listTitle }}</h2>
          </div>
          <p>{{ copy.listHint }}</p>
        </header>

        <div class="panel-body">
          <div class="group-list">
            <button
              v-for="group in localizedGroups"
              :key="group.id"
              type="button"
              class="group-card"
              :class="{ active: activeGroupId === group.id }"
              @click="activeGroupId = group.id"
            >
              <span v-if="activeGroupId === group.id" class="selected-check">✓</span>
              <strong>{{ group.name }}</strong>
              <span class="discount-badge">{{ group.badge }}</span>
              <small>{{ group.summary }}</small>
            </button>
          </div>

          <div class="group-intro">
            <strong>{{ copy.groupIntro }}</strong>
            <span class="divider">|</span>
            <span>{{ activeGroup.description }}</span>
          </div>

          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{{ copy.columns.model }}</th>
                  <th v-for="column in activeFamily.columns" :key="column">{{ copy.columns[column] }}</th>
                  <th>{{ copy.columns.savings }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="model in activeFamily.models" :key="model.id">
                  <td>
                    <div class="model-id">
                      <code>{{ model.id }}</code>
                      <button type="button" :title="`${copy.copy} ${model.id}`" @click="copyModelId(model.id)">
                        <Icon :name="copiedModel === model.id ? 'check' : 'copy'" size="sm" />
                      </button>
                    </div>
                  </td>
                  <td v-for="column in activeFamily.columns" :key="column">
                    <strong>{{ formatUsd(groupPrice(model[column])) }}</strong>
                    <small>{{ copy.tokenSuffix }}</small>
                  </td>
                  <td><span class="saving-badge">{{ formatPercent(activeGroup.multiplier) }}</span></td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>
    </main>

    <footer class="site-footer">
      <div class="footer-grid">
        <section class="footer-brand">
          <strong>Vote AI</strong>
          <p>{{ copy.footer.tagline }}</p>
        </section>
        <section>
          <h3>{{ copy.footer.services }}</h3>
          <router-link to="/pricing">{{ copy.nav.pricing }}</router-link>
          <router-link to="/docs">{{ copy.nav.docs }}</router-link>
        </section>
        <section>
          <h3>{{ copy.footer.models }}</h3>
          <span>Codex</span>
        </section>
        <section>
          <h3>{{ copy.footer.support }}</h3>
          <router-link to="/home#faq">{{ copy.nav.faq }}</router-link>
          <span>{{ copy.footer.contact }}</span>
        </section>
      </div>
      <div class="footer-bottom">
        <span>© {{ currentYear }} Vote AI. {{ copy.footer.rights }}</span>
        <span>{{ copy.footer.note }}</span>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'

type FamilyId = 'codex'
type GroupId = 'special' | 'plus' | 'pro'
type PriceColumn = 'input' | 'output' | 'cacheRead'
type PriceModel = { id: string } & Partial<Record<PriceColumn, number>>
interface PricingFamily {
  id: FamilyId
  name: string
  mark: string
  columns: PriceColumn[]
  models: PriceModel[]
}

const { locale } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const activeFamilyId = ref<FamilyId>('codex')
const activeGroupId = ref<GroupId>('special')
const copiedModel = ref('')
const mobileMenuOpen = ref(false)
const isDark = ref(document.documentElement.classList.contains('dark'))

const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() => authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
const currentYear = computed(() => new Date().getFullYear())

const families: PricingFamily[] = [
  {
    id: 'codex',
    name: 'Codex',
    mark: '◎',
    columns: ['input', 'output', 'cacheRead'],
    models: [
      { id: 'gpt-5.6-sol', input: 5, output: 30, cacheRead: 0.5 },
      { id: 'gpt-5.6-terra', input: 2.5, output: 15, cacheRead: 0.25 },
      { id: 'gpt-5.6-luna', input: 1, output: 6, cacheRead: 0.1 },
      { id: 'gpt-5.5', input: 5, output: 30, cacheRead: 0.5 },
      { id: 'gpt-5.4', input: 2.5, output: 15, cacheRead: 0.25 },
      { id: 'gpt-5.4-mini', input: 0.75, output: 4.5, cacheRead: 0.075 }
    ]
  }
]

const groups = {
  zh: [
    { id: 'special' as GroupId, name: '特惠', multiplier: 0.05, badge: '0.5 折', summary: '官方 API 价格 × 0.05', description: '特惠分组，按官方 API 价格的 5% 计算。' },
    { id: 'plus' as GroupId, name: 'Plus', multiplier: 0.12, badge: '1.2 折', summary: '官方 API 价格 × 0.12', description: 'Plus 分组，按官方 API 价格的 12% 计算。' },
    { id: 'pro' as GroupId, name: 'Pro', multiplier: 0.2, badge: '2 折', summary: '官方 API 价格 × 0.2', description: 'Pro 分组，按官方 API 价格的 20% 计算。' }
  ],
  en: [
    { id: 'special' as GroupId, name: 'Special', multiplier: 0.05, badge: '5%', summary: 'Official API price × 0.05', description: 'Special group pricing is calculated at 5% of the official API price.' },
    { id: 'plus' as GroupId, name: 'Plus', multiplier: 0.12, badge: '12%', summary: 'Official API price × 0.12', description: 'Plus group pricing is calculated at 12% of the official API price.' },
    { id: 'pro' as GroupId, name: 'Pro', multiplier: 0.2, badge: '20%', summary: 'Official API price × 0.2', description: 'Pro group pricing is calculated at 20% of the official API price.' }
  ]
}

const zhCopy = {
  nav: { home: '首页', pricing: '模型价格', docs: '接入文档', docsPending: '文档入口即将开放', faq: '常见问题' },
  actions: { light: '日间模式', dark: '夜间模式', login: '登录', start: '立即开始', create: '创建账号', console: '控制台', menu: '打开导航' },
  title: '模型价格',
  intro: '当前展示 Codex 模型静态分组价格。分组价格由官方 API 价格乘以对应倍率计算，最终可用分组与扣费记录以控制台为准。',
  familyLabel: '模型家族', rulesTitle: '计价规则',
  rulesPrefix: '分组价格 = 官方 API 价格 × ', rulesMiddle: '，即官方原价的 ',
  rulesOther: '1 元人民币充值 = 1 美元额度，扣费比例以密钥详情显示为准',
  listTitle: '价格列表', listHint: '查看当前模型可用的分组价格。', groupIntro: '分组介绍：',
  columns: { model: '模型 ID', input: '输入价格', output: '输出价格', cacheCreate: '缓存创建', cacheRead: '缓存读取', savings: '节省幅度' },
  copy: '复制', tokenSuffix: '/ 1M tokens', discountUnit: '折',
  footer: { tagline: '一个 API 连接全球顶级 AI 大模型', services: '服务', models: '支持模型', support: '支持', contact: '联系我们', rights: '保留所有权利。', note: '静态价格仅供展示，最终扣费以控制台记录为准。' }
}

const enCopy = {
  nav: { home: 'Home', pricing: 'Model pricing', docs: 'Docs', docsPending: 'Documentation is coming soon', faq: 'FAQ' },
  actions: { light: 'Light mode', dark: 'Dark mode', login: 'Sign in', start: 'Get started', create: 'Create account', console: 'Console', menu: 'Open navigation' },
  title: 'Model pricing',
  intro: 'This page currently shows static Codex group pricing. Each group price is calculated from the official API price and its multiplier; final availability and charges are subject to the console.',
  familyLabel: 'Model families', rulesTitle: 'Billing rules',
  rulesPrefix: 'Group price = official API price × ', rulesMiddle: ', equal to ',
  rulesOther: 'CNY 1 top-up equals USD 1 balance. Refer to key details for the final billing ratio.',
  listTitle: 'Price list', listHint: 'View available group pricing for the selected model family.', groupIntro: 'Group details:',
  columns: { model: 'Model ID', input: 'Input', output: 'Output', cacheCreate: 'Cache write', cacheRead: 'Cache read', savings: 'Savings' },
  copy: 'Copy', tokenSuffix: '/ 1M tokens', discountUnit: '折',
  footer: { tagline: 'One API for the world’s leading AI models', services: 'Services', models: 'Models', support: 'Support', contact: 'Contact us', rights: 'All rights reserved.', note: 'Static prices are for display; final charges are subject to console records.' }
}

const copy = computed(() => locale.value.startsWith('zh') ? zhCopy : enCopy)
const activeFamily = computed(() => families.find((family) => family.id === activeFamilyId.value) || families[0])
const localizedGroups = computed(() => groups[locale.value.startsWith('zh') ? 'zh' : 'en'])
const activeGroup = computed(() => localizedGroups.value.find((group) => group.id === activeGroupId.value) || localizedGroups.value[0])

function groupPrice(value: number | undefined) {
  return value === undefined ? undefined : value * activeGroup.value.multiplier
}

function formatUsd(value: number | undefined) {
  return value === undefined ? '—' : `$${Number(value.toFixed(4))}`
}

function formatPercent(multiplier: number) {
  return `${Number((multiplier * 100).toFixed(2))}%`
}

async function copyModelId(id: string) {
  try {
    await navigator.clipboard.writeText(id)
    copiedModel.value = id
    window.setTimeout(() => {
      if (copiedModel.value === id) copiedModel.value = ''
    }, 1800)
  } catch {
    appStore.showError(locale.value.startsWith('zh') ? '复制失败' : 'Copy failed')
  }
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(() => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings()
})
</script>

<style scoped>
.pricing-page {
  min-height: 100vh;
  color: #1c1c19;
  background: #fcf9f4;
}
.pricing-page.theme-dark { color-scheme: dark; color: #fcf9f4; background: #1c1c19; }
:global(html.dark body) { background: #1c1c19; }
a { text-decoration: none; }
.site-header {
  position: sticky;
  z-index: 50;
  top: 0;
  border-bottom: 1px solid rgba(88, 66, 56, 0.1);
  background: rgba(252, 249, 244, 0.94);
  backdrop-filter: blur(18px);
}
.theme-dark .site-header { border-color: #524e49; background: rgba(28, 28, 25, 0.94); }
.site-nav {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  width: min(100% - 48px, 1280px);
  min-height: 76px;
  margin: 0 auto;
  gap: 38px;
}
.brand { display: inline-flex; align-items: center; gap: 10px; color: #2b211c; font-size: 18px; font-weight: 700; white-space: nowrap; }
.theme-dark .brand { color: #fcf9f4; }
.brand-logo { width: 34px; height: 34px; overflow: hidden; border-radius: 10px; background: #fff; box-shadow: 0 5px 16px rgba(62, 22, 0, 0.1); }
.brand-logo img { width: 100%; height: 100%; object-fit: contain; }
.nav-links { display: flex; justify-content: center; align-items: center; gap: 32px; }
.nav-links a, .nav-disabled { color: #6f655e; font-size: 14px; font-weight: 550; white-space: nowrap; }
.nav-links a:hover, .nav-links .active { color: #9c3f00; }
.nav-links .active { font-weight: 750; }
.nav-disabled { cursor: not-allowed; opacity: 0.45; }
.theme-dark .nav-links a, .theme-dark .nav-disabled { color: #c4beb7; }
.theme-dark .nav-links a:hover, .theme-dark .nav-links .active { color: #ffb594; }
.nav-actions { display: flex; align-items: center; gap: 10px; }
.icon-button, .mobile-menu-button { display: grid; place-items: center; width: 38px; height: 38px; border: 0; border-radius: 10px; color: #584238; background: transparent; cursor: pointer; }
.icon-button:hover { background: #f6eee8; }
.theme-dark .icon-button { color: #ffdbcc; }
.theme-dark .icon-button:hover { background: #302f2a; }
.text-action { padding: 9px 10px; color: #584238; font-size: 14px; font-weight: 650; white-space: nowrap; }
.theme-dark .text-action { color: #ffdbcc; }
.primary-action { padding: 10px 20px; border-radius: 8px; color: #fff; background: #9c3f00; font-size: 14px; font-weight: 750; white-space: nowrap; transition: transform 0.2s; }
.primary-action:hover { transform: translateY(-2px); }
.mobile-menu-button { display: none; }
.mobile-menu { display: none; }
.pricing-main { width: min(100% - 48px, 1280px); margin: 0 auto; padding: 64px 0 72px; }
.page-heading { margin-bottom: 30px; }
.page-heading h1 { margin: 0; font-size: clamp(30px, 4vw, 42px); letter-spacing: -0.035em; }
.page-heading p { max-width: 1060px; margin: 12px 0 0; color: #6f655e; font-size: 16px; line-height: 1.85; }
.theme-dark .page-heading p { color: #c4beb7; }
.family-switcher { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 16px; padding: 8px; border: 1px solid #eadfd7; border-radius: 10px; background: #fff; box-shadow: 0 1px 3px rgba(62, 22, 0, 0.05); }
.theme-dark .family-switcher { border-color: #524e49; background: #252420; }
.family-switcher button { display: inline-flex; align-items: center; gap: 8px; padding: 10px 16px; border: 1px solid transparent; border-radius: 7px; color: #6f655e; background: transparent; font-size: 15px; font-weight: 650; cursor: pointer; }
.family-switcher button.active { border-color: #c45100; color: #9c3f00; background: #fef1e8; box-shadow: 0 1px 3px rgba(156, 63, 0, 0.08); }
.theme-dark .family-switcher button { color: #c4beb7; }
.theme-dark .family-switcher button.active { border-color: #ffb594; color: #ffb594; background: #5c2a10; }
.family-mark { display: grid; place-items: center; width: 22px; height: 22px; color: currentColor; font-size: 13px; font-weight: 800; }
.family-mark.gemini { font-size: 18px; }
.rules-card { display: flex; align-items: center; justify-content: space-between; gap: 24px; margin-bottom: 16px; padding: 14px 20px; border: 1px solid #eadfd7; border-radius: 9px; background: #fff; box-shadow: 0 1px 3px rgba(62, 22, 0, 0.04); }
.theme-dark .rules-card { border-color: #524e49; background: #252420; }
.rules-title { display: flex; align-items: center; gap: 10px; white-space: nowrap; }
.rules-card p { margin: 0; color: #584238; font-size: 14px; line-height: 1.7; }
.rules-card b { color: #338454; }
.theme-dark .rules-card p { color: #c4beb7; }
.price-panel { overflow: hidden; border: 1px solid #eadfd7; border-radius: 10px; background: #fff; box-shadow: 0 2px 6px rgba(62, 22, 0, 0.05); }
.theme-dark .price-panel { border-color: #524e49; background: #252420; }
.panel-header { display: flex; align-items: center; justify-content: space-between; gap: 24px; padding: 20px; border-bottom: 1px solid #eadfd7; }
.theme-dark .panel-header { border-color: #524e49; }
.panel-header > div { display: flex; align-items: center; gap: 10px; }
.panel-header h2 { margin: 0; font-size: 20px; }
.panel-header p { margin: 0; color: #8a7e77; font-size: 12px; }
.panel-body { padding: 20px; }
.group-list { display: flex; gap: 12px; overflow-x: auto; padding-bottom: 2px; }
.group-card { position: relative; display: flex; flex-direction: column; align-items: flex-start; min-width: 256px; padding: 16px; border: 1px solid #eadfd7; border-radius: 9px; color: inherit; background: #fff; text-align: left; }
.group-card.active { border-color: #c45100; background: #fef4ed; box-shadow: 0 2px 5px rgba(156, 63, 0, 0.08); }
.theme-dark .group-card { border-color: #524e49; background: #302f2a; }
.theme-dark .group-card.active { border-color: #ffb594; background: #5c2a10; }
.selected-check { position: absolute; top: 12px; right: 12px; display: grid; place-items: center; width: 20px; height: 20px; border-radius: 50%; color: #fff; background: #9c3f00; font-size: 12px; }
.group-card strong { padding-right: 28px; font-size: 15px; }
.discount-badge { margin-top: 18px; padding: 4px 12px; border-radius: 99px; color: #5c2a10; background: #ffdbcc; font-size: 14px; font-weight: 800; }
.theme-dark .discount-badge { color: #ffdbcc; background: #5c2a10; }
.group-card small { margin-top: 8px; color: #7d7068; font-size: 12px; }
.theme-dark .group-card small { color: #c4beb7; }
.group-intro { display: flex; align-items: center; gap: 10px; margin: 16px 0; padding: 12px 16px; border: 1px solid #f2cfbb; border-radius: 8px; color: #6f4b38; background: #fef4ed; font-size: 14px; }
.group-intro strong { color: #9c3f00; }
.theme-dark .group-intro { border-color: #704026; color: #ffdbcc; background: #3d2418; }
.theme-dark .group-intro strong { color: #ffb594; }
.table-wrap { overflow-x: auto; border: 1px solid #eadfd7; border-radius: 8px; }
.theme-dark .table-wrap { border-color: #524e49; }
table { width: 100%; min-width: 780px; border-collapse: collapse; }
th, td { padding: 14px 20px; border-bottom: 1px solid #eee5df; text-align: left; white-space: nowrap; }
th { color: #6f655e; background: #f7f2ed; font-size: 14px; font-weight: 550; }
tbody tr:last-child td { border-bottom: 0; }
tbody tr:hover td { background: #fdf9f6; }
.theme-dark th { color: #c4beb7; border-color: #524e49; background: #302f2a; }
.theme-dark td { border-color: #45423d; }
.theme-dark tbody tr:hover td { background: #2c2b26; }
.model-id { display: flex; align-items: center; gap: 8px; }
.model-id code { color: inherit; font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: 13px; }
.model-id button { display: grid; place-items: center; width: 28px; height: 28px; border: 0; border-radius: 6px; color: #8a7e77; background: transparent; cursor: pointer; }
.model-id button:hover { color: #9c3f00; background: #f6eee8; }
td > strong { display: block; color: #9c3f00; font-size: 16px; }
td > small { display: block; margin-top: 3px; color: #8a7e77; font-size: 11px; }
.theme-dark td > strong { color: #ffb594; }
.saving-badge { display: inline-flex; padding: 4px 10px; border-radius: 99px; color: #2c6c45; background: #e5f4ea; font-size: 12px; font-weight: 750; }
.theme-dark .saving-badge { color: #a8e6bd; background: #244632; }
.site-footer { border-top: 1px solid #eadfd7; background: #f5efe9; }
.theme-dark .site-footer { border-color: #524e49; background: #252420; }
.footer-grid { display: grid; grid-template-columns: 1.35fr 0.8fr 1fr 0.9fr; gap: 56px; width: min(100% - 48px, 1152px); margin: 0 auto; padding: 52px 0 44px; }
.footer-grid section { display: flex; flex-direction: column; align-items: flex-start; gap: 12px; }
.footer-brand strong { color: #9c3f00; font-size: 22px; }
.theme-dark .footer-brand strong { color: #ffb594; }
.footer-grid h3, .footer-grid p { margin: 0; }
.footer-grid h3 { font-size: 14px; }
.footer-grid p, .footer-grid a, .footer-grid span { color: #766a63; font-size: 14px; line-height: 1.7; }
.theme-dark .footer-grid p, .theme-dark .footer-grid a, .theme-dark .footer-grid span { color: #c4beb7; }
.footer-grid a:hover { color: #9c3f00; }
.footer-bottom { display: flex; justify-content: space-between; gap: 24px; width: min(100% - 48px, 1152px); margin: 0 auto; padding: 22px 0 30px; border-top: 1px solid #ddd1c8; color: #8a7e77; font-size: 12px; }
.theme-dark .footer-bottom { border-color: #524e49; color: #a9a29b; }

@media (max-width: 900px) {
  .site-nav { gap: 20px; }
  .nav-links { gap: 18px; }
  .primary-action { display: none; }
  .footer-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}

@media (max-width: 767px) {
  .site-nav { display: flex; justify-content: space-between; width: min(100% - 36px, 1280px); min-height: 68px; }
  .nav-links, .nav-actions > :not(.mobile-menu-button) { display: none; }
  .mobile-menu-button { display: grid; }
  .mobile-menu { display: flex; flex-direction: column; gap: 4px; padding: 18px 24px 24px; border-top: 1px solid #eadfd7; background: #fcf9f4; }
  .theme-dark .mobile-menu { border-color: #524e49; background: #1c1c19; }
  .mobile-menu > a, .mobile-menu > .nav-disabled { padding: 11px 4px; color: #584238; font-size: 17px; font-weight: 700; }
  .theme-dark .mobile-menu > a, .theme-dark .mobile-menu > .nav-disabled { color: #ffdbcc; }
  .mobile-menu .active { color: #9c3f00; }
  .mobile-tools { display: flex; align-items: center; justify-content: space-between; margin-top: 10px; padding-top: 14px; border-top: 1px solid #eadfd7; }
  .theme-dark .mobile-tools { border-color: #524e49; }
  .mobile-theme { display: inline-flex; align-items: center; gap: 8px; border: 0; color: inherit; background: transparent; }
  .mobile-primary, .mobile-secondary { margin-top: 8px; padding: 12px 16px !important; border-radius: 8px; text-align: center; }
  .mobile-primary { color: #fff !important; background: #9c3f00; }
  .mobile-secondary { border: 1px solid #c45100; color: #9c3f00 !important; }
  .pricing-main { width: min(100% - 36px, 1280px); padding-top: 46px; }
  .rules-card, .panel-header, .footer-bottom { align-items: flex-start; flex-direction: column; }
  .panel-header { gap: 8px; }
  .group-intro { align-items: flex-start; flex-direction: column; gap: 5px; }
  .group-intro .divider { display: none; }
  .footer-grid { grid-template-columns: 1fr; gap: 34px; width: min(100% - 36px, 1152px); }
  .footer-bottom { width: min(100% - 36px, 1152px); }
}
</style>
