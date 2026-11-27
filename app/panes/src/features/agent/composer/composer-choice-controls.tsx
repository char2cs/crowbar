import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Radio, RadioGroup } from '@/components/ui/radio-group'
import type {
  AgentChoiceOption,
  AgentChoiceQuestion,
} from '@/features/agent/api/agent-chat-activity'
import { optionLabel } from '@/features/agent/composer/lib/ask-labels'

export function ChoiceQuestion({
  question,
  picked,
  busy,
  onPick,
}: {
  question: AgentChoiceQuestion
  picked: string[]
  busy: boolean
  onPick: (optionIds: string[]) => void
}) {
  const ticked = new Set(picked)
  const heading = question.text || question.title || 'Question'
  return (
    <div className="rq-question" data-testid="agent-choice-question" data-question-id={question.id}>
      <span className="rq-qh">
        {heading}
        {question.multi && <span className="rq-qmulti">pick any</span>}
      </span>
      {question.multi ? (
        <div className="rq-opts" data-testid="agent-choice-options">
          {question.options.map((option, place) => (
            <div key={option.id} className="rq-opt">
              <label>
                <Checkbox
                  checked={ticked.has(option.id)}
                  disabled={busy}
                  onChange={() =>
                    onPick(
                      ticked.has(option.id)
                        ? picked.filter((id) => id !== option.id)
                        : [...picked, option.id],
                    )
                  }
                />
                <span>{optionLabel(option)}</span>
                <OptionKey place={place} />
              </label>
              {option.description && <span className="rq-opt-desc">{option.description}</span>}
            </div>
          ))}
        </div>
      ) : (
        <RadioGroup
          className="rq-opts"
          data-testid="agent-choice-options"
          aria-label={heading}
          value={picked[0] ?? ''}
          onValueChange={(value) => onPick([String(value)])}
        >
          {question.options.map((option, place) => (
            <div key={option.id} className="rq-opt">
              <label>
                <Radio value={option.id} disabled={busy} />
                <span>{optionLabel(option)}</span>
                <OptionKey place={place} />
              </label>
              {option.description && <span className="rq-opt-desc">{option.description}</span>}
            </div>
          ))}
        </RadioGroup>
      )}
    </div>
  )
}

function OptionKey({ place }: { place: number }) {
  return place < 9 ? (
    <kbd className="choice-key" aria-hidden="true">
      {place + 1}
    </kbd>
  ) : null
}

const refusing = (option: AgentChoiceOption) => option.kind === 'deny' || option.kind === 'decline'

export function ChoiceButtons({
  options,
  keyed = false,
  busy,
  onPick,
}: {
  options: AgentChoiceOption[]
  keyed?: boolean
  busy: boolean
  onPick: (option: AgentChoiceOption) => void
}) {
  const hasGrant = options.some((option) => !refusing(option) && option.kind !== 'allow')
  const labelOf = (option: AgentChoiceOption) =>
    hasGrant && option.kind === 'allow' && (!option.label || option.label === 'Allow')
      ? 'Allow once'
      : optionLabel(option)
  const ordered = [
    ...options.filter(refusing),
    ...options.filter((option) => !refusing(option) && option.kind !== 'allow'),
    ...options.filter((option) => option.kind === 'allow'),
  ]
  return (
    <div className="rq-answers" data-testid="agent-choice-options">
      {ordered.map((option) => {
        const place = options.indexOf(option)
        const role = refusing(option) ? 'refuse' : option.kind === 'allow' ? 'primary' : 'grant'
        return (
          <Button
            key={option.id}
            data-role={role}
            variant={role === 'primary' ? 'default' : 'outline'}
            className={role === 'primary' ? undefined : 'bg-background'}
            disabled={busy}
            title={option.description || undefined}
            onClick={() => onPick(option)}
          >
            {labelOf(option)}
            {keyed && place < 9 && (
              <kbd className="choice-key" aria-hidden="true">
                {place + 1}
              </kbd>
            )}
          </Button>
        )
      })}
    </div>
  )
}

export function ChoiceReadOnly({
  options,
  questions,
}: {
  options: AgentChoiceOption[]
  questions: AgentChoiceQuestion[]
}) {
  const listed: AgentChoiceOption[] = [...options, ...questions.flatMap((q) => q.options)]
  if (listed.length === 0) return null
  return (
    <>
      <p className="rq-note">Answer this in the terminal. It offers:</p>
      <ul className="rq-readonly" data-testid="agent-choice-options-readonly">
        {listed.map((option) => (
          <li key={option.id}>
            {optionLabel(option)}
            {option.description ? ` — ${option.description}` : ''}
          </li>
        ))}
      </ul>
    </>
  )
}
