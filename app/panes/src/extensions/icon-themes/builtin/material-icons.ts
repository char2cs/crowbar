import { iconThemeRegistry } from '../icon-theme-registry'

type MaterialIcons = typeof import('material-file-icons')

let icons: MaterialIcons | null = null
let loading: Promise<void> | null = null

/** @internal */
export function loadMaterialIcons(): Promise<void> {
  loading ??= import('material-file-icons').then((mod) => {
    icons = mod
    iconThemeRegistry.notifyChanged()
  })
  return loading
}

export function materialIconSvg(fileName: string): string | null {
  if (icons) return icons.getIcon(fileName).svg
  void loadMaterialIcons()
  return null
}
