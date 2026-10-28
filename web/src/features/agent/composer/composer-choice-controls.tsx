import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Radio, RadioGroup } from '@/components/ui/radio-group'
import type {
  AgentChoiceOption,
  AgentChoiceQuestion,
} from '@/features/agent/api/agent-chat-activity'
import { optionLabel } from '@/features/agent/lib/agent-activity'

/** Every question the prompt asks, and ONE send for all of them.
 *
 *  The send is a separate step and not a side effect of picking, because the
 *  provider is answered once: three questions produce one call carrying every
 *  pick. The send control lives in the BAR and stays disabled until every question
 *  has one, which is what makes a partial answer impossible from here — the shape
 *  that stranded a live agent. */
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

/** One question's own controls: radios when it takes a single answer, checkboxes
 *  when it takes several.
 *
 *  Which of the two is the QUESTION's own property, not the prompt's — claude
 *  carries multiSelect per question — so a card can legitimately hold both at
 *  once, and the control has to say which one this is. */
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
  busy,
  onPick,
}: {
  options: AgentChoiceOption[]
  busy: boolean
  onPick: (option: AgentChoiceOption) => void
}) {
  return (
    <div className="flex flex-wrap gap-2" data-testid="agent-choice-options">
      {options.map((option) => (
        <Button
          key={option.id}
          size="sm"
          variant={option.kind === 'deny' || option.kind === 'decline' ? 'outline' : 'secondary'}
          disabled={busy}
          title={option.description || undefined}
          onClick={() => onPick(option)}
        >
          {optionLabel(option)}
        </Button>
      ))}
    </div>
  )
}

/** What the provider offered but Crowbar cannot send.
 *
 *  Written down, and written down as PROSE. It used to be a row of disabled
 *  buttons, which still reads as a control — something greyed out now that will
 *  work in a moment — and the labels on them were claude's own internal type names
 *  (`addRules`), so it read as a real choice spelled in a language nobody outside
 *  the CLI's source uses. There is no declared answer template for a suggestion,
 *  so pressing one could only ever produce a 400; a note cannot be pressed, which
 *  is how that failure stops being reachable from here at all. Why Crowbar can't
 *  send them isn't said here either — the Terminal button in the bar below is the
 *  answer to that, for this reason and every other one. */
export function ChoiceSuggestions({ options }: { options: AgentChoiceOption[] }) {
  return (
    <ul className="flex flex-col gap-0.5 text-xs" data-testid="agent-choice-suggestions">
      {options.map((option) => (
        <li key={option.id} className="min-w-0 text-muted-foreground">
          {optionLabel(option)}
          {option.description ? ` — ${option.description}` : ''}
        </li>
      ))}
    </ul>
  )
}

/** A prompt nobody here can answer: pending, but with no relay holding the gate,
 *  or a relay that just closed one.
 *
 *  It is still the whole question — what is being asked and what the answers are —
 *  because the CLI genuinely IS asking it. Where to answer it is the bar's job
 *  below, not this list's: it draws the Terminal button whenever there's nothing
 *  else it can offer. */
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

/** The form an elicitation is asking for, as the provider described it.
 *
 *  Shown VERBATIM (pretty-printed where it parses) and never interpreted into
 *  fields: reading a JSON Schema well enough to render a form is a job this
 *  surface has not done, and half-reading one would ask for the wrong things. */
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
