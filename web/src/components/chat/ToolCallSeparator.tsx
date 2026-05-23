import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'

interface ToolCallSeparatorProps {
  toolCalls: number
  durationSec: number
}

export function ToolCallSeparator({ toolCalls, durationSec }: ToolCallSeparatorProps) {
  const label = `${toolCalls} tool ${toolCalls === 1 ? 'call' : 'calls'} · ${durationSec}s`
  return (
    <div className="flex items-center gap-2 px-6 pb-3 pt-1">
      <Separator className="flex-1" />
      <Badge variant="outline" className="cursor-pointer text-[10.5px] text-muted-foreground/50 hover:text-muted-foreground">
        {label}
      </Badge>
      <Separator className="flex-1" />
    </div>
  )
}
