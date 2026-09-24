import { iconThemeRegistry } from '../icon-theme-registry'

type MaterialIcons = typeof import('material-file-icons')

let icons: MaterialIcons | null = null
let loading: Promise<void> | null = null

/**
 * Loads `material-file-icons` (~500 KB raw: every icon's SVG source) on first
 * use instead of at boot, so it stays out of the entry chunk. When it lands,
 * the registry announces a change and every file icon re-renders with its
 * material glyph; until then the themes return nothing and callers draw their
 * own fallback glyph.
 */
/** @internal Exported for unit tests. */
export function loadMaterialIcons(): Promise<void> {
  loading ??= import('material-file-icons').then((mod) => {
    icons = mod
    iconThemeRegistry.notifyChanged()
  })
  return loading
}

/** The material SVG for `fileName`, or null while the icon set is loading. */
export function materialIconSvg(fileName: string): string | null {
  if (icons) return icons.getIcon(fileName).svg
  void loadMaterialIcons()
  return null
}
