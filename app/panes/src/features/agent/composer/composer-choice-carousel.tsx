import { useState } from 'react'
import { ComposerChoice } from '@/features/agent/composer/composer-choice'
import type { AskItem } from '@/features/agent/stores/lib/transcript-items'

interface ComposerChoiceCarouselProps {
  asks: AskItem[]
  wsId: string
  chatId: string
  providerLabel: string
  onOpenTerminal: () => void
}

export function ComposerChoiceCarousel({
  asks,
  wsId,
  chatId,
  providerLabel,
  onOpenTerminal,
}: ComposerChoiceCarouselProps) {
  const [index, setIndex] = useState(0)
  const current = Math.min(index, asks.length - 1)
  const go = (to: number) => setIndex(Math.min(Math.max(to, 0), asks.length - 1))
  return (
    <div className="choice-carousel" data-testid="agent-choice-stack">
      {asks.map((ask, i) => (
        <ComposerChoice
          key={ask.id}
          wsId={wsId}
          chatId={chatId}
          choice={ask.choice}
          toolTarget={ask.toolTarget}
          providerLabel={providerLabel}
          onOpenTerminal={onOpenTerminal}
          family={ask.family}
          active={i === current}
          position={
            asks.length > 1
              ? {
                  index: current,
                  total: asks.length,
                  noun: 'request',
                  onPrev: () => go(current - 1),
                  onNext: () => go(current + 1),
                }
              : undefined
          }
          request={{
            toolName: ask.choice.toolName,
            toolTarget: ask.toolTarget,
            toolInput: ask.toolInput,
            truncated: ask.toolInputTruncated,
            mcpServer: ask.mcpServer,
            why: ask.detail,
          }}
        />
      ))}
    </div>
  )
}
