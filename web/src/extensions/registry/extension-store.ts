// Stub
import { create } from 'zustand'
import { createSelectors } from '@/utils/zustand-selectors'

export interface ExtensionManifest {
  id: string
  name: string
  displayName?: string
  version?: string
  installation?: unknown
}

export interface Extension {
  manifest: ExtensionManifest
  isInstalled: boolean
}

export interface ExtensionStoreState {
  extensions: Extension[]
  installedExtensions: Extension[]
  availableExtensions: Map<string, Extension>
  extensionsWithUpdates: Set<string>
  actions: {
    install: (_id: string) => Promise<void>
    uninstall: (_id: string) => Promise<void>
    getExtensionForFile: (_path: string) => Extension | null
  }
}

const useExtensionStoreBase = create<ExtensionStoreState>(() => ({
  extensions: [],
  installedExtensions: [],
  availableExtensions: new Map(),
  extensionsWithUpdates: new Set<string>(),
  actions: {
    install: async () => {},
    uninstall: async () => {},
    getExtensionForFile: (_path: string) => null,
  },
}))

export const useExtensionStore = createSelectors(useExtensionStoreBase)
