import { CaretLeftIcon, CaretRightIcon } from '@phosphor-icons/react'
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

/** The pending asks, one on screen at a time. Every card stays mounted so the picks, form
 *  values and errors of the one set aside are still there when the person comes back. */
export function ComposerChoiceCarousel({
  asks,
  wsId,
  chatId,
  providerLabel,
  onOpenTerminal,
}: ComposerChoiceCarouselProps) {
  const [index, setIndex] = useState(0)
  // Clamped on read: an ask that resolves can leave the index past the end.
  const current = Math.min(index, asks.length - 1)
  const go = (to: number) => setIndex((to + asks.length) % asks.length)
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
          stepper={
            asks.length > 1 && (
              <ChoiceStepper
                asks={asks}
                current={current}
                onPrev={() => go(current - 1)}
                onNext={() => go(current + 1)}
              />
            )
          }
          onSkip={asks.length > 1 ? () => go(current + 1) : undefined}
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

function ChoiceStepper({
  asks,
  current,
  onPrev,
  onNext,
}: {
  asks: AskItem[]
  current: number
  onPrev: () => void
  onNext: () => void
}) {
  return (
    <div className="rq-step" data-testid="agent-choice-position">
      <span className="rq-dots" aria-hidden="true">
        {asks.map((ask, i) => (
          <i key={ask.id} data-on={i === current || undefined} />
        ))}
      </span>
      <button type="button" aria-label="Previous request" onClick={onPrev}>
        <CaretLeftIcon size={14} />
      </button>
      <span className="rq-pos">
        {current + 1} of {asks.length}
      </span>
      <button type="button" aria-label="Next request" onClick={onNext}>
        <CaretRightIcon size={14} />
      </button>
    </div>
  )
}
