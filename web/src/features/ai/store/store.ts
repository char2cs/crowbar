// Stub
import { create } from "zustand"
import { createSelectors } from "@/utils/zustand-selectors"
import type { AiModel } from "@/features/ai/types/providers"

interface AiStoreState {
  messages: unknown[]
  isLoading: boolean
  dynamicModels: Record<string, AiModel[]>
  setDynamicModels: (models: Record<string, AiModel[]>) => void
  hasProviderApiKey: (providerId: string) => boolean
  checkAllProviderApiKeys: () => Record<string, boolean>
  providerApiKeys: Record<string, string>
}

const useAiStoreBase = create<AiStoreState>((set) => ({
  messages: [],
  isLoading: false,
  dynamicModels: {},
  setDynamicModels: (models) => set({ dynamicModels: models }),
  hasProviderApiKey: (_providerId: string) => false,
  checkAllProviderApiKeys: () => ({}),
  providerApiKeys: {},
}))

export const useAiStore = createSelectors(useAiStoreBase)
/** Athas alias for useAiStore */
export const useAIChatStore = useAiStore
