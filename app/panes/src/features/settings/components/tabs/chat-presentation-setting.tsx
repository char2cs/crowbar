import { getDefaultSetting, useSettingsStore } from '@/features/settings/store'
import { Switch } from '@/components/ui/switch'
import Section, { SettingRow } from '../settings-section'

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
