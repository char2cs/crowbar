import { describe, expect, it } from 'vitest'
import {
  applyProviderPreferences,
  buildProviderPreferences,
  providerDisabledMap,
  reorderProviderIds,
} from '@/features/settings/components/tabs/provider-preferences'
import type { AgentProvider } from '@/features/agent/api/agent-api'

const provider = (id: string, enabled: boolean): AgentProvider => ({
  id,
  displayName: id,
  icon: '',
  connected: true,
  enabled,
})

describe('provider-preferences', () => {
  it('providerDisabledMap inverts enabled per id', () => {
    const map = providerDisabledMap([provider('codex', true), provider('claude', false)])
    expect(map).toEqual({ codex: false, claude: true })
  })

  it('buildProviderPreferences maps ordered ids + disabled flags to the wire payload', () => {
    expect(buildProviderPreferences(['codex', 'claude'], { claude: true })).toEqual([
      { id: 'codex', disabled: false },
      { id: 'claude', disabled: true },
    ])
  })

  it('reorderProviderIds moves the dragged id into the target slot', () => {
    expect(reorderProviderIds(['claude', 'codex'], 'codex', 'claude')).toEqual(['codex', 'claude'])
  })

  it('reorderProviderIds is a no-op when the drag did not move or an id is unknown', () => {
    expect(reorderProviderIds(['claude', 'codex'], 'codex', 'codex')).toEqual(['claude', 'codex'])
    expect(reorderProviderIds(['claude', 'codex'], 'ghost', 'claude')).toEqual(['claude', 'codex'])
  })

  it('reorder → payload: a drag produces the full ordered preference set', () => {
    // The exact composition the tab's onDragEnd performs: reorder the ids, keep
    // the current disabled flags, and rebuild the payload.
    const providers = [provider('claude', true), provider('codex', false)]
    const reordered = reorderProviderIds(
      providers.map((p) => p.id),
      'codex',
      'claude',
    )
    expect(buildProviderPreferences(reordered, providerDisabledMap(providers))).toEqual([
      { id: 'codex', disabled: true },
      { id: 'claude', disabled: false },
    ])
  })

  // The OPTIMISTIC half of a write: the same intent the payload carries, applied
  // to the live list so the switch moves on click instead of on the response.
  describe('applyProviderPreferences', () => {
    it('reorders the list and rewrites enabled from the disabled map', () => {
      const providers = [provider('claude', true), provider('codex', true)]
      expect(applyProviderPreferences(providers, ['codex', 'claude'], { claude: true })).toEqual([
        { ...provider('codex', true) },
        { ...provider('claude', false) },
      ])
    })

    it('keeps every other field of the provider row untouched', () => {
      const providers = [{ ...provider('codex', true), displayName: 'Codex', connected: false }]
      expect(applyProviderPreferences(providers, ['codex'], {})[0]).toMatchObject({
        displayName: 'Codex',
        connected: false,
        enabled: true,
      })
    })

    it('ignores ids the list does not have', () => {
      const providers = [provider('codex', true)]
      expect(applyProviderPreferences(providers, ['ghost', 'codex'], {})).toEqual([
        provider('codex', true),
      ])
    })
  })

  it('toggle → payload: flipping one id keeps order and the other flags', () => {
    const providers = [provider('codex', true), provider('claude', true)]
    const disabled = { ...providerDisabledMap(providers), codex: true } // disable codex
    expect(
      buildProviderPreferences(
        providers.map((p) => p.id),
        disabled,
      ),
    ).toEqual([
      { id: 'codex', disabled: true },
      { id: 'claude', disabled: false },
    ])
  })
})
