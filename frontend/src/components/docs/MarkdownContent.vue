<template>
  <div class="markdown-content" v-html="html" @click="handleClick"></div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import DOMPurify from 'dompurify'
import { marked } from 'marked'

const props = defineProps<{
  content: string
  copyLabel?: string
  copiedLabel?: string
}>()

const html = computed(() => {
  const rendered = marked.parse(props.content || '') as string
  const sanitized = DOMPurify.sanitize(rendered, {
    FORBID_TAGS: ['iframe', 'object', 'embed'],
    FORBID_ATTR: ['style']
  })
  return sanitized.replace(/<pre>/g, `<pre><button type="button" class="code-copy" aria-label="${props.copyLabel || 'Copy'}">${props.copyLabel || 'Copy'}</button>`)
})

async function handleClick(event: MouseEvent) {
  const button = (event.target as HTMLElement).closest<HTMLButtonElement>('.code-copy')
  if (!button) return
  const code = button.parentElement?.querySelector('code')?.textContent || ''
  try {
    await navigator.clipboard.writeText(code)
    button.textContent = props.copiedLabel || 'Copied'
    window.setTimeout(() => { button.textContent = props.copyLabel || 'Copy' }, 1600)
  } catch {
    button.textContent = props.copyLabel || 'Copy'
  }
}
</script>

<style>
.markdown-content { color: inherit; font-size: 15px; line-height: 1.82; overflow-wrap: anywhere; }
.markdown-content h1, .markdown-content h2, .markdown-content h3, .markdown-content h4 { color: inherit; font-weight: 750; letter-spacing: -.025em; line-height: 1.3; }
.markdown-content h1 { margin: 0 0 24px; font-size: clamp(30px, 4vw, 42px); }
.markdown-content h2 { margin: 42px 0 16px; padding-bottom: 10px; border-bottom: 1px solid #eadfd7; font-size: 25px; }
.markdown-content h3 { margin: 30px 0 12px; font-size: 20px; }
.markdown-content h4 { margin: 24px 0 10px; font-size: 17px; }
.markdown-content p { margin: 0 0 18px; color: #584f49; }
.markdown-content ul, .markdown-content ol { margin: 0 0 20px; padding-left: 26px; color: #584f49; }
.markdown-content ul { list-style: disc; }
.markdown-content ol { list-style: decimal; }
.markdown-content li { margin: 7px 0; }
.markdown-content a { color: #a64508; text-decoration: underline; text-underline-offset: 3px; }
.markdown-content blockquote { margin: 24px 0; padding: 14px 18px; border-left: 4px solid #c45100; border-radius: 0 8px 8px 0; color: #6b5143; background: #fef4ed; }
.markdown-content blockquote p { margin: 0; color: inherit; }
.markdown-content code { padding: 2px 6px; border-radius: 5px; color: #8c3700; background: #f5e9e1; font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: .9em; }
.markdown-content pre { position: relative; margin: 24px 0; overflow-x: auto; border: 1px solid #45352d; border-radius: 10px; background: #211b18; padding: 22px 18px; }
.markdown-content pre code { padding: 0; color: #f8eee8; background: transparent; font-size: 13px; line-height: 1.7; }
.markdown-content .code-copy { position: absolute; top: 8px; right: 8px; padding: 5px 9px; border: 1px solid #705e54; border-radius: 6px; color: #e8d9d0; background: #382e29; font-size: 11px; cursor: pointer; }
.markdown-content table { width: 100%; margin: 24px 0; border-collapse: collapse; display: block; overflow-x: auto; }
.markdown-content th, .markdown-content td { min-width: 120px; padding: 10px 14px; border: 1px solid #dfd2c9; text-align: left; }
.markdown-content th { background: #f7f0ea; font-weight: 700; }
.markdown-content hr { margin: 34px 0; border: 0; border-top: 1px solid #eadfd7; }
.theme-dark .markdown-content h2, .theme-dark .markdown-content hr { border-color: #4d4641; }
.theme-dark .markdown-content p, .theme-dark .markdown-content ul, .theme-dark .markdown-content ol { color: #cec4bc; }
.theme-dark .markdown-content a { color: #ffad7b; }
.theme-dark .markdown-content blockquote { color: #f0d2c2; background: #3d271c; }
.theme-dark .markdown-content code { color: #ffc09b; background: #3b302a; }
.theme-dark .markdown-content th, .theme-dark .markdown-content td { border-color: #514a45; }
.theme-dark .markdown-content th { background: #302d29; }
</style>
