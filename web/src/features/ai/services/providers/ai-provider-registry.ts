// Stub
import type { AiModel } from "@/features/ai/types/providers"

export interface AiProviderInstance {
  id: string
  getModels?: (apiKey?: string) => Promise<AiModel[]>
  checkApiKey?: (apiKey: string) => Promise<boolean>
}

export const aiProviderRegistry = {
  getProvider: (_id: string): AiProviderInstance | null => null,
  getProviders: (): AiProviderInstance[] => [],
}
export function getProvider(_id: string): AiProviderInstance | null { return null }
export function setOllamaApiKey(_key: string): void {}
