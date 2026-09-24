import { useSyncExternalStore } from 'react'
import { iconThemeRegistry } from './icon-theme-registry'

const subscribe = (onChange: () => void) => iconThemeRegistry.onRegistryChange(onChange)
const getVersion = () => iconThemeRegistry.getVersion()

/**
 * Re-renders the caller whenever the icon theme registry changes — a theme
 * registered or removed, or a theme's lazily loaded icons arriving. Use the
 * returned version as a memo dependency for anything derived from a theme.
 */
export function useIconThemeRegistryVersion(): number {
  return useSyncExternalStore(subscribe, getVersion, getVersion)
}
