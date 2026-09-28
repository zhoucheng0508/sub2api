import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'
export default defineConfig({
  base: '/preview/',
  plugins: [vue()],
  resolve: { alias: { '@': resolve(__dirname, 'src') } },
  build: { outDir: '../rikka-preview-dist', rollupOptions: { input: resolve(__dirname, 'rikka-preview.html') } }
})
