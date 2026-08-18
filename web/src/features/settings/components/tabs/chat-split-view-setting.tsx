import { Switch } from '@/components/ui/switch'
import { getDefaultSetting, useSettingsStore } from '@/features/settings/store'
import Section, { SettingRow } from '../settings-section'

/**
 * The switch behind the chat's Split surface — a DIAGNOSTIC INSTRUMENT, not a
 * feature, which is why it lives here and starts off.
 *
 * Crowbar reconstructs a conversation from hooks; the CLI's own TUI is the
 * ground truth. Every gap between the two — a turn that never closed, an
 * assistant message silently dropped, a prompt the chat could not see — is
 * obvious the moment you can read one against the other, and invisible
 * otherwise. Split puts them side by side so that comparison is a glance.
 *
 * It costs what a second live surface costs: the terminal keeps rendering
 * instead of resting under `display:none`. That is the whole reason this is
 * opt-in rather than always available.
 */
export function ChatSplitViewSetting() {
  const chatSplitPresentationEnabled = useSettingsStore(
    (s) => s.settings.chatSplitPresentationEnabled,
  )
  const updateSetting = useSettingsStore((s) => s.updateSetting)

  return (
    <Section
      title="Agent chats"
      description="Diagnostics for the chat surface itself. Persisted across restarts."
    >
      <SettingRow
        label="Split view"
        description="Add a third surface to every chat's switcher, showing the reconstructed chat and its live terminal side by side — for spotting where the two disagree. Both surfaces render at once, so leave it off unless you are looking at something."
        onReset={() =>
          updateSetting(
            'chatSplitPresentationEnabled',
            getDefaultSetting('chatSplitPresentationEnabled'),
          )
        }
        canReset={
          chatSplitPresentationEnabled !== getDefaultSetting('chatSplitPresentationEnabled')
        }
      >
        <Switch
          data-testid="chat-split-view-toggle"
          checked={chatSplitPresentationEnabled}
          onChange={(checked) => updateSetting('chatSplitPresentationEnabled', checked)}
          size="sm"
        />
      </SettingRow>
    </Section>
  )
}
