import { useChaosStore } from '@/lib/store/chaos'

export function ChaosPanel() {
  const latency = useChaosStore((s) => s.latency)
  const errorRate = useChaosStore((s) => s.errorRate)
  const setLatency = useChaosStore((s) => s.setLatency)
  const setErrorRate = useChaosStore((s) => s.setErrorRate)
  const reset = useChaosStore((s) => s.reset)

  return (
    <div className="fixed bottom-4 right-4 z-50 w-56 rounded-lg border border-border bg-background p-3 shadow-lg text-xs font-mono">
      <div className="mb-2 font-bold text-foreground">Chaos Panel</div>

      <label className="block mb-1 text-muted-foreground">
        Latency: {latency}ms
      </label>
      <input
        type="range"
        min={0}
        max={5000}
        step={100}
        value={latency}
        onChange={(e) => setLatency(Number(e.target.value))}
        className="w-full mb-2"
      />

      <label className="block mb-1 text-muted-foreground">
        Error rate: {(errorRate * 100).toFixed(0)}%
      </label>
      <input
        type="range"
        min={0}
        max={1}
        step={0.05}
        value={errorRate}
        onChange={(e) => setErrorRate(Number(e.target.value))}
        className="w-full mb-2"
      />

      <button
        onClick={reset}
        className="w-full rounded border border-border px-2 py-1 text-muted-foreground hover:bg-accent"
      >
        Reset
      </button>
    </div>
  )
}
