import { describe, expect, it } from 'vitest'
import {
  declaredModel,
  effectiveSelection,
  stageSelection,
} from '@/features/agent/lib/staged-selection'

const live = { providerId: 'claude', model: 'opus', effort: 'high' }

describe('effectiveSelection', () => {
  it('shows the live selection when nothing is staged', () => {
    expect(effectiveSelection(null, live)).toEqual(live)
  })

  it('shows the staged pick while the chat is still as it was when picked', () => {
    const staged = stageSelection(live, { providerId: 'codex', model: 'gpt', effort: 'low' })
    expect(effectiveSelection(staged, live)).toEqual({
      providerId: 'codex',
      model: 'gpt',
      effort: 'low',
    })
  })

  it('yields to the chat once it changes elsewhere, such as a switch made in the TUI', () => {
    const staged = stageSelection(live, { providerId: 'codex', model: 'gpt', effort: 'low' })
    const switched = { providerId: 'claude', model: 'sonnet', effort: 'high' }
    expect(effectiveSelection(staged, switched)).toEqual(switched)
  })
})

// A pane switch keeps the outgoing provider's model as the chat's pick; the chip
// must not name a Claude model beside the Codex icon.
describe('declaredModel', () => {
  it('names a model the provider declares', () => {
    expect(declaredModel({ models: ['gpt-6-luna'] }, 'gpt-6-luna')).toBe('gpt-6-luna')
  })

  it('reads a model the provider does not declare as its default', () => {
    expect(declaredModel({ models: ['gpt-6-luna'] }, 'haiku')).toBe('')
  })

  it('keeps any model for a provider with no catalogue to check against', () => {
    expect(declaredModel({ models: [] }, 'haiku')).toBe('haiku')
    expect(declaredModel(undefined, 'haiku')).toBe('haiku')
  })
})
