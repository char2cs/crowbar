import { Switch } from '@/components/ui/switch'
import { getDefaultSetting, useSettingsStore } from '@/features/settings/store'
import Section, { SettingRow } from '../settings-section'

// Split shows the hook-reconstructed chat beside the CLI's TUI (the ground truth) to expose gaps.
// A diagnostic, opt-in and off by default: the terminal keeps rendering instead of resting hidden.
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
