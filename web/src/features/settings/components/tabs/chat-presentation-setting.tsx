import { getDefaultSetting, useSettingsStore } from '@/features/settings/store'
import { Switch } from '@/components/ui/switch'
import Section, { SettingRow } from '../settings-section'

/**
 * Which surface a chat opens on. Its own component so the Agents tab stays
 * within react-doctor's `no-giant-component`.
 *
 * The copy has one job beyond naming the switch: to say that turning it OFF
 * takes nothing away. Chat and Terminal are peers inside every chat, and this
 * only decides which of the two is in front when the chat appears.
 */
export function ChatPresentationSetting() {
  const chatIsDefaultPresentation = useSettingsStore((s) => s.settings.chatIsDefaultPresentation)
  const updateSetting = useSettingsStore((s) => s.updateSetting)

  return (
    <Section title="Chat">
      <SettingRow
        label="Chat By Default"
        description="Open a chat on Chat rather than Terminal. Both surfaces stay in every chat either way — this only picks the one you land on."
        onReset={() =>
          updateSetting('chatIsDefaultPresentation', getDefaultSetting('chatIsDefaultPresentation'))
        }
        canReset={chatIsDefaultPresentation !== getDefaultSetting('chatIsDefaultPresentation')}
      >
        <Switch
          data-testid="chat-default-presentation-toggle"
          checked={chatIsDefaultPresentation}
          onChange={(checked) => updateSetting('chatIsDefaultPresentation', checked)}
          size="sm"
        />
      </SettingRow>
    </Section>
  )
}
