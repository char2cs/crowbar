import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

const pane = process.env.PANE ?? 'chat'

export default defineConfig({
  root: path.resolve(import.meta.dirname, 'entries', pane),
  base: './',
  publicDir: path.resolve(import.meta.dirname, 'public'),
  plugins: [react(), tailwindcss()],
  define: { __APP_VERSION__: JSON.stringify('shell') },
  worker: { format: 'es' },
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
      'remark-mdx': path.resolve(import.meta.dirname, './src/lib/stubs/remark-mdx.ts'),
    },
    dedupe: ['react', 'react-dom'],
  },
  build: {
    outDir: path.resolve(import.meta.dirname, 'dist-panes', pane),
    emptyOutDir: true,
  },
})
