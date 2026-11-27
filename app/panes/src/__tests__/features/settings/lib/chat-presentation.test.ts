import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import {
  getDefaultChatPresentation,
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
  loadPersistedSettings,
  persistSettings,
} from '@/features/settings/lib/settings-persistence'
import type { Settings } from '@/features/settings/types/settings'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'

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
      const loaded = await loadPersistedSettings()
      expect(loaded.chatIsDefaultPresentation).toBe(true)
      useSettingsStore.setState({ settings: loaded })
      expect(getDefaultChatPresentation()).toBe('chat')
    })

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
    it('survives one', async () => {
      await useSettingsStore.getState().updateSetting('chatIsDefaultPresentation', false)

      getOrCreateWorkspaceStore('w1')
      setActiveWorkspaceId('w1')

      destroyWorkspaceStore('w1')
      setActiveWorkspaceId('w2')
      getOrCreateWorkspaceStore('w2')

      expect(useSettingsStore.getState().settings.chatIsDefaultPresentation).toBe(false)
      expect(getDefaultChatPresentation()).toBe('terminal')
    })

    it('is still off after a reload, not just for this session', async () => {
      await persistSettings({ chatIsDefaultPresentation: false })

      destroyWorkspaceStore('w1')
      setActiveWorkspaceId('w2')
      getOrCreateWorkspaceStore('w2')

      const reloaded = await loadPersistedSettings()
      expect(reloaded.chatIsDefaultPresentation).toBe(false)
      useSettingsStore.setState({ settings: reloaded })
      expect(getDefaultChatPresentation()).toBe('terminal')
    })
  })
})
