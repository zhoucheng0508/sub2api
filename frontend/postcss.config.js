import { fileURLToPath } from 'node:url'

// CUSTOM(VOTE-AI-BUILD): resolve the branded Tailwind config from the frontend root.
const tailwindConfig = fileURLToPath(new URL('./tailwind.config.js', import.meta.url))

export default {
  plugins: {
    tailwindcss: { config: tailwindConfig },
    autoprefixer: {}
  }
}
