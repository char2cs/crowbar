import { useRef, useState } from 'react'
import { ArrowClockwise } from '@phosphor-icons/react'
import { getBuildInfo } from '@/lib/build-info'
import Section, { SettingRow } from '../settings-section'
import { SETTINGS_CONTROL_WIDTHS } from '../settings-control-widths'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { useSettingsStore, getDefaultSetting } from '@/features/settings/store'
import type { Settings } from '@/features/settings/types/settings'
import { downloadSettingsFile } from '@/features/settings/lib/settings-download'
import { exportDiagnostics } from '@/features/settings/lib/diagnostics-export'
import { isTauri } from '@/lib/crowbar-bridge'
import { primitiveConfirm } from '@/components/ui/primitive-dialog-service'
import { toast } from '@/features/window/stores/toast-store'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@/components/ui/select'
import { ChatSplitViewSetting } from './chat-split-view-setting'

function handleExport() {
  downloadSettingsFile(useSettingsStore.getState().settings)
  toast.success('Settings exported', 'Saved crowbar-settings.json to your downloads.')
}

async function handleImportFile(event: React.ChangeEvent<HTMLInputElement>) {
  const file = event.target.files?.[0]
  // Allow re-selecting the same file later.
  event.target.value = ''
  if (!file) return

  try {
    const text = await file.text()
    const ok = useSettingsStore.getState().updateSettingsFromJSON(text)
    if (ok) {
      toast.success('Settings imported', 'Your settings have been restored from the file.')
    } else {
      toast.error('Import failed', 'The file is not a valid Crowbar settings export.')
    }
  } catch {
    toast.error('Import failed', 'Could not read the selected file.')
  }
}

async function handleReset() {
  const confirmed = await primitiveConfirm(
    'Reset all settings to their defaults? This cannot be undone.',
    {
      title: 'Reset settings',
      confirmLabel: 'Reset all',
      cancelLabel: 'Cancel',
    },
  )
  if (!confirmed) return

  await useSettingsStore.getState().resetToDefaults()
  toast.success('Settings reset', 'All settings have been restored to their defaults.')
}

// Persisted diagnostic overlays. Split out of DeveloperSettings so each stays
// small and readable; both rows drive `useSettingsStore` directly.
function PerformanceSection() {
  const showFpsOverlay = useSettingsStore((s) => s.settings.showFpsOverlay)
  const updateSetting = useSettingsStore((s) => s.updateSetting)

  return (
    <Section
      title="Performance"
      description="Diagnostic overlays that appear on top of the editor. Persisted across restarts."
    >
      <SettingRow
        label="FPS overlay"
        description="Show a live frame-rate counter in the bottom-right corner — fps, worst frame time, and drop count per 500ms window."
        onReset={() => updateSetting('showFpsOverlay', getDefaultSetting('showFpsOverlay'))}
        canReset={showFpsOverlay !== getDefaultSetting('showFpsOverlay')}
      >
        <Switch
          checked={showFpsOverlay}
          onChange={(checked) => updateSetting('showFpsOverlay', checked)}
          size="sm"
        />
      </SettingRow>
    </Section>
  )
}

const BUILD_BADGE_OPTIONS: { value: Settings['buildBadgeOverride']; label: string }[] = [
  { value: 'auto', label: `Auto (detected: ${getBuildInfo().channel})` },
  { value: 'dev', label: 'Force: Development' },
  { value: 'nightly', label: 'Force: Nightly' },
  { value: 'beta', label: 'Force: Beta' },
  { value: 'release', label: 'Force: Release' },
  { value: 'off', label: 'Off' },
]

export function BuildBadgeSection() {
  const buildBadgeOverride = useSettingsStore((s) => s.settings.buildBadgeOverride)
  const updateSetting = useSettingsStore((s) => s.updateSetting)

  function cycle() {
    const i = BUILD_BADGE_OPTIONS.findIndex((opt) => opt.value === buildBadgeOverride)
    const next = BUILD_BADGE_OPTIONS[(i + 1) % BUILD_BADGE_OPTIONS.length]!.value
    updateSetting('buildBadgeOverride', next)
  }

  return (
    <Section
      title="Build Badge"
      description="The build-state indicator in the sidebar header's dead space, next to the traffic lights."
    >
      <SettingRow
        label="Mode"
        description="Auto detects the channel from the running build. Force a state to preview it regardless of the actual build, or cycle through every state."
        onReset={() => updateSetting('buildBadgeOverride', getDefaultSetting('buildBadgeOverride'))}
        canReset={buildBadgeOverride !== getDefaultSetting('buildBadgeOverride')}
      >
        <div className="flex items-center gap-1">
          <Select
            value={buildBadgeOverride}
            onValueChange={(v) =>
              updateSetting('buildBadgeOverride', v as Settings['buildBadgeOverride'])
            }
          >
            <SelectTrigger className={SETTINGS_CONTROL_WIDTHS.wide} size="sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {BUILD_BADGE_OPTIONS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            type="button"
            variant="outline"
            size="icon-sm"
            onClick={cycle}
            tooltip="Cycle to the next mode"
            tooltipSide="bottom"
            aria-label="Cycle build badge mode"
          >
            <ArrowClockwise />
          </Button>
        </div>
      </SettingRow>
    </Section>
  )
}

export function DeveloperSettings() {
  const fileInputRef = useRef<HTMLInputElement>(null)

  const [exportingDiagnostics, setExportingDiagnostics] = useState(false)

  async function handleExportDiagnostics() {
    setExportingDiagnostics(true)
    try {
      const path = await exportDiagnostics()
      toast.success('Diagnostics exported', path)
    } catch (e) {
      toast.error('Diagnostics export failed', e instanceof Error ? e.message : String(e))
    } finally {
      setExportingDiagnostics(false)
    }
  }

  return (
    <div className="space-y-4">
      <PerformanceSection />

      <BuildBadgeSection />

      <ChatSplitViewSetting />

      {isTauri() && (
        <Section
          title="Diagnostics"
          description="For bug reports: collect the backend and app logs plus a live backend snapshot into a single zip."
        >
          <SettingRow
            label="Export diagnostics"
            description="Bundles the daemon log (crashes, watchdog dumps), the app log, fresh goroutine/heap dumps, and version info into your Downloads folder."
          >
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={exportingDiagnostics}
              onClick={handleExportDiagnostics}
            >
              {exportingDiagnostics ? 'Exporting…' : 'Export diagnostics'}
            </Button>
          </SettingRow>
        </Section>
      )}

      <Section
        title="Backup & Restore"
        description="Export your settings to a file, import a previous export, or reset everything to defaults."
      >
        <SettingRow
          label="Export settings"
          description="Download all current settings as a versioned crowbar-settings.json file."
        >
          <Button type="button" variant="outline" size="sm" onClick={handleExport}>
            Export settings
          </Button>
        </SettingRow>

        <SettingRow
          label="Import settings"
          description="Restore settings from a previously exported crowbar-settings.json file."
        >
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => fileInputRef.current?.click()}
          >
            Import settings
          </Button>
          <input
            ref={fileInputRef}
            type="file"
            accept="application/json"
            className="hidden"
            onChange={handleImportFile}
          />
        </SettingRow>

        <SettingRow
          label="Reset all settings"
          description="Restore every setting to its default value. This cannot be undone."
        >
          <Button type="button" variant="destructive" size="sm" onClick={handleReset}>
            Reset all
          </Button>
        </SettingRow>
      </Section>
    </div>
  )
}
