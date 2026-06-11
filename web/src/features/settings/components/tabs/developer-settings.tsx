import { useRef, useState } from 'react'
import { useChaosStore, FAULT_KEYS, FAULT_LABELS } from '@/lib/store/chaos'
import type { Scenario } from '@/lib/store/chaos'
import Section, { SETTINGS_CONTROL_WIDTHS, SettingRow } from '../settings-section'
import NumberInput from '@/components/ui/number-input'
import { Button } from '@/components/ui/button'
import { useSettingsStore } from '@/features/settings/store'
import { downloadSettingsFile } from '@/features/settings/lib/settings-download'
import { primitiveConfirm } from '@/components/ui/primitive-dialog-service'
import { toast } from '@/features/window/stores/toast-store'
import { Slider } from '@/components/ui/slider'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@/components/ui/select'
import { cn } from '@/utils/cn'

const SCENARIO_OPTIONS: { value: Scenario; label: string; description: string }[] = [
  {
    value: 'extreme',
    label: 'Extreme',
    description: '4 repos · 50+ workspaces · 1M-line diffs · 28 threads/PR',
  },
  {
    value: 'normal',
    label: 'Normal — Rabbyte',
    description: '1 repo · 3 workspaces · realistic daily usage',
  },
  { value: 'empty', label: 'Empty', description: 'New user — no repos, no workspaces' },
]

export function DeveloperSettings() {
  const latency = useChaosStore((s) => s.latency)
  const errorRate = useChaosStore((s) => s.errorRate)
  const scenario = useChaosStore((s) => s.scenario)
  const faults = useChaosStore((s) => s.faults)
  const setLatency = useChaosStore((s) => s.setLatency)
  const setErrorRate = useChaosStore((s) => s.setErrorRate)
  const setFault = useChaosStore((s) => s.setFault)
  const reset = useChaosStore((s) => s.reset)
  const resetFaults = useChaosStore((s) => s.resetFaults)
  const applyScenario = useChaosStore((s) => s.applyScenario)

  const [selectedScenario, setSelectedScenario] = useState<Scenario>(scenario)
  const [applying, setApplying] = useState(false)

  async function handleApply() {
    setApplying(true)
    await applyScenario(selectedScenario)
    // page reloads — this line never executes
  }

  const anyFaultActive = FAULT_KEYS.some((k) => faults[k] > 0)

  const fileInputRef = useRef<HTMLInputElement>(null)

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

  return (
    <div className="space-y-4">
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

      <Section
        title="Network Chaos"
        description="Simulate poor network conditions against the Go API server."
      >
        <SettingRow
          label="Latency"
          description="Extra delay added to every API request via X-Crowbar-Latency header"
          onReset={() => setLatency(0)}
          canReset={latency !== 0}
          resetLabel="Reset latency"
        >
          <NumberInput
            min="0"
            max="5000"
            step="50"
            value={latency}
            onChange={setLatency}
            className={cn(SETTINGS_CONTROL_WIDTHS.number, 'tabular-nums')}
            size="xs"
            aria-label={`Latency: ${latency} milliseconds`}
          />
        </SettingRow>

        <SettingRow
          label="Error Rate"
          description="Probability each API request returns a 500 via X-Crowbar-Error-Rate header"
          onReset={() => setErrorRate(0)}
          canReset={errorRate !== 0}
          resetLabel="Reset error rate"
        >
          <NumberInput
            min="0"
            max="100"
            step="5"
            value={Math.round(errorRate * 100)}
            onChange={(v) => setErrorRate(v / 100)}
            className={cn(SETTINGS_CONTROL_WIDTHS.number, 'tabular-nums')}
            size="xs"
            aria-label={`Error rate: ${Math.round(errorRate * 100)} percent`}
          />
        </SettingRow>

        <div className="px-1 pt-1">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={reset}
            disabled={latency === 0 && errorRate === 0}
          >
            Reset network chaos
          </Button>
        </div>
      </Section>

      {import.meta.env.VITE_USE_MOCK === 'true' && (
        <>
          <Section
            title="Mock Scenario"
            description="Switch the full mock dataset. Clears all local caches and reloads the app."
          >
            <SettingRow
              label="Scenario"
              description={SCENARIO_OPTIONS.find((o) => o.value === selectedScenario)?.description}
            >
              <div className="flex items-center gap-2">
                <Select
                  value={selectedScenario}
                  onValueChange={(v) => setSelectedScenario(v as Scenario)}
                >
                  <SelectTrigger className={SETTINGS_CONTROL_WIDTHS.wide} size="sm">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {SCENARIO_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {opt.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button
                  type="button"
                  variant="default"
                  size="sm"
                  onClick={handleApply}
                  disabled={applying || selectedScenario === scenario}
                >
                  {applying ? 'Applying…' : 'Apply & Reload'}
                </Button>
              </div>
            </SettingRow>
            {scenario !== selectedScenario && (
              <p className="px-1 text-xs text-muted-foreground">
                Current:{' '}
                <span className="font-medium">
                  {SCENARIO_OPTIONS.find((o) => o.value === scenario)?.label}
                </span>
                {' → pending: '}
                <span className="font-medium text-foreground">
                  {SCENARIO_OPTIONS.find((o) => o.value === selectedScenario)?.label}
                </span>
              </p>
            )}
          </Section>

          <Section
            title="Fault Injection"
            description="Force specific API endpoints to return 500 errors. Changes take effect on the next request — no reload needed."
          >
            {/* Purpose-built rows (not SettingRow): a fixed label column, a slider
                that fills the available width, and a percentage with a guaranteed
                right gutter (pr-2) so it never clips against the panel edge at any
                dialog width. */}
            <div className="flex flex-col gap-0.5 pr-2">
              {FAULT_KEYS.map((key) => (
                <div
                  key={key}
                  className="flex items-center gap-4 rounded-lg px-1 py-1.5 transition-colors hover:bg-muted/50"
                >
                  <span className="w-32 shrink-0 truncate text-sm text-foreground">
                    {FAULT_LABELS[key]}
                  </span>
                  <Slider
                    value={faults[key]}
                    onValueChange={(values) =>
                      setFault(key, Array.isArray(values) ? (values[0] ?? 0) : values)
                    }
                    min={0}
                    max={100}
                    step={5}
                    className="min-w-0 flex-1"
                    aria-label={`${FAULT_LABELS[key]} fault rate`}
                  />
                  <span
                    className={cn(
                      'w-9 shrink-0 text-right text-xs tabular-nums',
                      faults[key] > 0 ? 'text-destructive font-medium' : 'text-muted-foreground',
                    )}
                  >
                    {faults[key]}%
                  </span>
                </div>
              ))}
            </div>
            <div className="px-1 pt-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={resetFaults}
                disabled={!anyFaultActive}
              >
                Reset all faults
              </Button>
            </div>
          </Section>
        </>
      )}
    </div>
  )
}
