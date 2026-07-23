<template>
  <div v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <div v-else v-html="homeContent"></div>
  </div>

  <div v-else class="home-page" :class="{ 'theme-dark': isDark }">
    <header class="site-header">
      <nav class="site-nav" aria-label="首页导航">
        <a href="#home" class="brand" aria-label="返回 Vote AI 首页">
          <span class="brand-logo">
            <img :src="siteLogo || '/logo.svg'" alt="" />
          </span>
          <span class="brand-name">Vote AI</span>
        </a>

        <div class="nav-links">
          <a href="#home">{{ copy.nav.home }}</a>
          <router-link to="/pricing">{{ copy.nav.pricing }}</router-link>
          <a href="#faq">{{ copy.nav.faq }}</a>
          <router-link to="/docs">{{ copy.nav.docs }}</router-link>
        </div>

        <div class="nav-actions">
          <div class="locale-control">
            <LocaleSwitcher />
          </div>
          <button
            type="button"
            class="icon-button"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <router-link v-if="isAuthenticated" :to="dashboardPath" class="console-button">
            <span class="avatar">{{ userInitial }}</span>
            <span>{{ t('home.dashboard') }}</span>
            <Icon name="arrowRight" size="sm" />
          </router-link>
          <router-link v-else to="/login" class="console-button">
            <span>{{ t('home.login') }}</span>
            <Icon name="arrowRight" size="sm" />
          </router-link>
        </div>
      </nav>
    </header>

    <main>
      <section id="home" class="hero-section">
        <div class="hero-grid">
          <div class="hero-copy">
            <p class="eyebrow">{{ copy.hero.eyebrow }}</p>
            <h1>Vote AI</h1>
            <p class="hero-lead">{{ siteSubtitle }}</p>
            <p class="hero-description">{{ copy.hero.description }}</p>
            <div class="hero-actions">
              <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="primary-cta">
                {{ isAuthenticated ? t('home.goToDashboard') : copy.hero.cta }}
                <Icon name="arrowRight" size="md" />
              </router-link>
              <router-link to="/pricing" class="secondary-cta">{{ copy.hero.pricingCta }}</router-link>
            </div>
            <div class="hero-trust">
              <span v-for="item in copy.hero.trust" :key="item">
                <i></i>{{ item }}
              </span>
            </div>
          </div>

          <div class="hero-globe">
            <InteractiveGlobe :is-dark="isDark" />
          </div>
        </div>
      </section>

      <section id="pricing" class="pricing-section section-anchor">
        <div class="section-heading">
          <p class="eyebrow">{{ copy.pricing.eyebrow }}</p>
          <h2>{{ copy.pricing.title }}</h2>
          <p>{{ copy.pricing.subtitle }}</p>
        </div>

        <div class="pricing-grid">
          <article
            v-for="plan in copy.pricing.plans"
            :key="plan.name"
            class="pricing-card"
            :class="{ featured: plan.featured, dark: plan.dark }"
          >
            <span v-if="plan.featured" class="recommended">{{ copy.pricing.recommended }}</span>
            <p class="plan-kicker">{{ plan.kicker }}</p>
            <h3>{{ plan.name }}</h3>
            <p class="plan-rate">{{ plan.rate }}</p>
            <p class="plan-description">{{ plan.description }}</p>
            <ul>
              <li v-for="feature in plan.features" :key="feature">
                <span>✓</span>{{ feature }}
              </li>
            </ul>
            <button
              type="button"
              class="plan-button"
              :disabled="plan.action === 'pending'"
              :aria-disabled="plan.action === 'pending'"
              @click="handlePlanAction(plan.action)"
            >
              {{ plan.cta }}
            </button>
          </article>
        </div>

        <p class="pricing-note">{{ copy.pricing.note }}</p>
      </section>

      <section class="value-section">
        <div class="value-inner">
          <div class="value-heading">
            <p class="eyebrow">{{ copy.value.eyebrow }}</p>
            <h2>{{ copy.value.title }}</h2>
          </div>

          <div class="value-grid">
            <article v-for="item in copy.value.items" :key="item.title" class="value-card">
              <span class="value-icon" aria-hidden="true">
                <svg v-if="item.icon === 'globe'" viewBox="0 0 24 24">
                  <circle cx="12" cy="12" r="8.5" />
                  <path d="M3.8 12h16.4M12 3.5c2.2 2.3 3.4 5.1 3.4 8.5S14.2 18.2 12 20.5M12 3.5C9.8 5.8 8.6 8.6 8.6 12s1.2 6.2 3.4 8.5" />
                </svg>
                <svg v-else-if="item.icon === 'refresh'" viewBox="0 0 24 24">
                  <path d="M19.2 8A7.8 7.8 0 0 0 6 5.4L4 8m0 0h5M4 8V3M4.8 16A7.8 7.8 0 0 0 18 18.6l2-2.6m0 0h-5m5 0v5" />
                </svg>
                <svg v-else-if="item.icon === 'terminal'" viewBox="0 0 24 24">
                  <rect x="3.5" y="4.5" width="17" height="15" rx="2" />
                  <path d="m7 9 3 3-3 3m5-1h4" />
                </svg>
                <svg v-else viewBox="0 0 24 24">
                  <path d="M12 3.5 19 6v5.2c0 4.2-2.7 7.7-7 9.3-4.3-1.6-7-5.1-7-9.3V6l7-2.5Z" />
                  <path d="m8.7 12 2.1 2.1 4.7-4.7" />
                </svg>
              </span>
              <div>
                <h3>{{ item.title }}</h3>
                <p>{{ item.description }}</p>
              </div>
            </article>
          </div>
        </div>
      </section>

      <section id="faq" class="faq-section section-anchor">
        <div class="faq-layout">
          <div class="faq-intro">
            <p class="eyebrow">FAQ</p>
            <h2>{{ copy.faq.title }}</h2>
            <p>{{ copy.faq.subtitle }}</p>
            <a href="#home" class="text-link">{{ copy.faq.back }} ↑</a>
          </div>

          <div class="faq-list">
            <details v-for="(item, index) in copy.faq.items" :key="item.question" :open="index === 0">
              <summary>
                <span>{{ item.question }}</span>
                <i></i>
              </summary>
              <p>{{ item.answer }}</p>
            </details>
          </div>
        </div>
      </section>
    </main>

    <footer class="site-footer">
      <div>
        <span>Vote AI</span>
        <p>© {{ currentYear }}. {{ t('home.footer.allRightsReserved') }}</p>
      </div>
      <div class="footer-links">
        <a href="#home">{{ copy.nav.home }}</a>
        <router-link to="/pricing">{{ copy.nav.pricing }}</router-link>
        <a href="#faq">{{ copy.nav.faq }}</a>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import InteractiveGlobe from '@/components/home/InteractiveGlobe.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'

const { t, locale } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()

const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isDark = ref(document.documentElement.classList.contains('dark'))
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const userInitial = computed(() => authStore.user?.email?.charAt(0).toUpperCase() || '')
const currentYear = computed(() => new Date().getFullYear())

const zhCopy = {
  nav: { home: '首页', pricing: '价格', faq: '常见问题', docs: '文档', docsPending: '文档入口即将开放' },
  hero: {
    eyebrow: '全球 AI API 网关',
    description: '一站式连接 Claude、ChatGPT、Gemini 等主流模型。稳定、快速、按量计费，让团队专注于创造，而不是复杂的接入配置。',
    cta: '立即开始',
    pricingCta: '查看价格',
    trust: ['全球节点加速', '余额永不过期', '统一 API 接入']
  },
  pricing: {
    eyebrow: '定价方案',
    title: '按量付费，按需使用',
    subtitle: '透明、灵活的计费方式，按照实际用量结算，无需承担长期订阅成本。',
    recommended: '推荐',
    note: '价格页面与完整模型费率表将在下一阶段接入。',
    plans: [
      {
        kicker: 'PAYGO', name: '按量付费', rate: '灵活充值', description: '一次充值，按实际使用量扣费。', dark: true,
        features: ['充值后获得平台余额', '按实际模型用量计费', '余额长期有效'], cta: '立即开始', action: 'login'
      },
      {
        kicker: 'CLAUDE', name: 'Claude 按量付费', rate: '官方费率同步', description: '无需订阅，根据实际使用量灵活计费。', featured: true,
        features: ['支持 Claude 全系列模型', '专为 Claude Code 优化', '稳定的全球节点接入'], cta: '火速接入中', action: 'pending'
      },
      {
        kicker: 'OPENAI', name: 'ChatGPT 按量付费', rate: '官方费率同步', description: '统一余额，按模型调用量结算。', featured: true,
        features: ['支持 OpenAI GPT 全系列模型', '专为 Codex 工作流优化', '兼容官方 API 调用方式'], cta: '查看价格', action: 'pricing'
      }
    ]
  },
  value: {
    eyebrow: '使用价值',
    title: '释放你的编程潜能，让顶尖 AI 为你写代码',
    items: [
      { icon: 'globe', title: '国内直连', description: '无论身处何地，都能尽量稳定访问，减少等待与网络波动带来的中断。' },
      { icon: 'refresh', title: '高可用架构', description: '分布式架构设计与自动故障转移，保障关键时刻依然可用。' },
      { icon: 'terminal', title: '简单集成', description: '只需修改 API 地址即可使用，无需重写现有业务逻辑。' },
      { icon: 'shield', title: '完整兼容', description: '统一接入主流模型能力，尽可能保留官方 API 的调用行为。' }
    ]
  },
  faq: {
    title: '常见问题',
    subtitle: '关于接入、计费和使用方式的快速解答。',
    back: '返回顶部',
    items: [
      { question: '如何开始使用 Vote AI？', answer: '登录控制台后创建 API Key，根据控制台提供的地址替换原有 API Endpoint，即可使用兼容的客户端和开发工具。' },
      { question: '余额和用量如何计算？', answer: '平台按实际模型调用量计费。详细倍率和模型费率将在价格页面展示，控制台中可以实时查看余额与用量记录。' },
      { question: '支持哪些模型和开发工具？', answer: '当前面向 Claude、OpenAI、Gemini 等主流模型提供统一接入，并兼容 Claude Code、Codex 等常用 AI 开发工具。' },
      { question: '是否需要长期订阅？', answer: '不需要。默认采用按量付费方式，按需充值、按实际使用量结算。' },
      { question: 'API 接入方式是否复杂？', answer: '不复杂。大多数情况下只需修改 API 地址与密钥，不需要重写现有业务代码。' }
    ]
  }
}

const enCopy = {
  nav: { home: 'Home', pricing: 'Pricing', faq: 'FAQ', docs: 'Docs', docsPending: 'Documentation is coming soon' },
  hero: {
    eyebrow: 'Global AI API gateway',
    description: 'Connect Claude, ChatGPT, Gemini, and more through one reliable gateway. Fast global access and usage-based billing keep your team focused on building.',
    cta: 'Get started', pricingCta: 'View pricing',
    trust: ['Global connectivity', 'Balance never expires', 'Unified API access']
  },
  pricing: {
    eyebrow: 'Pricing', title: 'Pay as you go', subtitle: 'Transparent usage-based billing without long-term subscription costs.', recommended: 'Recommended',
    note: 'The dedicated pricing page and full model rate table will be added next.',
    plans: [
      { kicker: 'PAYGO', name: 'Pay as you go', rate: 'Flexible top-up', description: 'Top up once and pay for actual usage.', dark: true, features: ['Platform balance after top-up', 'Usage-based model billing', 'Balance remains valid'], cta: 'Get started', action: 'login' },
      { kicker: 'CLAUDE', name: 'Claude on demand', rate: 'Official rates synced', description: 'No subscription. Pay for what you use.', featured: true, features: ['All Claude model families', 'Optimized for Claude Code', 'Stable global connectivity'], cta: 'Coming soon', action: 'pending' },
      { kicker: 'OPENAI', name: 'ChatGPT on demand', rate: 'Official rates synced', description: 'One balance for usage-based billing.', featured: true, features: ['All OpenAI GPT families', 'Optimized for Codex', 'Official API compatibility'], cta: 'View pricing', action: 'pricing' }
    ]
  },
  value: {
    eyebrow: 'Why us',
    title: 'Unleash your coding potential and let top-tier AI write the code',
    items: [
      { icon: 'globe', title: 'Direct connectivity', description: 'Stable access wherever you work, reducing interruptions caused by latency and network issues.' },
      { icon: 'refresh', title: 'High availability', description: 'Distributed architecture and automatic failover keep the gateway available when it matters.' },
      { icon: 'terminal', title: 'Easy integration', description: 'Change the API endpoint and keep your existing application logic and development workflow.' },
      { icon: 'shield', title: 'Full compatibility', description: 'Use mainstream model capabilities through a unified gateway with familiar official API behavior.' }
    ]
  },
  faq: {
    title: 'Frequently asked questions', subtitle: 'Quick answers about access, billing, and usage.', back: 'Back to top',
    items: [
      { question: 'How do I start using Vote AI?', answer: 'Sign in, create an API key, and replace your existing API endpoint with the address shown in the console.' },
      { question: 'How are balance and usage calculated?', answer: 'Billing follows actual model usage. Detailed rates will be available on the upcoming pricing page.' },
      { question: 'Which models and tools are supported?', answer: 'The gateway targets Claude, OpenAI, Gemini, and common AI development tools including Claude Code and Codex.' },
      { question: 'Do I need a subscription?', answer: 'No. The default model is pay as you go, so you only pay for actual usage.' },
      { question: 'Is API integration difficult?', answer: 'Usually not. In most cases, changing the endpoint and API key is all that is required.' }
    ]
  }
}

const copy = computed(() => locale.value.startsWith('zh') ? zhCopy : enCopy)

function handlePlanAction(action: string) {
  if (action === 'login') {
    router.push('/login')
    return
  }

  if (action === 'pricing') {
    router.push('/pricing')
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
.home-page {
  min-height: 100vh;
  overflow-x: hidden;
  background: #fcf9f4;
  color: #1c1c19;
}

.home-page.theme-dark {
  color-scheme: dark;
  background: #15100d;
  color: #f6f3ee;
}

:global(html.dark body) {
  background: #15100d;
}

.site-header {
  position: sticky;
  z-index: 50;
  top: 0;
  border-bottom: 1px solid rgba(88, 66, 56, 0.08);
  background: rgba(252, 249, 244, 0.9);
  backdrop-filter: blur(18px);
}

.home-page.theme-dark .site-header {
  border-color: rgba(234, 223, 216, 0.08);
  background: rgba(21, 16, 13, 0.9);
}

.site-nav,
.site-footer {
  width: min(100% - 40px, 1180px);
  margin-inline: auto;
}

.site-nav {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  min-height: 72px;
}

.brand {
  display: inline-flex;
  align-items: center;
  justify-self: start;
  gap: 10px;
}
.brand-logo {
  display: block;
  width: 36px;
  height: 36px;
  overflow: hidden;
  border-radius: 10px;
  box-shadow: 0 8px 22px rgba(62, 22, 0, 0.12);
}
.brand-logo img { width: 100%; height: 100%; object-fit: contain; }
.brand-name {
  color: #2b211c;
  font-size: 17px;
  font-weight: 650;
  letter-spacing: -0.02em;
  white-space: nowrap;
}
.home-page.theme-dark .brand-name { color: #f6f3ee; }

.nav-links { display: flex; align-items: center; gap: 30px; }
.nav-links a,
.nav-link-disabled,
.footer-links a {
  color: #584238;
  font-size: 14px;
  transition: color 0.2s;
}
.nav-link-disabled {
  cursor: not-allowed;
  opacity: 0.48;
  white-space: nowrap;
}
.nav-links a:hover,
.footer-links a:hover { color: #9c3f00; }
.home-page.theme-dark .nav-links a,
.home-page.theme-dark .nav-link-disabled,
.home-page.theme-dark .footer-links a { color: #d8c7bd; }

.nav-actions { display: flex; align-items: center; justify-self: end; gap: 8px; }
.icon-button {
  display: grid;
  width: 34px;
  height: 34px;
  place-items: center;
  border-radius: 50%;
  color: #755f54;
  transition: background 0.2s;
}
.icon-button:hover { background: #f0e7e0; }
.console-button {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-height: 34px;
  padding: 4px 12px 4px 5px;
  border-radius: 999px;
  background: #1c1c19;
  color: white;
  font-size: 12px;
}
.avatar {
  display: grid;
  width: 24px;
  height: 24px;
  place-items: center;
  border-radius: 50%;
  background: #c45100;
}

.hero-section {
  position: relative;
  min-height: calc(100vh - 72px);
  padding: 64px 20px 72px;
  background-image: linear-gradient(rgba(156, 63, 0, 0.035) 1px, transparent 1px), linear-gradient(90deg, rgba(156, 63, 0, 0.035) 1px, transparent 1px);
  background-size: 64px 64px;
}
.home-page.theme-dark .hero-section {
  background-color: #15100d;
  background-image: linear-gradient(rgba(223, 123, 66, 0.045) 1px, transparent 1px), linear-gradient(90deg, rgba(223, 123, 66, 0.045) 1px, transparent 1px);
}

.hero-section::after {
  position: absolute;
  top: -180px;
  right: -110px;
  width: 440px;
  height: 440px;
  border-radius: 50%;
  background: rgba(223, 123, 66, 0.11);
  filter: blur(80px);
  content: '';
}
.hero-grid {
  position: relative;
  z-index: 1;
  display: grid;
  grid-template-columns: minmax(0, 0.9fr) minmax(460px, 1.1fr);
  align-items: center;
  width: min(100%, 1180px);
  min-height: 650px;
  margin-inline: auto;
  gap: 56px;
}
.hero-copy { max-width: 560px; }
.eyebrow {
  margin-bottom: 14px;
  color: #9c3f00;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}
.hero-copy h1 {
  margin: 0;
  font-size: clamp(56px, 7vw, 92px);
  font-weight: 600;
  letter-spacing: -0.055em;
  line-height: 0.96;
}
.hero-lead { margin-top: 24px; color: #584238; font-size: clamp(19px, 2vw, 25px); }
.hero-description { max-width: 550px; margin-top: 18px; color: #755f54; font-size: 16px; line-height: 1.8; }
.home-page.theme-dark .hero-lead,
.home-page.theme-dark .hero-description { color: #d8c7bd; }
.hero-actions { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 32px; }
.primary-cta,
.secondary-cta,
.plan-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  min-height: 46px;
  padding: 0 24px;
  border-radius: 9px;
  font-size: 14px;
  font-weight: 600;
  transition: transform 0.2s, box-shadow 0.2s, background 0.2s;
}
.primary-cta { background: #c45100; box-shadow: 0 10px 24px rgba(196, 81, 0, 0.22); color: white; }
.primary-cta:hover { transform: translateY(-2px); box-shadow: 0 14px 30px rgba(196, 81, 0, 0.28); }
.secondary-cta { border: 1px solid #e0c0b2; color: #78350f; }
.hero-trust { display: flex; flex-wrap: wrap; gap: 18px; margin-top: 28px; color: #755f54; font-size: 12px; }
.hero-trust span { display: inline-flex; align-items: center; gap: 7px; }
.hero-trust i { width: 5px; height: 5px; border-radius: 50%; background: #c45100; }
.hero-globe { display: flex; justify-content: center; min-width: 0; }

.section-anchor { scroll-margin-top: 72px; }
.pricing-section { padding: 112px 20px 96px; background: #f6f3ee; }
.home-page.theme-dark .pricing-section { background: #211814; }
.section-heading { max-width: 760px; margin: 0 auto 58px; text-align: center; }
.section-heading h2,
.faq-intro h2 { margin: 0; font-size: clamp(40px, 5vw, 62px); font-weight: 500; letter-spacing: -0.045em; line-height: 1.12; }
.section-heading > p:last-child,
.faq-intro > p { margin-top: 18px; color: #755f54; font-size: 16px; line-height: 1.7; }
.home-page.theme-dark .section-heading > p:last-child,
.home-page.theme-dark .faq-intro > p { color: #c9b5aa; }
.pricing-grid { display: grid; grid-template-columns: repeat(3, 1fr); width: min(100%, 1040px); margin-inline: auto; gap: 18px; }
.pricing-card {
  position: relative;
  display: flex;
  min-height: 450px;
  flex-direction: column;
  padding: 34px 30px 28px;
  border: 1px solid #e0c0b2;
  border-radius: 14px;
  background: #fff;
}
.home-page.theme-dark .pricing-card { border-color: #584238; background: #30231d; }
.pricing-card.dark { border-color: #1c1c19; }
.pricing-card.featured { border-width: 1.5px; border-color: #c07151; }
.recommended {
  position: absolute;
  top: -12px;
  left: 50%;
  padding: 5px 14px;
  transform: translateX(-50%);
  border-radius: 999px;
  background: #b96f50;
  color: white;
  font-size: 11px;
}
.plan-kicker { color: #9c3f00; font-size: 13px; font-weight: 600; letter-spacing: 0.08em; }
.pricing-card h3 { margin-top: 12px; font-size: 27px; font-weight: 500; }
.plan-rate { margin-top: 12px; font-size: 19px; font-weight: 600; }
.plan-description { min-height: 46px; margin-top: 7px; color: #755f54; font-size: 13px; line-height: 1.6; }
.pricing-card ul { display: grid; margin: 25px 0 28px; gap: 14px; color: #584238; font-size: 13px; }
.pricing-card li { display: flex; gap: 10px; line-height: 1.5; }
.pricing-card li span { color: #c45100; }
.home-page.theme-dark .plan-description,
.home-page.theme-dark .pricing-card ul { color: #d8c7bd; }
.plan-button { width: 100%; margin-top: auto; border: 0; background: #bd7253; color: white; }
.pricing-card.dark .plan-button { background: #1c1c19; }
.plan-button:hover:not(:disabled) { transform: translateY(-2px); box-shadow: 0 10px 24px rgba(62, 22, 0, 0.14); }
.plan-button:disabled {
  cursor: not-allowed;
  opacity: 0.58;
  box-shadow: none;
}
.pricing-note { margin-top: 24px; color: #8f776a; font-size: 12px; text-align: center; }

.value-section {
  padding: 108px 20px 112px;
  background: #fcf9f4;
}
.home-page.theme-dark .value-section { background: #15100d; }
.value-inner { width: min(100%, 1040px); margin-inline: auto; }
.value-heading { max-width: 790px; margin-bottom: 48px; }
.value-heading h2 {
  margin: 0;
  font-size: clamp(38px, 4.8vw, 58px);
  font-weight: 500;
  letter-spacing: -0.045em;
  line-height: 1.15;
}
.value-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 22px; }
.value-card {
  display: grid;
  grid-template-columns: 42px minmax(0, 1fr);
  min-height: 134px;
  align-items: start;
  gap: 18px;
  padding: 30px;
  border: 1px solid rgba(224, 192, 178, 0.28);
  border-radius: 14px;
  background: #fff;
}
.home-page.theme-dark .value-card { border-color: rgba(216, 199, 189, 0.12); background: #30231d; }
.value-icon {
  display: grid;
  width: 38px;
  height: 38px;
  place-items: center;
  border-radius: 9px;
  background: #fef8f4;
  color: #c45100;
}
.home-page.theme-dark .value-icon { background: rgba(196, 81, 0, 0.16); color: #f4b38f; }
.value-icon svg {
  width: 18px;
  height: 18px;
  fill: none;
  stroke: currentColor;
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-width: 1.6;
}
.value-card h3 { margin: 1px 0 8px; font-size: 19px; font-weight: 600; }
.value-card p { color: #755f54; font-size: 13px; line-height: 1.75; }
.home-page.theme-dark .value-card p { color: #c9b5aa; }

.faq-section { padding: 112px 20px 120px; background: #f6f3ee; }
.home-page.theme-dark .faq-section { background: #15100d; }
.faq-layout { display: grid; grid-template-columns: 0.75fr 1.25fr; width: min(100%, 1040px); margin-inline: auto; gap: 90px; }
.faq-intro { position: sticky; top: 120px; align-self: start; }
.faq-intro h2 { font-size: clamp(38px, 4vw, 54px); }
.text-link { display: inline-block; margin-top: 24px; color: #9c3f00; font-size: 13px; }
.faq-list { border-top: 1px solid #e0c0b2; }
.faq-list details { border-bottom: 1px solid #e0c0b2; }
.home-page.theme-dark .faq-list,
.home-page.theme-dark .faq-list details { border-color: #584238; }
.faq-list summary { display: flex; align-items: center; justify-content: space-between; gap: 20px; padding: 25px 2px; cursor: pointer; font-size: 17px; list-style: none; }
.faq-list summary::-webkit-details-marker { display: none; }
.faq-list summary i { position: relative; width: 16px; height: 16px; flex: 0 0 auto; }
.faq-list summary i::before,
.faq-list summary i::after { position: absolute; top: 7px; left: 1px; width: 14px; height: 1px; background: #9c3f00; content: ''; transition: transform 0.2s; }
.faq-list summary i::after { transform: rotate(90deg); }
.faq-list details[open] summary i::after { transform: rotate(0); }
.faq-list details > p { max-width: 650px; padding: 0 38px 24px 2px; color: #755f54; font-size: 14px; line-height: 1.8; }
.home-page.theme-dark .faq-list details > p { color: #c9b5aa; }

.site-footer { display: flex; align-items: center; justify-content: space-between; padding-block: 34px; border-top: 1px solid rgba(88, 66, 56, 0.12); }
.site-footer > div:first-child > span { font-weight: 600; }
.site-footer p { margin-top: 4px; color: #8f776a; font-size: 12px; }
.footer-links { display: flex; gap: 24px; }

@media (max-width: 900px) {
  .site-nav {
    grid-template-columns: auto minmax(0, 1fr) auto;
    min-height: 64px;
    gap: 16px;
  }
  .nav-links {
    justify-content: center;
    gap: clamp(16px, 4vw, 28px);
  }
  .nav-actions { gap: 5px; }
  .hero-section { min-height: auto; padding-top: 54px; }
  .hero-grid {
    grid-template-columns: minmax(0, 0.92fr) minmax(320px, 1.08fr);
    min-height: 520px;
    gap: 24px;
  }
  .hero-copy h1 { font-size: clamp(48px, 7vw, 68px); }
  .hero-lead { font-size: 19px; }
  .hero-description { font-size: 14px; }
  .hero-globe :deep(.globe-stage) { width: min(100%, 430px); }
  .pricing-grid { grid-template-columns: 1fr; max-width: 560px; gap: 28px; }
  .pricing-card { min-height: auto; }
  .value-heading { margin-inline: auto; text-align: center; }
  .faq-layout { grid-template-columns: 1fr; gap: 48px; }
  .faq-intro { position: static; text-align: center; }
}

@media (max-width: 680px) {
  .site-nav,
  .site-footer { width: min(100% - 24px, 1180px); }
  .site-nav {
    grid-template-columns: auto minmax(0, 1fr) auto;
    min-height: 58px;
    gap: 10px;
  }
  .brand {
    display: inline-flex;
    gap: 7px;
  }
  .brand-logo {
    width: 30px;
    height: 30px;
    border-radius: 8px;
  }
  .brand-name { font-size: 14px; }
  .nav-links {
    display: flex;
    justify-content: center;
    gap: clamp(10px, 3vw, 18px);
    min-width: 0;
  }
  .nav-links a,
  .nav-link-disabled {
    flex: 0 0 auto;
    font-size: 12px;
    white-space: nowrap;
  }
  .nav-actions {
    min-width: 0;
    justify-self: end;
    gap: 4px;
  }
  .locale-control {
    position: relative;
    width: auto;
    flex: 0 0 auto;
  }
  .locale-control :deep(> .relative) {
    position: relative;
  }
  .locale-control :deep(> .relative > button) {
    width: auto;
    min-width: 42px;
    height: 30px;
    justify-content: center;
    gap: 2px;
    padding: 0 4px;
    overflow: visible;
  }
  .locale-control :deep(> .relative > button span:first-child) {
    display: none;
  }
  .locale-control :deep(> .relative > button span:nth-child(2)) {
    display: inline;
    font-size: 11px;
    line-height: 1;
  }
  .locale-control :deep(> .relative > button svg) {
    width: 10px;
    height: 10px;
    margin-left: 0;
  }
  .locale-control :deep(> .relative > div) {
    right: 0;
    left: auto;
    width: 128px;
  }
  .icon-button {
    width: 30px;
    height: 30px;
  }
  .console-button {
    min-width: 62px;
    min-height: 32px;
    justify-content: center;
    gap: 5px;
    padding: 4px 9px;
    white-space: nowrap;
  }
  .avatar {
    width: 24px;
    height: 24px;
  }
  .console-button > span:not(.avatar) {
    display: inline;
    white-space: nowrap;
  }
  .console-button > svg { width: 14px; height: 14px; flex: 0 0 auto; }
  .hero-section { min-height: auto; padding: 36px 12px 64px; }
  .hero-grid {
    grid-template-columns: 1fr;
    min-height: auto;
    gap: 24px;
    text-align: center;
  }
  .hero-copy { margin-inline: auto; }
  .hero-copy h1 { font-size: 52px; }
  .hero-description { font-size: 14px; }
  .hero-actions,
  .hero-trust { justify-content: center; }
  .hero-actions {
    flex-wrap: nowrap;
    width: 100%;
    max-width: 430px;
    margin-inline: auto;
  }
  .hero-actions .primary-cta,
  .hero-actions .secondary-cta {
    width: auto;
    min-width: max-content;
    flex: 0 0 auto;
    padding-inline: 24px;
    white-space: nowrap;
    writing-mode: horizontal-tb;
  }
  .hero-actions .primary-cta { min-width: 172px; }
  .hero-actions .secondary-cta { min-width: 132px; }
  .hero-actions .primary-cta > svg { flex: 0 0 auto; }
  .hero-globe { order: 2; }
  .hero-globe :deep(.globe-stage) { width: min(88vw, 390px); }
  .hero-trust { gap: 10px; }
  .pricing-section,
  .value-section,
  .faq-section { padding-inline: 14px; }
  .pricing-card { padding: 30px 22px 24px; }
  .value-section { padding-block: 78px 82px; }
  .value-heading { margin-bottom: 34px; }
  .value-grid { grid-template-columns: 1fr; gap: 14px; }
  .value-card { min-height: auto; padding: 24px 20px; }
  .site-footer { flex-direction: column; align-items: flex-start; gap: 18px; }
}

@media (max-width: 520px) {
  .site-nav { width: calc(100% - 16px); gap: 6px; }
  .brand-name { display: none; }
  .nav-links { gap: 10px; }
  .nav-links a,
  .nav-link-disabled { font-size: 11px; }
  .nav-actions { gap: 2px; }
  .locale-control :deep(> .relative > button) { min-width: 38px; padding-inline: 2px; }
}

@media (max-width: 420px) {
  .nav-links a[href="#home"],
  .nav-link-disabled { display: none; }
  .hero-actions {
    max-width: none;
    gap: 10px;
  }
  .hero-actions .primary-cta,
  .hero-actions .secondary-cta {
    min-height: 46px;
    padding-inline: 16px;
    font-size: 13px;
  }
  .hero-actions .primary-cta { min-width: 156px; }
  .hero-actions .secondary-cta { min-width: 118px; }
}
</style>
