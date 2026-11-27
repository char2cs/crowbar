import { getDefaultSetting, useSettingsStore } from '@/features/settings/store'
import Section, { SettingRow } from '../settings-section'
import { Switch } from '@/components/ui/switch'

export const GitSettings = () => {
  const settings = useSettingsStore((s) => s.settings)
  const updateSetting = useSettingsStore((s) => s.updateSetting)

  return (
    <div className="space-y-4">
      <Section title="Git View">
        <SettingRow
          label="Compact Git Status Badges"
          description="Use a denser layout for diff stats and staged labels in the Git panel"
          onReset={() =>
            updateSetting('compactGitStatusBadges', getDefaultSetting('compactGitStatusBadges'))
          }
          canReset={settings.compactGitStatusBadges !== getDefaultSetting('compactGitStatusBadges')}
        >
          <Switch
            checked={settings.compactGitStatusBadges}
            onChange={(checked) => updateSetting('compactGitStatusBadges', checked)}
            size="sm"
          />
        </SettingRow>
      </Section>

      <Section title="File Tree">
        <SettingRow
          label="Show Git Status In File Tree"
          description="Display Git color decorations in Files"
          onReset={() =>
            updateSetting('showGitStatusInFileTree', getDefaultSetting('showGitStatusInFileTree'))
          }
          canReset={
            settings.showGitStatusInFileTree !== getDefaultSetting('showGitStatusInFileTree')
          }
        >
          <Switch
            checked={settings.showGitStatusInFileTree}
            onChange={(checked) => updateSetting('showGitStatusInFileTree', checked)}
            size="sm"
          />
        </SettingRow>
      </Section>
    </div>
  )
}
