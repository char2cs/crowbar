// Stub
export interface AiModel {
  id: string
  name: string
  maxTokens?: number
  contextWindow?: number
  proOnly?: boolean
  description?: string
}

export interface AiProvider {
  id: string
  name: string
  maxTokens?: number
  proOnly?: boolean
  requiresApiKey?: boolean
  models: AiModel[]
  getModels?: () => Promise<AiModel[]>
}

export function getAvailableProviders(): AiProvider[] { return [] }
export function getProviderById(_id: string): AiProvider | null { return null }
export function getModelById(_id: string): AiModel | null { return null }
