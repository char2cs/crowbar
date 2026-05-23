import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import { ArrowUp } from 'lucide-react'

interface ChatInputProps {
  placeholder: string
  onSend: (message: string) => void
  modelName?: string
  tokenPct?: number
  disabled?: boolean
}

export function ChatInput({ placeholder, onSend, modelName, tokenPct = 0, disabled }: ChatInputProps) {
  const [value, setValue] = useState('')

  const handleSend = () => {
    if (!value.trim()) return
    onSend(value.trim())
    setValue('')
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) handleSend()
  }

  return (
    <div className="border-t border-border bg-card px-4 pb-4 pt-2.5">
      <div className="rounded-xl border border-border bg-background px-3 pt-2.5 pb-2 focus-within:ring-1 focus-within:ring-primary">
        <Textarea
          placeholder={placeholder}
          value={value}
          onChange={e => setValue(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={disabled}
          className="min-h-5 resize-none border-0 bg-transparent p-0 text-[13px] shadow-none focus-visible:ring-0"
          rows={1}
        />
        <div className="mt-2 flex items-center gap-1.5">
          {modelName && (
            <Button variant="outline" size="sm" className="h-7 gap-1.5 px-2 text-[12px] text-muted-foreground">
              <span>✦</span><span>{modelName}</span>
            </Button>
          )}
          {tokenPct > 0 && (
            <Badge variant="outline" className="h-7 gap-1.5 px-2 text-[11px] text-muted-foreground">
              <Progress value={tokenPct} className="h-[3px] w-7" />
              {tokenPct}%
            </Badge>
          )}
          <Button variant="outline" size="sm" className="h-7 px-2 text-[12px] text-muted-foreground">
            Max
          </Button>
          <Button
            size="icon"
            className="ml-auto h-7 w-7"
            onClick={handleSend}
            disabled={disabled || !value.trim()}
            aria-label="send"
          >
            <ArrowUp className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  )
}
