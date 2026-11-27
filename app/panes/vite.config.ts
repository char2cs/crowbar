import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { TanStackRouterVite } from '@tanstack/router-plugin/vite'
import path from 'path'
import { execSync } from 'child_process'

const gitSHA = (() => {
  try {
    // react-doctor-disable-next-line import-metadata-execution-risk -- FALSE POSITIVE: the exec argument is a hardcoded static `git rev-parse` literal, not imported metadata/upload/manifest. The rule's regex only fires because the unrelated word "plugin" (Vite's `plugins:` array below) falls within 200 semicolon-free chars of this call in a semicolon-less file. Build-time only; no attacker-influenceable input reaches exec.
    return execSync('git rev-parse --short HEAD').toString().trim()
  } catch {
    return 'dev'
  }
})()

export default defineConfig({
  plugins: [
    TanStackRouterVite({ target: 'react', autoCodeSplitting: true }),
    react(),
    tailwindcss(),
  ],
  define: {
    __APP_VERSION__: JSON.stringify(gitSHA),
  },
  worker: { format: 'es' },
  server: { port: 5173 },
  optimizeDeps: {
    include: ['@tauri-apps/api/event'],
  },
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
      'remark-mdx': path.resolve(import.meta.dirname, './src/lib/stubs/remark-mdx.ts'),
    },
    dedupe: ['react', 'react-dom'],
  },
  test: {
    environment: 'jsdom',
    environmentOptions: {
      jsdom: { url: 'http://localhost' },
    },
    globals: true,
    setupFiles: ['./src/__tests__/setup.ts'],
    env: { VITE_API_URL: '' },
    alias: {
      '@tauri-apps/api/core': path.resolve(
        import.meta.dirname,
        './src/__tests__/__mocks__/tauri-api-core.ts',
      ),
      'katex/dist/katex.min.css': path.resolve(
        import.meta.dirname,
        './src/__tests__/__mocks__/katex-css-stub.ts',
      ),
    },
    server: {
      deps: {
        inline: [/@platejs\/math/, /katex/],
      },
    },
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      thresholds: { lines: 66, functions: 60, branches: 56, statements: 65 },
    },
  },
})
