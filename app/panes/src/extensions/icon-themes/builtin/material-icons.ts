import { iconThemeRegistry } from '../icon-theme-registry'

type MaterialIcons = typeof import('material-file-icons')

let icons: MaterialIcons | null = null
let loading: Promise<void> | null = null

/** @internal Loads the ~500 KB icon set on first use, out of the entry chunk. Until it lands
 *  callers draw their fallback glyph; then the registry announces a change and icons re-render. */
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
