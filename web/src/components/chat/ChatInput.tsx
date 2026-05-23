import { useState } from 'react'
import { ArrowUp, Paperclip } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { useModelPreference, MODELS } from '@/hooks/useModelPreference'
import {
  PromptInput,
  PromptInputTextarea,
  PromptInputFooter,
  PromptInputTools,
  type PromptInputMessage,
  usePromptInputAttachments,
} from '@/components/ai-elements/prompt-input'
import {
  ModelSelector,
  ModelSelectorTrigger,
  ModelSelectorContent,
  ModelSelectorInput,
  ModelSelectorList,
  ModelSelectorGroup,
  ModelSelectorItem,
} from '@/components/ai-elements/model-selector'

const EFFORT_LEVELS = [
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Mid' },
  { value: 'high', label: 'High' },
  { value: 'max', label: 'Max' },
] as const

type EffortLevel = typeof EFFORT_LEVELS[number]['value']

interface ChatInputProps {
  placeholder: string
  onSend: (message: string, attachments: File[]) => void
  disabled?: boolean
}

// Inner component that can access PromptInput context for the attach button
function AttachButton() {
  const attachments = usePromptInputAttachments()
  return (
    <Button
      variant="ghost"
      size="icon"
      className="h-7 w-7 text-muted-foreground"
      type="button"
      aria-label="Attach file"
      onClick={() => attachments.openFileDialog()}
    >
      <Paperclip className="h-3.5 w-3.5" />
    </Button>
  )
}

export function ChatInput({ placeholder, onSend, disabled }: ChatInputProps) {
  const [textLength, setTextLength] = useState(0)
  const [effort, setEffort] = useState<EffortLevel>('medium')
  const [modelPickerOpen, setModelPickerOpen] = useState(false)
  const { setModel, modelLabel } = useModelPreference()

  const tokenPct = Math.min(Math.round((textLength / 4 / 200000) * 100), 100)

  const handleSubmit = (msg: PromptInputMessage) => {
    const files = msg.files
      .map((f: any) => f.file)
      .filter((f: any): f is File => f instanceof File)
    onSend(msg.text, files)
  }

  // base-ui ToggleGroup uses value as array; wrap effort in array for controlled single-select
  const effortValue: readonly EffortLevel[] = [effort]
  const handleEffortChange = (values: string[]) => {
    const next = values[values.length - 1] as EffortLevel | undefined
    if (next) setEffort(next)
  }

  return (
    <div className="border-t border-border bg-card px-4 pb-4 pt-2.5">
      <PromptInput
        onSubmit={handleSubmit}
        multiple
        className="rounded-xl border border-border bg-background"
      >
        <PromptInputTextarea
          placeholder={placeholder}
          onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) => setTextLength(e.target.value.length)}
          className="min-h-5 resize-none border-0 bg-transparent p-3 text-[13px] shadow-none focus-visible:ring-0"
          disabled={disabled}
        />

        <PromptInputFooter className="flex items-center gap-1.5 px-3 pb-2">
          <PromptInputTools>
            {/* Attach — uses context to open the hidden file input managed by PromptInput */}
            <AttachButton />

            {/* Model selector */}
            <ModelSelector open={modelPickerOpen} onOpenChange={setModelPickerOpen}>
              {/* DialogTrigger from base-ui doesn't expose asChild in its TS types, so use render prop pattern */}
              <ModelSelectorTrigger>
                <Button
                  variant="outline"
                  size="sm"
                  type="button"
                  className="h-7 gap-1.5 px-2 text-[12px] text-muted-foreground"
                >
                  <span>✦</span>
                  <span>{modelLabel}</span>
                </Button>
              </ModelSelectorTrigger>
              <ModelSelectorContent>
                <ModelSelectorInput placeholder="Search models…" />
                <ModelSelectorList>
                  <ModelSelectorGroup>
                    {MODELS.map(m => (
                      <ModelSelectorItem
                        key={m.id}
                        value={m.id}
                        onSelect={() => {
                          setModel(m.id)
                          setModelPickerOpen(false)
                        }}
                      >
                        {m.label}
                      </ModelSelectorItem>
                    ))}
                  </ModelSelectorGroup>
                </ModelSelectorList>
              </ModelSelectorContent>
            </ModelSelector>

            {/* Effort level — base-ui ToggleGroup uses value as array */}
            <ToggleGroup
              value={effortValue}
              onValueChange={handleEffortChange}
              className="h-7 gap-0 rounded-md border border-border"
            >
              {EFFORT_LEVELS.map(l => (
                <ToggleGroupItem
                  key={l.value}
                  value={l.value}
                  className="h-7 rounded-none px-2 text-[11px] first:rounded-l-md last:rounded-r-md data-[state=on]:bg-primary data-[state=on]:text-primary-foreground"
                >
                  {l.label}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>

            {/* Token indicator */}
            {tokenPct > 0 && (
              <Badge variant="outline" className="h-7 gap-1.5 px-2 text-[11px] text-muted-foreground">
                <Progress value={tokenPct} className="h-[3px] w-7" />
                {tokenPct}%
              </Badge>
            )}
          </PromptInputTools>

          {/* Send */}
          <button
            type="submit"
            aria-label="send"
            disabled={disabled}
            className="ml-auto flex h-7 w-7 items-center justify-center rounded-md bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            <ArrowUp className="h-4 w-4" />
          </button>
        </PromptInputFooter>
      </PromptInput>
    </div>
  )
}
