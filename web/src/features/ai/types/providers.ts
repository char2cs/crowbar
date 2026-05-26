// Stub — minimal provider registry so normalization logic works correctly.
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

const PROVIDERS: AiProvider[] = [
  { id: 'anthropic', name: 'Anthropic', requiresApiKey: true, models: [{ id: 'claude-sonnet-4-6', name: 'Claude Sonnet 4.6' }] },
  { id: 'openai',    name: 'OpenAI',    requiresApiKey: true, models: [{ id: 'gpt-4o', name: 'GPT-4o' }] },
  { id: 'custom',    name: 'Custom',    requiresApiKey: false, models: [] },
]

export function getAvailableProviders(): AiProvider[] { return PROVIDERS }
export function getProviderById(id: string): AiProvider | null {
  return PROVIDERS.find(p => p.id === id) ?? null
}
export function getModelById(_providerId: string, _modelId?: string): AiModel | null { return null }
