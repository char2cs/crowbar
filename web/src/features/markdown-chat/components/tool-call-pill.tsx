import { useState, type CSSProperties } from 'react'
import type { ToolCallData } from '../types'

interface ToolCallPillProps {
  data: ToolCallData
}

// Theme tokens (no hardcoded palette): warning = pending, info = done,
// destructive = error. Subtle tinted background + the token as the text color.
const tint = (token: string): CSSProperties => ({
  background: `color-mix(in srgb, var(${token}) 15%, transparent)`,
  color: `var(${token})`,
})

const STATUS_STYLE: Record<ToolCallData['status'], CSSProperties> = {
  pending: tint('--warning'),
  done: tint('--info'),
  error: tint('--destructive'),
}

export function ToolCallPill({ data }: ToolCallPillProps) {
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="my-1 inline-flex flex-col gap-1">
      <button
        onClick={() => setExpanded((v) => !v)}
        className="inline-flex items-center gap-1.5 rounded-full border border-border bg-muted px-2.5 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-muted/80"
      >
        <span className="text-muted-foreground/60">{expanded ? '▾' : '▸'}</span>
        <span className="font-mono font-medium">{data.name}</span>
        {data.args.path != null && (
          <span className="max-w-48 truncate text-muted-foreground/70">
            {String(data.args.path)}
          </span>
        )}
        <span
          className="rounded px-1 py-0.5 text-[10px] font-semibold"
          style={STATUS_STYLE[data.status]}
        >
          {data.status}
        </span>
      </button>

      {expanded && (
        <div className="ml-3 rounded border border-border bg-card p-2 text-xs font-mono">
          <div className="mb-1 text-muted-foreground/60">args</div>
          <pre className="whitespace-pre-wrap text-foreground">
            {JSON.stringify(data.args, null, 2)}
          </pre>
          {data.output && (
            <>
              <div className="mb-1 mt-2 text-muted-foreground/60">output</div>
              <pre className="max-h-40 overflow-auto whitespace-pre-wrap text-foreground">
                {data.output}
              </pre>
            </>
          )}
        </div>
      )}
    </div>
  )
}
