// Stub
import { create } from "zustand"
import { createSelectors } from "@/utils/zustand-selectors"
import type { AiModel } from "@/features/ai/types/providers"

interface AiStoreState {
  messages: unknown[]
  isLoading: boolean
  dynamicModels: Record<string, AiModel[]>
  setDynamicModels: (providerIdOrModels: string | Record<string, AiModel[]>, models?: AiModel[]) => void
  hasProviderApiKey: (providerId: string) => boolean
  checkAllProviderApiKeys: () => Record<string, boolean>
  providerApiKeys: Map<string, string>
}

const useAiStoreBase = create<AiStoreState>((set, _get) => ({
  messages: [],
  isLoading: false,
  dynamicModels: {},
  setDynamicModels: (providerIdOrModels, models) => {
    if (typeof providerIdOrModels === 'string' && models !== undefined) {
      set((state) => ({ dynamicModels: { ...state.dynamicModels, [providerIdOrModels]: models } }))
    } else if (typeof providerIdOrModels === 'object') {
      set({ dynamicModels: providerIdOrModels })
    }
  },
  hasProviderApiKey: (_providerId: string) => false,
  checkAllProviderApiKeys: () => ({}),
  providerApiKeys: new Map<string, string>(),
}))

export const useAiStore = createSelectors(useAiStoreBase)
/** Athas alias for useAiStore */
export const useAIChatStore = useAiStore
