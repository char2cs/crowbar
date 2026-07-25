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
    // Pre-bundle Tauri packages so the dev server never serves them as
    // "Outdated Optimize Dep" 504s when running inside the Tauri shell.
    include: ['@tauri-apps/api/event'],
  },
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
    // Force a single copy of react/react-dom — pnpm's virtual store can
    // resolve multiple versions when peer deps differ across packages.
    dedupe: ['react', 'react-dom'],
  },
  test: {
    environment: 'jsdom',
    environmentOptions: {
      jsdom: { url: 'http://localhost' },
    },
    globals: true,
    setupFiles: ['./src/__tests__/setup.ts'],
    alias: {
      // Stub the Tauri desktop API — it is only available inside the native shell.
      '@tauri-apps/api/core': path.resolve(
        __dirname,
        './src/__tests__/__mocks__/tauri-api-core.ts',
      ),
      // Vitest externalizes node_modules imports by default (bypassing Vite's
      // CSS-to-JS transform), so both math-kit's and @platejs/math's own
      // `katex/dist/katex.min.css` side-effect import hit Node's raw ESM
      // loader and fail on the `.css` extension — see the stub file for detail.
      // Only takes effect once the module is inlined (below): an externalized
      // import is a raw Node `import()` that never consults this alias.
      'katex/dist/katex.min.css': path.resolve(
        __dirname,
        './src/__tests__/__mocks__/katex-css-stub.ts',
      ),
    },
    server: {
      deps: {
        // Force @platejs/math (and its nested katex copy) through Vite's own
        // transform pipeline instead of Node's raw ESM loader, so both the
        // CSS alias above and Vite's module resolution actually apply to it.
        inline: [/@platejs\/math/, /katex/],
      },
    },
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      // Ratchet floor, not a target: set just under measured coverage so the
      // gate is green yet still fails on regressions. Raise as tests grow
      // (long-term goal: 70 across the board).
      thresholds: { lines: 28, functions: 50, branches: 70, statements: 28 },
    },
  },
})
