<template>
  <section class="space-y-5 rounded-2xl border border-primary-200 bg-white p-5 dark:border-primary-800 dark:bg-dark-800 sm:p-6" data-testid="first-use-journey" aria-labelledby="first-use-title">
    <header class="space-y-2">
      <h2 id="first-use-title" class="text-xl font-semibold tracking-tight text-gray-900 dark:text-white">{{ t('firstUseJourney.title') }}</h2>
      <p class="text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t(phase === 'simple' ? 'firstUseJourney.simpleReadyBody' : 'firstUseJourney.description') }}</p>
    </header>
    <ol class="grid grid-cols-2 gap-3" :class="phase === 'simple' ? 'lg:grid-cols-3' : 'lg:grid-cols-4'" :aria-label="t('firstUseJourney.progressLabel')">
      <li v-for="(step, index) in steps" :key="step.id" :data-testid="`journey-step-${step.id}`" :aria-current="currentStep === step.id ? 'step' : undefined"
        class="flex items-start gap-3 rounded-xl border p-3" :class="step.done ? 'border-primary-100 bg-primary-50/50 dark:border-primary-900 dark:bg-primary-950/20' : currentStep === step.id ? 'border-primary-400 bg-primary-50 dark:border-primary-500 dark:bg-primary-950/30' : 'border-gray-200 dark:border-dark-700'">
        <span class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold" :class="step.done || currentStep === step.id ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-300'">
          <Icon v-if="step.done" name="check" size="sm" /><span v-else>{{ index + 1 }}</span>
        </span>
        <div class="min-w-0">
          <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t(`firstUseJourney.${step.id}Step`) }}</p>
          <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t(`firstUseJourney.${step.id}Hint`) }}</p>
          <p class="mt-1 text-xs font-medium text-primary-700 dark:text-primary-300">{{ t(step.done ? 'firstUseJourney.done' : currentStep === step.id ? 'firstUseJourney.current' : 'firstUseJourney.later') }}</p>
        </div>
      </li>
    </ol>
    <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900/50 sm:p-5" data-testid="journey-funding">
      <p v-if="loading && phase !== 'simple'" class="text-sm text-gray-600 dark:text-gray-300" role="status">{{ t('firstUseJourney.verifying') }}</p>
      <template v-else>
        <div class="flex items-start gap-3">
          <Icon :name="fundingReady ? 'checkCircle' : phase === 'unknown' ? 'infoCircle' : 'creditCard'" size="lg" class="mt-0.5 shrink-0 text-primary-600 dark:text-primary-400" />
          <div class="min-w-0 space-y-1">
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ fundingTitle }}</h3>
            <p class="text-sm leading-6 text-gray-600 dark:text-gray-300">{{ fundingBody }}</p>
          </div>
        </div>
        <div v-if="fundingReady" class="mt-4 flex flex-wrap items-center gap-3">
          <RouterLink v-if="context === 'dashboard'" to="/get-started" class="btn btn-primary min-h-11" data-testid="journey-continue">{{ t(keyReady ? 'firstUseJourney.connectNext' : 'firstUseJourney.createKeyNext') }}<Icon name="arrowRight" size="sm" /></RouterLink>
          <button v-else type="button" :disabled="busy" class="btn btn-primary min-h-11" data-testid="journey-continue" @click="emit('continue')">{{ t(keyReady ? 'firstUseJourney.connectNext' : 'firstUseJourney.createKeyNext') }}<Icon name="arrowRight" size="sm" /></button>
          <button v-if="phase !== 'simple'" type="button" :disabled="busy" class="text-sm font-medium text-primary-700 hover:underline disabled:opacity-50 dark:text-primary-300" @click="emit('refresh')">{{ t('firstUseJourney.refresh') }}</button>
        </div>
        <div v-else-if="phase === 'empty'" class="mt-4 space-y-3">
          <div class="grid gap-3 sm:grid-cols-2">
            <RouterLink v-if="onlineAvailable === true" :to="purchaseTo" class="rounded-xl border border-primary-500 bg-primary-600 p-4 text-white transition hover:bg-primary-700" data-testid="journey-purchase" :aria-disabled="busy || undefined" @click="preventIfBusy">
              <span class="flex items-center justify-between gap-3 font-semibold">{{ t(subscriptionOnly ? 'firstUseJourney.buySubscription' : 'firstUseJourney.online') }}<Icon name="arrowRight" size="sm" /></span>
              <span class="mt-2 block text-sm leading-6 text-primary-100">{{ t(subscriptionOnly ? 'firstUseJourney.subscriptionPurchaseBody' : 'firstUseJourney.onlineHint') }}</span>
            </RouterLink>
            <RouterLink v-if="canRedeem" :to="redeemTo" class="rounded-xl border border-gray-200 bg-white p-4 text-gray-900 transition hover:border-primary-400 dark:border-dark-600 dark:bg-dark-800 dark:text-white" data-testid="journey-redeem" :aria-disabled="busy || undefined" @click="preventIfBusy">
              <span class="flex items-center justify-between gap-3 font-semibold">{{ t('firstUseJourney.redeem') }}<Icon name="gift" size="sm" /></span>
              <span class="mt-2 block text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('firstUseJourney.redeemHint') }}</span>
            </RouterLink>
          </div>
          <p v-if="onlineAvailable !== true" class="text-sm leading-6 text-gray-600 dark:text-gray-300" role="status">{{ t(onlineAvailable === false ? 'firstUseJourney.offlineHint' : 'firstUseJourney.paymentUnknown') }}<span v-if="contactInfo" class="ml-2 font-medium">{{ contactInfo }}</span></p>
          <button type="button" :disabled="busy" class="min-h-10 text-sm font-medium text-primary-700 hover:underline disabled:opacity-50 dark:text-primary-300" data-testid="journey-refresh" @click="emit('refresh')">{{ t('firstUseJourney.refresh') }}</button>
        </div>
        <button v-else type="button" :disabled="busy" class="btn btn-secondary mt-4 min-h-11" data-testid="journey-refresh" @click="emit('refresh')">{{ t('firstUseJourney.retry') }}</button>
      </template>
    </div>
    <div v-if="keyReady && fundingReady && !hasRequests" class="rounded-xl border border-primary-100 bg-primary-50/50 p-4 dark:border-primary-900 dark:bg-primary-950/20" data-testid="journey-verify-usage">
      <p class="text-sm leading-6 text-gray-700 dark:text-gray-200">{{ t('firstUseJourney.verifyUsageBody') }}</p>
      <button type="button" :disabled="busy || loading" class="btn btn-secondary mt-3 min-h-11" @click="emit('refresh')">{{ t('firstUseJourney.verifyUsage') }}</button>
    </div>
    <p v-if="hasRequests" class="text-sm leading-6 text-gray-600 dark:text-gray-300" data-testid="journey-request-history">{{ t('firstUseJourney.usageReady') }} · <RouterLink to="/usage" class="font-medium text-primary-700 hover:underline dark:text-primary-300">{{ t('firstUseJourney.viewUsage') }}</RouterLink></p>
    <p v-else-if="phase !== 'simple'" class="text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('firstUseJourney.fundingNotUse') }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { FundingPhase } from './funding-readiness'

const props = withDefaults(defineProps<{
  phase: FundingPhase; balance: number | null; loading?: boolean; onlineAvailable: boolean | null; canRedeem: boolean; simpleMode?: boolean
  subscriptionOnly?: boolean; keyReady?: boolean; requestCount?: number | null; busy?: boolean; context?: 'setup' | 'dashboard'; scene?: 'code' | 'image'; contactInfo?: string
}>(), { loading: false, simpleMode: false, subscriptionOnly: false, keyReady: false, requestCount: null, busy: false, context: 'setup', scene: 'code', contactInfo: '' })
const emit = defineEmits<{ refresh: []; continue: [] }>()
const { t } = useI18n()
const phase = computed(() => props.simpleMode ? 'simple' : props.phase)
const fundingReady = computed(() => ['balance', 'subscription', 'simple'].includes(phase.value))
const hasRequests = computed(() => typeof props.requestCount === 'number' && props.requestCount > 0)
const currentStep = computed(() => hasRequests.value ? null : !fundingReady.value ? 'funding' : !props.keyReady ? 'key' : 'use')
const steps = computed(() => [
  { id: 'account', done: true }, { id: 'funding', done: fundingReady.value || hasRequests.value },
  { id: 'key', done: props.keyReady || hasRequests.value }, { id: 'use', done: hasRequests.value },
].filter(step => phase.value !== 'simple' || step.id !== 'funding'))
const fundingTitle = computed(() => t(`firstUseJourney.${phase.value === 'balance' ? 'balanceReady' : phase.value === 'subscription' ? 'subscriptionReady' : phase.value === 'simple' ? 'simpleReady' : phase.value === 'unknown' ? 'unknownTitle' : 'fundingTitle'}`, { balance: typeof props.balance === 'number' ? props.balance.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 4 }) : '' }))
const fundingBody = computed(() => t(`firstUseJourney.${phase.value === 'balance' ? 'balanceReadyBody' : phase.value === 'subscription' ? 'subscriptionReadyBody' : phase.value === 'simple' ? 'simpleReadyBody' : phase.value === 'unknown' ? 'unknownBody' : 'fundingBody'}`))
const purchaseTo = computed(() => ({ path: '/purchase', query: { tab: props.subscriptionOnly ? 'subscription' : 'recharge', scene: props.scene } }))
const redeemTo = computed(() => ({ path: '/redeem', query: { scene: props.scene } }))
function preventIfBusy(event: MouseEvent) { if (props.busy) event.preventDefault() }
</script>
