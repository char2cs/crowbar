import { describe, expect, it } from 'vitest'
import { stageSelection, effectiveSelection } from '@/features/agent/lib/staged-selection'

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
