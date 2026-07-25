import { Info } from '@phosphor-icons/react'
import { useEffect } from 'react'
import { getDefaultSetting, useSettingsStore } from '@/features/settings/store'
import { useFontStore } from '@/features/settings/stores/font-store'
import { COMMON_TERMINAL_NERD_FONTS } from '@/features/terminal/utils/terminal-fonts'
import NumberInput from '@/components/ui/number-input'
import Section, { SettingRow } from '../settings-section'
import { SETTINGS_CONTROL_WIDTHS } from '../settings-control-widths'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import Tooltip from '@/components/ui/tooltip'

const FONT_HELP_TEXT =
  'Note: Selected font must be installed on your system to work correctly. If icons are missing, try installing a Nerd Font.'

// react-doctor-disable-next-line no-giant-component -- accepted: cohesive settings tab — a flat list of SettingRow controls over the terminal settings slice; no logic to extract.
export const TerminalSettings = () => {
  const settings = useSettingsStore((state) => state.settings)
  const updateSetting = useSettingsStore((state) => state.updateSetting)
  const monospaceFonts = useFontStore.use.monospaceFonts()
  const { loadMonospaceFonts } = useFontStore.use.actions()

  useEffect(() => {
    loadMonospaceFonts()
  }, [loadMonospaceFonts])

  // Combine Nerd Fonts with system monospace fonts
  // Only include Nerd Fonts if they are actually installed on the system
  const installedNerdFonts = COMMON_TERMINAL_NERD_FONTS.filter((nerdFont) =>
    monospaceFonts.some((sysFont) => sysFont.family === nerdFont),
  )

  const fontOptions = installedNerdFonts.map((font) => ({
    value: font,
    label: `${font} (Nerd Font)`,
  }))
  for (const f of monospaceFonts) {
    if (!COMMON_TERMINAL_NERD_FONTS.includes(f.family)) {
      fontOptions.push({ value: f.family, label: f.family })
    }
  }

  // Add custom option if current value is not in list
  if (
    settings.terminalFontFamily &&
    !fontOptions.some((opt) => opt.value === settings.terminalFontFamily)
  ) {
    fontOptions.unshift({
      value: settings.terminalFontFamily,
      label: `${settings.terminalFontFamily} (Custom)`,
    })
  }

  return (
    <div className="space-y-4">
      <Section title="Typography">
        <SettingRow
          label="Font Family"
          description="Font family for the integrated terminal. Select a Nerd Font for best icon support."
          onReset={() =>
            updateSetting('terminalFontFamily', getDefaultSetting('terminalFontFamily'))
          }
          canReset={settings.terminalFontFamily !== getDefaultSetting('terminalFontFamily')}
        >
          <div className="flex items-center gap-2">
            <Select
              value={settings.terminalFontFamily}
              onValueChange={(val) => {
                if (val) updateSetting('terminalFontFamily', val)
              }}
            >
              <SelectTrigger size="sm" className={SETTINGS_CONTROL_WIDTHS.xwide}>
                <SelectValue placeholder="Select font..." />
              </SelectTrigger>
              <SelectContent>
                {fontOptions.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Tooltip content={FONT_HELP_TEXT} side="left">
              <Info className="size-4 cursor-help text-muted-foreground transition-colors hover:text-foreground" />
            </Tooltip>
          </div>
        </SettingRow>

        <SettingRow
          label="Font Size"
          description="Terminal font size in pixels"
          onReset={() => updateSetting('terminalFontSize', getDefaultSetting('terminalFontSize'))}
          canReset={settings.terminalFontSize !== getDefaultSetting('terminalFontSize')}
        >
          <NumberInput
            min="8"
            max="32"
            value={settings.terminalFontSize}
            onChange={(val) => updateSetting('terminalFontSize', val)}
            className={SETTINGS_CONTROL_WIDTHS.number}
            size="xs"
          />
        </SettingRow>

        <SettingRow
          label="Line Height"
          description="Line height multiplier"
          onReset={() =>
            updateSetting('terminalLineHeight', getDefaultSetting('terminalLineHeight'))
          }
          canReset={settings.terminalLineHeight !== getDefaultSetting('terminalLineHeight')}
        >
          <NumberInput
            min="1"
            max="2"
            step={0.1}
            value={settings.terminalLineHeight}
            onChange={(val) => updateSetting('terminalLineHeight', val)}
            className={SETTINGS_CONTROL_WIDTHS.number}
            size="xs"
          />
        </SettingRow>

        <SettingRow
          label="Letter Spacing"
          description="Additional spacing between characters"
          onReset={() =>
            updateSetting('terminalLetterSpacing', getDefaultSetting('terminalLetterSpacing'))
          }
          canReset={settings.terminalLetterSpacing !== getDefaultSetting('terminalLetterSpacing')}
        >
          <NumberInput
            min="-5"
            max="5"
            step={0.1}
            value={settings.terminalLetterSpacing}
            onChange={(val) => updateSetting('terminalLetterSpacing', val)}
            className={SETTINGS_CONTROL_WIDTHS.number}
            size="xs"
          />
        </SettingRow>

        <SettingRow
          label="Scrollback"
          description="How many lines of terminal history to keep in memory"
          onReset={() =>
            updateSetting('terminalScrollback', getDefaultSetting('terminalScrollback'))
          }
          canReset={settings.terminalScrollback !== getDefaultSetting('terminalScrollback')}
        >
          <NumberInput
            min="1000"
            max="100000"
            step={1000}
            value={settings.terminalScrollback}
            onChange={(val) => updateSetting('terminalScrollback', val)}
            className={SETTINGS_CONTROL_WIDTHS.default}
            size="xs"
          />
        </SettingRow>
      </Section>

      <Section title="Cursor">
        <SettingRow
          label="Cursor Style"
          description="Shape of the cursor"
          onReset={() =>
            updateSetting('terminalCursorStyle', getDefaultSetting('terminalCursorStyle'))
          }
          canReset={settings.terminalCursorStyle !== getDefaultSetting('terminalCursorStyle')}
        >
          <Select
            value={settings.terminalCursorStyle}
            onValueChange={(val) => {
              if (val) updateSetting('terminalCursorStyle', val as 'block' | 'underline' | 'bar')
            }}
          >
            <SelectTrigger size="sm" className={SETTINGS_CONTROL_WIDTHS.default}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {[
                { value: 'block', label: 'Block' },
                { value: 'underline', label: 'Underline' },
                { value: 'bar', label: 'Bar' },
              ].map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingRow>

        <SettingRow
          label="Blinking Cursor"
          description="Whether the cursor should blink"
          onReset={() =>
            updateSetting('terminalCursorBlink', getDefaultSetting('terminalCursorBlink'))
          }
          canReset={settings.terminalCursorBlink !== getDefaultSetting('terminalCursorBlink')}
        >
          <Switch
            checked={settings.terminalCursorBlink}
            onChange={(val) => updateSetting('terminalCursorBlink', val)}
            size="sm"
          />
        </SettingRow>

        <SettingRow
          label="Cursor Width"
          description="Thickness of the bar or block cursor"
          onReset={() =>
            updateSetting('terminalCursorWidth', getDefaultSetting('terminalCursorWidth'))
          }
          canReset={settings.terminalCursorWidth !== getDefaultSetting('terminalCursorWidth')}
        >
          <NumberInput
            min="1"
            max="6"
            value={settings.terminalCursorWidth}
            onChange={(val) => updateSetting('terminalCursorWidth', val)}
            className={SETTINGS_CONTROL_WIDTHS.number}
            size="xs"
          />
        </SettingRow>
      </Section>
    </div>
  )
}
