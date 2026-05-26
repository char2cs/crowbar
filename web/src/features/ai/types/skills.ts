// Stub
export interface AiSkill {
  id: string
  name?: string
  title: string
  content?: string
  author?: string
  createdAt?: string | number
  updatedAt?: string | number
  upstreamUpdatedAt?: string | number
  upstreamContent?: string
  upstreamDescription?: string
  upstreamTitle?: string
  localOverride?: boolean
  tags?: string[]
  version?: string
  sourceId?: string
  source?: string
  description?: string
}
export type AIChatSkill = AiSkill
