<template>
  <img
    v-if="assetSrc"
    :src="assetSrc"
    :alt="label"
    class="h-6 w-6 shrink-0 rounded-md object-contain"
    loading="lazy"
    aria-hidden="true"
  />
  <ModelIcon
    v-else-if="brandModel"
    :model="brandModel"
    :size="sizePixels"
    class="shrink-0"
    aria-hidden="true"
  />
  <svg
    v-else
    :class="sizeClass"
    viewBox="0 0 24 24"
    xmlns="http://www.w3.org/2000/svg"
    :aria-label="label"
    role="img"
    fill="none"
    stroke="currentColor"
    stroke-width="2"
    stroke-linecap="round"
    stroke-linejoin="round"
  >
    <title>{{ label }}</title>
    <path v-for="(path, index) in iconPaths" :key="index" :d="path" />
  </svg>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import type { CcSwitchAppType } from '@/utils/ccswitchImport'

type IconSize = 'xs' | 'sm' | 'md' | 'lg' | 'xl'

const props = withDefaults(defineProps<{
  app: CcSwitchAppType
  label?: string
  size?: IconSize
}>(), {
  size: 'sm'
})

const sizeClass = computed(() => ({
  xs: 'h-3 w-3',
  sm: 'h-4 w-4',
  md: 'h-5 w-5',
  lg: 'h-6 w-6',
  xl: 'h-8 w-8'
}[props.size]))

const sizePixels = computed(() => ({
  xs: '12px',
  sm: '16px',
  md: '20px',
  lg: '24px',
  xl: '32px'
}[props.size]))

const label = computed(() => props.label || props.app)

// ModelIcon owns the @lobehub-derived paths and brand colors already used by
// the rest of the console, so the picker stays consistent without duplication.
const brandModel = computed(() => {
  switch (props.app) {
    case 'claude':
    case 'claude-desktop':
      return 'claude'
    case 'codex':
      return 'gpt-5'
    case 'gemini':
      return 'gemini'
    case 'grokbuild':
      return 'grok'
    default:
      return ''
  }
})

// These are the same local assets used by CC Switch's app switcher. Keeping
// them in the frontend bundle avoids a network/CSP dependency and preserves
// the recognizable marks at small sizes.
const assetSrc = computed(() => ({
  opencode: '/ccswitch-icons/opencode.svg',
  openclaw: '/ccswitch-icons/openclaw.svg',
  hermes: '/ccswitch-icons/hermes.png'
} as Partial<Record<CcSwitchAppType, string>>)[props.app] || '')

// Fallback glyphs remain available for a future asset packaging failure.
const APP_PATHS: Record<CcSwitchAppType, readonly string[]> = {
  claude: [],
  'claude-desktop': [],
  codex: [],
  gemini: [],
  grokbuild: [],
  opencode: ['M4 6l6 6-6 6', 'M12 18h8'],
  openclaw: ['M6 4c-1 4 1 6 3 7-3 0-5 2-5 5 0 2 2 4 4 4 2 0 3-1 4-3 1 2 2 3 4 3 2 0 4-2 4-4 0-3-2-5-5-5 2-1 4-3 3-7-2 2-3 3-5 3S8 6 6 4z', 'M9 13h6'],
  hermes: ['M4 7l3-3 5 4 5-4 3 3-3 3 3 3-3 3-5-4-5 4-3-3 3-3-3-3-3z', 'M9 20h6'],
  pi: ['M4 5h16', 'M8 5v8a5 5 0 005 5h2', 'M14 5v14']
}

const iconPaths = computed(() => APP_PATHS[props.app])
</script>
