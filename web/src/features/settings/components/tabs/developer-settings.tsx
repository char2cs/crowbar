import { useChaosStore } from '@/lib/store/chaos'
import Section, { SETTINGS_CONTROL_WIDTHS, SettingRow } from '../settings-section'
import NumberInput from '@/components/ui/number-input'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'

export function DeveloperSettings() {
  const latency = useChaosStore((s) => s.latency)
  const errorRate = useChaosStore((s) => s.errorRate)
  const setLatency = useChaosStore((s) => s.setLatency)
  const setErrorRate = useChaosStore((s) => s.setErrorRate)
  const reset = useChaosStore((s) => s.reset)

  return (
    <div className="space-y-4">
      <Section title="Network Chaos" description="Simulate poor network conditions against the Go API server.">
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
            Reset all chaos settings
          </Button>
        </div>
      </Section>
    </div>
  )
}
