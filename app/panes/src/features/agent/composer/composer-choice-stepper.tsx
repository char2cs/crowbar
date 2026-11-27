import { CaretLeftIcon, CaretRightIcon } from '@phosphor-icons/react'

export interface StepPosition {
  index: number
  total: number
  onPrev: () => void
  onNext: () => void
  noun: 'request' | 'question'
}

export function ChoiceStepper({ index, total, onPrev, onNext, noun }: StepPosition) {
  return (
    <div className="rq-step" data-testid="agent-choice-position">
      <span className="rq-dots" aria-hidden="true">
        {Array.from({ length: total }, (_, i) => (
          // react-doctor-disable-next-line no-array-index-as-key -- dots are positional and never reorder
          <i key={i} data-on={i === index || undefined} data-done={i < index || undefined} />
        ))}
      </span>
      <button type="button" aria-label={`Previous ${noun}`} disabled={index === 0} onClick={onPrev}>
        <CaretLeftIcon size={14} />
      </button>
      <span className="rq-pos">
        <b>{index + 1}</b> of {total}
      </span>
      <button
        type="button"
        aria-label={`Next ${noun}`}
        disabled={index >= total - 1}
        onClick={onNext}
      >
        <CaretRightIcon size={14} />
      </button>
    </div>
  )
}

export function ChoicePeeks({ ahead }: { ahead: number }) {
  return (
    <>
      {ahead > 1 && <div className="rq-peek" data-far="" aria-hidden="true" />}
      {ahead > 0 && <div className="rq-peek" aria-hidden="true" />}
    </>
  )
}
