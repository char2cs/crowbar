// Stub
import { create } from 'zustand'
import { createSelectors } from '@/utils/zustand-selectors'

export interface ExtensionStoreState {
  extensions: unknown[]
  availableExtensions: Map<string, unknown>
  extensionsWithUpdates: Set<string>
  actions: {
    install: (_id: string) => Promise<void>
    uninstall: (_id: string) => Promise<void>
  }
}

const useExtensionStoreBase = create<ExtensionStoreState>(() => ({
  extensions: [],
  availableExtensions: new Map(),
  extensionsWithUpdates: new Set<string>(),
  actions: {
    install: async () => {},
    uninstall: async () => {},
  },
}))

export const useExtensionStore = createSelectors(useExtensionStoreBase)
export async function waitForExtensionStoreInitialization(): Promise<void> {}
