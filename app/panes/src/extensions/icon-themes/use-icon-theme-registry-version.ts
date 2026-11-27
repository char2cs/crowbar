import { useSyncExternalStore } from 'react'
import { iconThemeRegistry } from './icon-theme-registry'

const subscribe = (onChange: () => void) => iconThemeRegistry.onRegistryChange(onChange)
const getVersion = () => iconThemeRegistry.getVersion()

export function useIconThemeRegistryVersion(): number {
  return useSyncExternalStore(subscribe, getVersion, getVersion)
}
