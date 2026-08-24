import { useState } from 'react'
import { TerminalIcon } from '@/features/agent/shared/agent-icons'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Radio, RadioGroup } from '@/components/ui/radio-group'
import {
  answerChoice,
  type AgentActivity,
  type AgentChoice,
  type AgentChoiceOption,
  type AgentChoiceQuestion,
} from '@/features/agent/api/agent-api'
import { choiceDetail, choiceQuestions, describeChoice } from '@/features/agent/lib/agent-activity'
import { ApiError } from '@/lib/api'

// The verbs an elicitation is answered with. They are the PROVIDER's own words
// (MCP's accept / decline / cancel) and they go in `optionIds` — a prompt with no
// options has no option kind to read, so the id itself is the decision key.
//
// `accept` is deliberately absent: accepting means handing back a filled-in form
// built from the provider's schema, and a form Crowbar cannot populate must not be
// dressed as a button that sends an empty one.
const ELICITATION_VERBS = [
  { id: 'decline', label: 'Decline' },
  { id: 'cancel', label: 'Cancel' },
] as const

type CardState = {
  choiceId: string
  /** Picks for a prompt whose controls are one flat row — a permission, an
   *  elicitation. */
  picked: string[]
  /** Picks for a question prompt, keyed by question id. */
  byQuestion: Record<string, string[]>
  sending: boolean
  sent: boolean
  error: string
  /** A 409 is the relay letting go between the poll that said "answerable" and the
   *  click. The server is right and this view is stale, so the controls come down
   *  at once instead of waiting up to a poll to stop lying. */
  gateClosed: boolean
}

function freshCard(choiceId: string): CardState {
  return {
    choiceId,
    picked: [],
    byQuestion: {},
    sending: false,
    sent: false,
    error: '',
    gateClosed: false,
  }
}

/**
 * One question the provider CLI is sitting on, answerable from the chat.
 *
 * The rules that shape it:
 *
 *  1. `pending` and `answerable` are different facts. Pending means the CLI is
 *     still asking; answerable means a relay is holding its gate open RIGHT NOW.
 *     A prompt that is pending and not answerable is still drawn — it is a real
 *     question, and hiding it is what made a blocked agent look frozen — but it
 *     offers no controls, because a button on it would silently reach nobody.
 *
 *  2. An answer sent from here is ADVISORY until the next poll says otherwise.
 *     Somebody at the terminal can decide the same prompt in the same instant, so
 *     nothing is removed locally: the card paints "sent" and disappears when the
 *     server reports it resolved, whichever way it actually ended.
 *
 *  3. A prompt is answered WHOLE or not at all. One AskUserQuestion can carry
 *     three questions, and answering one of them hands the CLI an input covering a
 *     third of what it asked — measured: claude then said "still waiting on your
 *     answers to questions 2 & 3", which nothing in the chat could ever send. So
 *     every question gets its own group and the send stays disabled until all of
 *     them have a pick.
 *
 *  4. What the backend refuses is SAID, never dressed as a control. A `suggestion`
 *     — claude's "and stop asking about this directory" — has no declared answer
 *     template, so it is written down as a note rather than drawn as a button that
 *     would fail.
 */
export function ComposerChoice({
  wsId,
  chatId,
  activity,
  choice,
  providerLabel,
  onOpenTerminal,
}: {
  wsId: string
  chatId: string
  /** The feed the second line is read off: a permission carries the tool's NAME
   *  but never its target, which is reported on the call it gates. */
  activity: AgentActivity
  choice: AgentChoice
  providerLabel: string
  onOpenTerminal?: () => void
}) {
  const detail = choiceDetail(activity, choice)
  const [stored, setStored] = useState<CardState>(() => freshCard(choice.id))
  // Read through the stamp rather than resetting in an effect: an effect runs
  // AFTER the paint, so there would be one frame in which the previous prompt's
  // picks were on screen and pressable.
  const card = stored.choiceId === choice.id ? stored : freshCard(choice.id)
  const update = (patch: Partial<CardState>) =>
    setStored((current) => ({
      ...(current.choiceId === choice.id ? current : freshCard(choice.id)),
      ...patch,
      choiceId: choice.id,
    }))

  const answerable = choice.answerable && !card.gateClosed
  // Suggestions are separated out rather than filtered away: what the provider
  // offered is worth showing even when Crowbar cannot send it.
  const suggestions = choice.options.filter((option) => option.kind === 'suggestion')
  const questions = choiceQuestions(choice)
  const isForm = choice.options.length === 0 && choice.kind === 'elicitation'
  // The flat row is what is left once questions are their own groups: a
  // permission's allow and deny, or an elicitation's verbs.
  const controls: AgentChoiceOption[] = isForm
    ? ELICITATION_VERBS.map((verb) => ({ id: verb.id, kind: verb.id, label: verb.label }))
    : choice.options.filter((option) => option.kind !== 'suggestion')
  // Answerable is necessary and not sufficient: a relay can be holding the gate
  // for a prompt whose only options are ones Crowbar declines to send, and an
  // empty row of buttons would be a control surface that answers nothing.
  const canAnswerHere = answerable && (questions.length > 0 || controls.length > 0)
  const busy = card.sending || card.sent

  const send = async (optionIds: string[]) => {
    update({ error: '', sending: true })
    try {
      await answerChoice(wsId, chatId, choice.id, { optionIds })
      // Deliberately NOT removed here. The card lives until the server says the
      // prompt stopped pending, so an answer that raced a terminal keystroke
      // resolves to whatever actually reached the CLI.
      update({ sent: true, sending: false })
    } catch (failure) {
      update({
        sending: false,
        error: answerErrorMessage(failure),
        gateClosed: failure instanceof ApiError && failure.status === 409,
      })
    }
  }

  // WHAT IS ASKED goes above the bar; the bar is the QUESTION and its controls.
  // Options are a list to read down, and folding them into a 38px row turns them
  // into a column squeezed against the sentence that introduces them.
  return (
    <div
      className="asks"
      role="group"
      aria-label={`${providerLabel} is waiting for your answer`}
      data-testid="agent-choice-prompt"
      data-choice-id={choice.id}
      data-choice-kind={choice.kind}
      data-answerable={String(answerable)}
    >
      {/* ── above the bar: everything there is to read ── */}
      {choice.schema && <ChoiceSchema schema={choice.schema} />}
      {canAnswerHere && questions.length > 0 && (
        <ChoiceQuestions
          questions={questions}
          picked={card.byQuestion}
          busy={busy}
          onPick={(questionId, optionIds) =>
            update({ byQuestion: { ...card.byQuestion, [questionId]: optionIds } })
          }
        />
      )}
      {canAnswerHere && suggestions.length > 0 && (
        <ChoiceSuggestions options={suggestions} onOpenTerminal={onOpenTerminal} />
      )}
      {canAnswerHere && isForm && (
        <p className="text-muted-foreground text-xs" data-testid="agent-choice-form-note">
          Filling this form in has to be done in the terminal — Crowbar cannot compose the answer it
          is asking for.
        </p>
      )}
      {!canAnswerHere && (
        <ChoiceReadOnly
          options={choice.options}
          questions={questions}
          note={
            answerable
              ? `Crowbar has no answer it can send for this one — ${providerLabel} is asking at its own terminal, and that is where it has to be decided.`
              : `Answer this in the terminal — ${providerLabel} is asking there, and nothing sent from here would reach it.`
          }
          onOpenTerminal={onOpenTerminal}
        />
      )}

      {/* ── the bar itself ── */}
      <div className="pill asking">
        <span className="q">
          {/* Not "working…". A chat waiting on a person is a different state from
              a chat doing work, and the two used to look identical. `alert` is the
              same role the interruption banner carries, for the same reason: this
              appears without the reader having asked for it, and it is the one
              thing on the screen that will not move until they act. */}
          <span role="alert">{describeChoice(choice)}</span>
          {detail && (
            <span className="sub" data-testid="agent-choice-detail">
              {detail}
            </span>
          )}
          {card.sent && (
            <span className="sub" data-testid="agent-choice-sent">
              Answer sent. Waiting for {providerLabel} to confirm it.
            </span>
          )}
        </span>
        {canAnswerHere && (
          <span className="acts">
            {questions.length > 0 ? (
              <Button
                size="sm"
                variant="secondary"
                disabled={
                  busy ||
                  !questions.every((question) => (card.byQuestion[question.id] ?? []).length > 0)
                }
                onClick={() => void send(flatPicks(questions, card.byQuestion))}
              >
                {questions.length > 1 ? 'Send answers' : 'Send answer'}
              </Button>
            ) : (
              <ChoiceButtons
                options={controls}
                busy={busy}
                onPick={(option) => void send([option.id])}
              />
            )}
          </span>
        )}
      </div>

      {card.error && (
        <p className="text-destructive text-xs" role="alert" data-testid="agent-choice-error">
          {card.error}
        </p>
      )}
    </div>
  )
}

/** Every question the prompt asks, and ONE send for all of them.
 *
 *  The send is a separate step and not a side effect of picking, because the
 *  provider is answered once: three questions produce one call carrying every
 *  pick. The send control lives in the BAR and stays disabled until every question
 *  has one, which is what makes a partial answer impossible from here — the shape
 *  that stranded a live agent. */
function ChoiceQuestions({
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
function ChoiceButtons({
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
 *  is how that failure stops being reachable from here at all. */
function ChoiceSuggestions({
  options,
  onOpenTerminal,
}: {
  options: AgentChoiceOption[]
  onOpenTerminal?: () => void
}) {
  return (
    <div className="flex flex-col gap-1" data-testid="agent-choice-suggestions">
      <p className="text-muted-foreground text-xs">
        {options.length > 1
          ? 'Crowbar cannot send the broader permissions this provider also offered:'
          : 'Crowbar cannot send the broader permission this provider also offered:'}
      </p>
      <ul className="flex flex-col gap-0.5 text-xs">
        {options.map((option) => (
          <li key={option.id} className="min-w-0 text-muted-foreground">
            {optionLabel(option)}
            {option.description ? ` — ${option.description}` : ''}
          </li>
        ))}
      </ul>
      <p className="text-muted-foreground text-xs">
        It declares no shape for them, and one narrowed to a plain allow would grant something else.
        The terminal can do it.
      </p>
      {onOpenTerminal && <TerminalLink onOpenTerminal={onOpenTerminal} />}
    </div>
  )
}

/** A prompt nobody here can answer: pending, but with no relay holding the gate.
 *
 *  It is still the whole question — what is being asked and what the answers are —
 *  because the CLI genuinely IS asking it. It just says where. */
function ChoiceReadOnly({
  options,
  questions,
  note,
  onOpenTerminal,
}: {
  options: AgentChoiceOption[]
  questions: AgentChoiceQuestion[]
  note: string
  onOpenTerminal?: () => void
}) {
  // Every option the prompt holds, wherever it lives, because the reader is being
  // shown what the CLI is asking rather than anything they can act on.
  const listed: AgentChoiceOption[] = [...options, ...questions.flatMap((q) => q.options)]
  return (
    <div className="flex flex-col gap-1">
      {listed.length > 0 && (
        <ul className="flex flex-col gap-0.5 text-xs" data-testid="agent-choice-options-readonly">
          {listed.map((option) => (
            <li key={option.id} className="truncate text-muted-foreground">
              {optionLabel(option)}
              {option.description ? ` — ${option.description}` : ''}
            </li>
          ))}
        </ul>
      )}
      <p className="text-xs" data-testid="agent-choice-terminal-note">
        {note}
      </p>
      {onOpenTerminal && <TerminalLink onOpenTerminal={onOpenTerminal} />}
    </div>
  )
}

function TerminalLink({ onOpenTerminal }: { onOpenTerminal: () => void }) {
  return (
    <Button className="self-start" size="xs" variant="ghost" onClick={onOpenTerminal}>
      <TerminalIcon /> Open the terminal
    </Button>
  )
}

/** The form an elicitation is asking for, as the provider described it.
 *
 *  Shown VERBATIM (pretty-printed where it parses) and never interpreted into
 *  fields: reading a JSON Schema well enough to render a form is a job this
 *  surface has not done, and half-reading one would ask for the wrong things. */
function ChoiceSchema({ schema }: { schema: string }) {
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

/** The picks of every question, flattened into the ONE list the answer endpoint
 *  takes — the option ids say which question each belongs to, so the prompt is
 *  answered in a single call however many questions it asked.
 *
 *  Driven off the QUESTIONS rather than off the pick map, so an id left behind by
 *  a question that is no longer on the prompt can never be sent. */
function flatPicks(questions: AgentChoiceQuestion[], picked: Record<string, string[]>): string[] {
  return questions.flatMap((question) => picked[question.id] ?? [])
}

/** An option with no label is named by its kind — Crowbar's own word for it — so a
 *  provider that labels nothing still gets a legible control rather than a blank. */
function optionLabel(option: AgentChoiceOption): string {
  if (option.label) return option.label
  return option.kind.charAt(0).toUpperCase() + option.kind.slice(1)
}

/**
 * Say what the server said, in the user's terms.
 *
 * 409 is the one that matters most: the prompt is no longer answerable from here,
 * which is an ordinary outcome (somebody typed at the terminal, or the relay's
 * budget ran out) and not a bug to hide behind a retry.
 */
function answerErrorMessage(failure: unknown): string {
  if (failure instanceof ApiError) {
    if (failure.status === 409) {
      return 'This can no longer be answered from Crowbar — answer it in the terminal.'
    }
    if (failure.status === 400) {
      return `This provider cannot be answered that way from here: ${failure.message}`
    }
  }
  return failure instanceof Error ? failure.message : String(failure)
}
