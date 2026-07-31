import { describe, expect, it } from 'vitest'
import {
  applyProviderPreferences,
  buildProviderPreferences,
  providerDisabledMap,
  reorderProviderIds,
} from '@/features/settings/components/tabs/provider-preferences'
import type { AgentProvider } from '@/features/agent/api/agent-api'

const provider = (id: string, enabled: boolean, mcpEnabled = true): AgentProvider => ({
  id,
  displayName: id,
  icon: '',
  connected: true,
  enabled,
  mcpEnabled,
})

describe('provider-preferences', () => {
  it('providerDisabledMap inverts both switches per id', () => {
    const map = providerDisabledMap([
      provider('codex', true, false),
      provider('claude', false, true),
    ])
    expect(map).toEqual({
      codex: { disabled: false, mcpDisabled: true },
      claude: { disabled: true, mcpDisabled: false },
    })
  })

  it('buildProviderPreferences maps ordered ids + flags to the wire payload', () => {
    expect(
      buildProviderPreferences(['codex', 'claude'], {
        claude: { disabled: true, mcpDisabled: true },
      }),
    ).toEqual([
      { id: 'codex', disabled: false, mcpDisabled: false },
      { id: 'claude', disabled: true, mcpDisabled: true },
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
    // the current flags, and rebuild the payload.
    const providers = [provider('claude', true), provider('codex', false)]
    const reordered = reorderProviderIds(
      providers.map((p) => p.id),
      'codex',
      'claude',
    )
    expect(buildProviderPreferences(reordered, providerDisabledMap(providers))).toEqual([
      { id: 'codex', disabled: true, mcpDisabled: false },
      { id: 'claude', disabled: false, mcpDisabled: false },
    ])
  })

  // ── The landmine ────────────────────────────────────────────────────
  // The backend upserts WHOLE ROWS. A payload that names only `disabled` does not
  // leave `mcpDisabled` alone — it writes the zero value over it. So a provider
  // whose tools the user had just switched off would have them switched back on
  // by the next drag, or the next unrelated enable/disable, and nothing in the UI
  // would say so. Every write carries both flags for every provider.
  it('a reorder preserves mcpDisabled on every row', () => {
    const providers = [provider('claude', true, false), provider('codex', true, true)]
    const reordered = reorderProviderIds(
      providers.map((p) => p.id),
      'codex',
      'claude',
    )
    expect(buildProviderPreferences(reordered, providerDisabledMap(providers))).toEqual([
      { id: 'codex', disabled: false, mcpDisabled: false },
      { id: 'claude', disabled: false, mcpDisabled: true },
    ])
  })

  it('an enable toggle preserves mcpDisabled on the row it flips', () => {
    const providers = [provider('claude', true, false)]
    const flags = providerDisabledMap(providers)
    // What the tab's handleToggle composes: flip ONE FIELD of one row.
    const next = { ...flags, claude: { disabled: true, mcpDisabled: flags.claude.mcpDisabled } }
    expect(buildProviderPreferences(['claude'], next)).toEqual([
      { id: 'claude', disabled: true, mcpDisabled: true },
    ])
  })

  // The OPTIMISTIC half of a write: the same intent the payload carries, applied
  // to the live list so the switch moves on click instead of on the response.
  describe('applyProviderPreferences', () => {
    it('reorders the list and rewrites both switches from the flags map', () => {
      const providers = [provider('claude', true), provider('codex', true)]
      expect(
        applyProviderPreferences(providers, ['codex', 'claude'], {
          claude: { disabled: true, mcpDisabled: true },
        }),
      ).toEqual([provider('codex', true, true), provider('claude', false, false)])
    })

    it('keeps every other field of the provider row untouched', () => {
      const providers = [{ ...provider('codex', true), displayName: 'Codex', connected: false }]
      expect(applyProviderPreferences(providers, ['codex'], {})[0]).toMatchObject({
        displayName: 'Codex',
        connected: false,
        enabled: true,
        mcpEnabled: true,
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
    const flags = providerDisabledMap(providers)
    const disabled = { ...flags, codex: { disabled: true, mcpDisabled: false } } // disable codex
    expect(
      buildProviderPreferences(
        providers.map((p) => p.id),
        disabled,
      ),
    ).toEqual([
      { id: 'codex', disabled: true, mcpDisabled: false },
      { id: 'claude', disabled: false, mcpDisabled: false },
    ])
  })
})
