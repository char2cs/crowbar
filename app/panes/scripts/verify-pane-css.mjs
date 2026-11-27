#!/usr/bin/env node

import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const PANES_DIR = resolve(dirname(fileURLToPath(import.meta.url)), '..', 'dist-panes')
const UTILITY = /\.flex\{/
const UI_FONT_FACE = /url\(\.\.\/fonts\/CalSansUI\.woff2\)/

if (!existsSync(PANES_DIR)) {
  console.error('verify-pane-css: no dist-panes directory; run build:panes first')
  process.exit(1)
}

const cssOf = (pane) => {
  const assets = join(PANES_DIR, pane, 'assets')
  if (!existsSync(assets)) return []
  return readdirSync(assets)
    .filter((file) => file.endsWith('.css'))
    .map((file) => readFileSync(join(assets, file), 'utf8'))
}

const panes = readdirSync(PANES_DIR).filter((pane) => cssOf(pane).length > 0)
const bare = panes.filter((pane) => !cssOf(pane).some((css) => UTILITY.test(css)))
const noFont = panes.filter(
  (pane) =>
    !cssOf(pane).some((css) => UI_FONT_FACE.test(css)) ||
    !existsSync(join(PANES_DIR, pane, 'fonts', 'CalSansUI.woff2')),
)

if (bare.length > 0) {
  console.error(`verify-pane-css: no Tailwind utilities in the CSS of: ${bare.join(', ')}`)
  process.exit(1)
}

if (noFont.length > 0) {
  console.error(`verify-pane-css: no CalSansUI @font-face or font file in: ${noFont.join(', ')}`)
  process.exit(1)
}
