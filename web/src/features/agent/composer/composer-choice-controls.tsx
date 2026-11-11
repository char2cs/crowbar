import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Radio, RadioGroup } from '@/components/ui/radio-group'
import type {
  AgentChoiceOption,
  AgentChoiceQuestion,
} from '@/features/agent/api/agent-chat-activity'
import { optionLabel } from '@/features/agent/composer/lib/ask-labels'

/** Every question the prompt asks. Picking does not send: the provider is answered once, by the
 *  bar's send, which stays disabled until every question has a pick (a partial answer strands
 *  the agent). */
export function ChoiceQuestions({
  questions,
  picked,
  busy,
  onPick,
}: {
  questions: AgentChoiceQuestion[]
  picked: Record<string, string[]>
  busy: boolean
  onPick: (questionId: string, optionIds: string[]) => void
}) {
  return (
    <div className="flex flex-col gap-3" data-testid="agent-choice-options">
      {questions.map((question, index) => (
        <ChoiceQuestionGroup
          key={question.id}
          question={question}
          // The number is drawn only when there is more than one, because "1." over
          // a single question is a list that is not a list.
          ordinal={questions.length > 1 ? index + 1 : 0}
          picked={picked[question.id] ?? []}
          busy={busy}
          onPick={(optionIds) => onPick(question.id, optionIds)}
        />
      ))}
    </div>
  )
}

/** Radios for a single-answer question, checkboxes for multi-select. Per question, not per
 *  prompt, so one card can hold both. */
function ChoiceQuestionGroup({
  question,
  ordinal,
  picked,
  busy,
  onPick,
}: {
  question: AgentChoiceQuestion
  ordinal: number
  picked: string[]
  busy: boolean
  onPick: (optionIds: string[]) => void
}) {
  const heading = question.text || question.title || `Question ${ordinal || 1}`
  return (
    <div
      className="flex min-w-0 flex-col gap-1"
      data-testid="agent-choice-question"
      data-question-id={question.id}
    >
      <span className="font-medium text-xs">
        {ordinal > 0 ? `${ordinal}. ` : ''}
        {heading}
        {question.multi && <span className="ml-1 text-muted-foreground">(pick any)</span>}
      </span>
      {question.multi ? (
        <ChoiceCheckboxes
          options={question.options}
          picked={picked}
          busy={busy}
          onToggle={(option) =>
            onPick(
              picked.includes(option.id)
                ? picked.filter((id) => id !== option.id)
                : [...picked, option.id],
            )
          }
        />
      ) : (
        <ChoiceRadios
          options={question.options}
          label={heading}
          picked={picked[0] ?? ''}
          busy={busy}
          onPick={(optionId) => onPick([optionId])}
        />
      )}
    </div>
  )
}

/** A single-answer question. Picking does NOT send: the prompt may hold other
 *  questions that still need answers, and a send per question would hand the CLI
 *  the partial input this card exists to prevent. */
function ChoiceRadios({
  options,
  label,
  picked,
  busy,
  onPick,
}: {
  options: AgentChoiceOption[]
  label: string
  picked: string
  busy: boolean
  onPick: (optionId: string) => void
}) {
  return (
    <RadioGroup
      className="gap-1"
      aria-label={label}
      value={picked}
      onValueChange={(value) => onPick(String(value))}
    >
      {options.map((option) => (
        // The description sits OUTSIDE the label on purpose: the wrapping label is
        // what names the control, and folding a sentence of explanation into that
        // name is what a screen reader would read out on every tab.
        <div key={option.id} className="flex min-w-0 flex-col">
          <label className="flex items-center gap-2">
            <Radio value={option.id} disabled={busy} />
            <span>{optionLabel(option)}</span>
          </label>
          {option.description && (
            <span className="ml-6 text-muted-foreground text-xs">{option.description}</span>
          )}
        </div>
      ))}
    </RadioGroup>
  )
}

/** A question that takes several answers. Every ticked id travels in the one call
 *  the prompt is answered with. */
function ChoiceCheckboxes({
  options,
  picked,
  busy,
  onToggle,
}: {
  options: AgentChoiceOption[]
  picked: string[]
  busy: boolean
  onToggle: (option: AgentChoiceOption) => void
}) {
  const ticked = new Set(picked)
  return (
    <div className="flex flex-col gap-1">
      {options.map((option) => (
        <div key={option.id} className="flex min-w-0 flex-col">
          <label className="flex items-center gap-2">
            <Checkbox
              checked={ticked.has(option.id)}
              onChange={() => onToggle(option)}
              disabled={busy}
            />
            <span>{optionLabel(option)}</span>
          </label>
          {option.description && (
            <span className="ml-6 text-muted-foreground text-xs">{option.description}</span>
          )}
        </div>
      ))}
    </div>
  )
}

/** One button per option — the case where choosing IS answering, because there is
 *  nothing else on the prompt to answer. A permission's allow and deny, an
 *  elicitation's verbs. */
export function ChoiceButtons({
  options,
  keyed = false,
  busy,
  onPick,
}: {
  options: AgentChoiceOption[]
  /** The card answers digit keys: each button shows its digit. */
  keyed?: boolean
  busy: boolean
  onPick: (option: AgentChoiceOption) => void
}) {
  return (
    <div className="flex flex-wrap gap-2" data-testid="agent-choice-options">
      {options.map((option, i) => (
        <Button
          key={option.id}
          size="sm"
          variant={option.kind === 'deny' || option.kind === 'decline' ? 'outline' : 'secondary'}
          disabled={busy}
          title={option.description || undefined}
          onClick={() => onPick(option)}
        >
          {optionLabel(option)}
          {keyed && i < 9 && (
            <kbd className="choice-key" aria-hidden="true">
              {i + 1}
            </kbd>
          )}
        </Button>
      ))}
    </div>
  )
}

/** A prompt pending with no relay holding its gate, still shown whole because the CLI is
 *  really asking it. The bar offers the Terminal to answer it. */
export function ChoiceReadOnly({
  options,
  questions,
}: {
  options: AgentChoiceOption[]
  questions: AgentChoiceQuestion[]
}) {
  // Every option the prompt holds, wherever it lives, because the reader is being
  // shown what the CLI is asking rather than anything they can act on.
  const listed: AgentChoiceOption[] = [...options, ...questions.flatMap((q) => q.options)]
  if (listed.length === 0) return null
  return (
    <ul className="flex flex-col gap-0.5 text-xs" data-testid="agent-choice-options-readonly">
      {listed.map((option) => (
        <li key={option.id} className="truncate text-muted-foreground">
          {optionLabel(option)}
          {option.description ? ` — ${option.description}` : ''}
        </li>
      ))}
    </ul>
  )
}

/** The elicitation's schema verbatim (pretty-printed where it parses), never rendered as a
 *  form: half-reading a JSON Schema would ask for the wrong things. */
export function ChoiceSchema({ schema }: { schema: string }) {
  return (
    <details className="text-xs" data-testid="agent-choice-schema">
      <summary className="cursor-pointer text-muted-foreground">What it is asking for</summary>
      <pre className="mt-1 max-h-40 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted/50 p-2 font-mono">
        {prettySchema(schema)}
      </pre>
    </details>
  )
}

function prettySchema(schema: string): string {
  try {
    return JSON.stringify(JSON.parse(schema), null, 2)
  } catch {
    // Not parseable here is not a failure to report: it is the provider's bytes,
    // and they are shown as they arrived.
    return schema
  }
}
