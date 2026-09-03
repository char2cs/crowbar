/**
 * "Chat is the default presentation" (spec §11): a USER-level preference, on by
 * default, that picks the surface a chat LANDS on and nothing else.
 *
 * Two things have to hold, and only one of them is about the default value:
 *
 * 1. It lives in the global settings store, not the per-workspace registry. That
 *    registry is DESTROYED on a workspace switch, so a preference parked there
 *    would silently revert the first time the user changed workspace. The switch
 *    below is a real destroy/recreate, and the workspace copy it takes with it is
 *    asserted alongside the preference that survives it — otherwise a test that
 *    never actually destroyed anything would pass just as green.
 * 2. Turning it OFF takes nothing away. Its whole output domain is the two
 *    surfaces the chat's own switcher already moves between, and flipping it
 *    writes exactly one settings key — it cannot disable Chat because it has no
 *    value that means "no Chat".
 */
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import {
  getDefaultChatPresentation,
  selectDefaultChatPresentation,
  type ChatPresentation,
} from '@/features/settings/lib/chat-presentation'
import { useSettingsStore } from '@/features/settings/store'
import {
  defaultSettings,
  getDefaultSettingsSnapshot,
} from '@/features/settings/config/default-settings'
import {
  normalizeSettingValue,
  normalizeSettings,
} from '@/features/settings/lib/settings-normalization'
import {
  loadSettingsFromStore,
  saveSettingsToStore,
} from '@/features/settings/lib/settings-persistence'
import type { Settings } from '@/features/settings/types/settings'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'

const CLAUDE: AgentProvider = {
  id: 'claude',
  displayName: 'Claude',
  icon: '<svg></svg>',
  connected: true,
  enabled: true,
  mcpEnabled: true,
}

/** Every surface the preference can name. There is no third value, and no
 *  absence — which is why turning it off cannot remove a surface. */
const SURFACES: ChatPresentation[] = ['chat', 'terminal']

beforeEach(() => {
  localStorage.clear()
  useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
})

afterEach(() => {
  useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
  destroyWorkspaceStore('w1')
  destroyWorkspaceStore('w2')
  setActiveWorkspaceId('')
  localStorage.clear()
})

describe('the chat landing-surface preference', () => {
  describe('with nothing stored', () => {
    it('is on, so a chat lands on Chat', () => {
      expect(defaultSettings.chatIsDefaultPresentation).toBe(true)
      expect(getDefaultChatPresentation()).toBe('chat')
    })

    it('reads back as on from an empty store', async () => {
      const loaded = await loadSettingsFromStore()
      expect(loaded.chatIsDefaultPresentation).toBe(true)
      expect(selectDefaultChatPresentation({ settings: loaded })).toBe('chat')
    })

    // An older profile, or a hand-edited export, carries no choice at all here —
    // which is not the same as choosing Terminal.
    it.each([undefined, null, 'terminal', 0])(
      'treats a stored %p as no choice and stays on Chat',
      (stored) => {
        const settings = {
          ...getDefaultSettingsSnapshot(),
          chatIsDefaultPresentation: stored,
        } as unknown as Settings

        expect(normalizeSettings(settings).chatIsDefaultPresentation).toBe(true)
        expect(
          normalizeSettingValue('chatIsDefaultPresentation', stored as unknown as boolean),
        ).toBe(true)
      },
    )
  })

  describe('once turned off', () => {
    it('lands the chat on Terminal', async () => {
      await useSettingsStore.getState().updateSetting('chatIsDefaultPresentation', false)
      expect(getDefaultChatPresentation()).toBe('terminal')
    })

    // The claim under test is that this is a LANDING choice, not a capability
    // one: the value it produces is one of the two surfaces the chat's switcher
    // already toggles between, and the write touches nothing else in Settings.
    it('changes the landing surface and nothing else', async () => {
      const before = { ...useSettingsStore.getState().settings }
      await useSettingsStore.getState().updateSetting('chatIsDefaultPresentation', false)
      const after = useSettingsStore.getState().settings

      const changed = (Object.keys(after) as Array<keyof Settings>).filter(
        (key) => after[key] !== before[key],
      )
      expect(changed).toEqual(['chatIsDefaultPresentation'])
      expect(SURFACES).toContain(getDefaultChatPresentation())
    })

    it('is reversible, and Chat is what it comes back to', async () => {
      await useSettingsStore.getState().updateSetting('chatIsDefaultPresentation', false)
      await useSettingsStore
        .getState()
        .updateSetting('chatIsDefaultPresentation', defaultSettings.chatIsDefaultPresentation)
      expect(getDefaultChatPresentation()).toBe('chat')
    })
  })

  describe('across a workspace switch', () => {
    it('survives one, while the per-workspace copy does not', async () => {
      await useSettingsStore.getState().updateSetting('chatIsDefaultPresentation', false)

      const w1 = getOrCreateWorkspaceStore('w1')
      setActiveWorkspaceId('w1')
      w1.getState().setAgentProviders([CLAUDE])
      expect(w1.getState().agentChats.providers).toEqual([CLAUDE])

      // THE SWITCH, as the app performs it: the outgoing workspace's store is
      // destroyed and the incoming one is built from nothing.
      destroyWorkspaceStore('w1')
      setActiveWorkspaceId('w2')
      getOrCreateWorkspaceStore('w2')

      // The control: anything that lived in the registry is gone. Re-entering w1
      // gets a brand-new store, not the one that held the providers.
      expect(getOrCreateWorkspaceStore('w1').getState().agentChats.providers).toEqual([])

      // The preference is not in the registry, so the switch cannot touch it.
      expect(useSettingsStore.getState().settings.chatIsDefaultPresentation).toBe(false)
      expect(getDefaultChatPresentation()).toBe('terminal')
    })

    it('is still off after a reload, not just for this session', async () => {
      await saveSettingsToStore({ chatIsDefaultPresentation: false })

      destroyWorkspaceStore('w1')
      setActiveWorkspaceId('w2')
      getOrCreateWorkspaceStore('w2')

      const reloaded = await loadSettingsFromStore()
      expect(reloaded.chatIsDefaultPresentation).toBe(false)
      expect(selectDefaultChatPresentation({ settings: reloaded })).toBe('terminal')
    })
  })
})
