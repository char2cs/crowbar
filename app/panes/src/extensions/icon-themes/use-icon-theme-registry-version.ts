import { useSyncExternalStore } from 'react'
import { iconThemeRegistry } from './icon-theme-registry'

const subscribe = (onChange: () => void) => iconThemeRegistry.onRegistryChange(onChange)
const getVersion = () => iconThemeRegistry.getVersion()

/** Bumps on any registry change, including lazily loaded icons arriving; use it as a memo
 *  dependency for anything derived from a theme. */
export function useIconThemeRegistryVersion(): number {
  return useSyncExternalStore(subscribe, getVersion, getVersion)
}
