import { Button } from '@/components/ui/button'

interface InlineErrorProps {
  error: Error
  onRetry: () => void
  title?: string
}

export function InlineError({ error, onRetry, title = 'Failed to load' }: InlineErrorProps) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center">
      <span className="text-lg opacity-50">⚠</span>
      <p className="text-sm font-medium text-foreground">{title}</p>
      {import.meta.env.DEV && (
        <p className="font-mono text-[11px] text-muted-foreground">{error.message}</p>
      )}
      <Button variant="outline" size="sm" onClick={onRetry} className="mt-1">
        ↺ Retry
      </Button>
    </div>
  )
}
