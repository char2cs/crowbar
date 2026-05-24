// Stub
export interface AiProviderInterface {
  id: string
  name: string
}

export interface ProviderModel {
  id: string
  name: string
  providerId?: string
  maxTokens?: number
  contextWindow?: number
}

export interface AiProviderInstance {
  buildHeaders: (apiKey?: string) => Record<string, string>
  buildPayload: (request: unknown) => unknown
  buildUrl?: (request?: unknown) => string
  apiUrl?: string
  getModels?: (apiKey?: string) => Promise<import('@/features/ai/types/providers').AiModel[]>
}
