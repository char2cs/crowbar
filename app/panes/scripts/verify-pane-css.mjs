#!/usr/bin/env node
/**
 * verify-pane-css.mjs
 *
 * Proves every pane bundle carries the Tailwind utilities its components use. A pane is built
 * with its own Vite root (`entries/<pane>`), and Tailwind scans for class names from there
 * unless the stylesheet names its source, so a bundle once shipped with ~30 KB of CSS and
 * unstyled panes. Every pane uses `.flex`, so its absence means the scan missed the sources.
 */

import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const PANES_DIR = resolve(dirname(fileURLToPath(import.meta.url)), '..', 'dist-panes')
const UTILITY = /\.flex\{/

if (!existsSync(PANES_DIR)) {
  console.error('verify-pane-css: no dist-panes directory; run build:panes first')
  process.exit(1)
}

const bare = readdirSync(PANES_DIR).filter((pane) => {
  const assets = join(PANES_DIR, pane, 'assets')
  if (!existsSync(assets)) return false
  const css = readdirSync(assets).filter((file) => file.endsWith('.css'))
  return !css.some((file) => UTILITY.test(readFileSync(join(assets, file), 'utf8')))
})

if (bare.length > 0) {
  console.error(`verify-pane-css: no Tailwind utilities in the CSS of: ${bare.join(', ')}`)
  process.exit(1)
}
