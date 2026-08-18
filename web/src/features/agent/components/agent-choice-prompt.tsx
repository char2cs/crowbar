import { useState } from 'react'
import { TerminalIcon } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  answerChoice,
  type AgentActivity,
  type AgentChoice,
  type AgentChoiceOption,
} from '@/features/agent/api/agent-api'
import { choiceDetail, describeChoice, pendingChoices } from '@/features/agent/lib/agent-activity'
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

/**
 * The prompts a chat is blocked on, drawn where the "working…" line would be.
 *
 * Every pending prompt is drawn — not just the newest. Each one is separately
 * holding a CLI gate open, and collapsing them would leave the provider blocked on
 * a question nobody was ever shown.
 */
export function AgentChoicePrompts({
  wsId,
  chatId,
  activity,
  providerLabel,
  onOpenTerminal,
}: {
  wsId: string
  chatId: string
  activity: AgentActivity
  providerLabel: string
  onOpenTerminal?: () => void
}) {
  const prompts = pendingChoices(activity)
  if (prompts.length === 0) return null

  return (
    <div className="flex flex-col gap-2" data-testid="agent-choice-prompts">
      {prompts.map((choice) => (
        <AgentChoicePrompt
          key={choice.id}
          wsId={wsId}
          chatId={chatId}
          choice={choice}
          detail={choiceDetail(activity, choice)}
          providerLabel={providerLabel}
          onOpenTerminal={onOpenTerminal}
        />
      ))}
    </div>
  )
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
 *  3. What the backend refuses is SAID, never worked around. A `suggestion`
 *     option — claude's "and stop asking about this directory" — has no declared
 *     answer template, so it is drawn as the provider offered it and marked
 *     unsendable rather than quietly narrowed to a plain allow.
 */
export function AgentChoicePrompt({
  wsId,
  chatId,
  choice,
  detail,
  providerLabel,
  onOpenTerminal,
}: {
  wsId: string
  chatId: string
  choice: AgentChoice
  /** The second line: a permission's target, or a title the headline did not use. */
  detail: string
  providerLabel: string
  onOpenTerminal?: () => void
}) {
  const [picked, setPicked] = useState<string[]>([])
  const [sending, setSending] = useState(false)
  const [sent, setSent] = useState(false)
  const [error, setError] = useState('')
  // A 409 is the relay letting go between the poll that said "answerable" and the
  // click. The server is right and this view is stale, so the controls come down
  // at once instead of waiting up to a poll to stop lying.
  const [gateClosed, setGateClosed] = useState(false)

  const answerable = choice.answerable && !gateClosed
  // Suggestions are separated out rather than filtered away: what the provider
  // offered is worth showing even when Crowbar cannot send it.
  const suggestions = choice.options.filter((option) => option.kind === 'suggestion')
  const offered = choice.options.filter((option) => option.kind !== 'suggestion')
  const isForm = choice.options.length === 0 && choice.kind === 'elicitation'
  const controls: AgentChoiceOption[] = isForm
    ? ELICITATION_VERBS.map((verb) => ({ id: verb.id, kind: verb.id, label: verb.label }))
    : offered
  // Answerable is necessary and not sufficient: a relay can be holding the gate
  // for a prompt whose only options are ones Crowbar declines to send, and an
  // empty row of buttons would be a control surface that answers nothing.
  const canAnswerHere = answerable && controls.length > 0

  const send = async (optionIds: string[]) => {
    setError('')
    setSending(true)
    try {
      await answerChoice(wsId, chatId, choice.id, { optionIds })
      // Deliberately NOT removed here. The card lives until the server says the
      // prompt stopped pending, so an answer that raced a terminal keystroke
      // resolves to whatever actually reached the CLI.
      setSent(true)
    } catch (failure) {
      if (failure instanceof ApiError && failure.status === 409) setGateClosed(true)
      setError(answerErrorMessage(failure))
    } finally {
      setSending(false)
    }
  }

  const toggle = (option: AgentChoiceOption) =>
    setPicked((current) => {
      // Two picks of different kinds are not one answer — "allow" and "deny"
      // together mean nothing, and the backend refuses them — so a pick of a new
      // kind starts a new answer rather than building an invalid one.
      const kept = current.filter(
        (id) => choice.options.find((candidate) => candidate.id === id)?.kind === option.kind,
      )
      return kept.includes(option.id) ? kept.filter((id) => id !== option.id) : [...kept, option.id]
    })

  return (
    <div
      className="flex flex-col gap-2 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-sm"
      role="group"
      aria-label={`${providerLabel} is waiting for your answer`}
      data-testid="agent-choice-prompt"
      data-choice-id={choice.id}
      data-choice-kind={choice.kind}
      data-answerable={String(answerable)}
    >
      <div className="flex flex-col gap-0.5">
        {/* Not "working…". A chat waiting on a person is a different state from a
            chat doing work, and the two used to look identical. `alert` is the
            same role the interruption banner carries, for the same reason: this
            appears without the reader having asked for it, and it is the one
            thing on the screen that will not move until they act. */}
        <span className="text-warning-foreground text-xs" role="alert">
          {providerLabel} is waiting for your answer
        </span>
        <span className="font-medium">{describeChoice(choice)}</span>
        {detail && (
          <span
            className="truncate text-muted-foreground text-xs"
            data-testid="agent-choice-detail"
          >
            {detail}
          </span>
        )}
      </div>

      {choice.schema && <ChoiceSchema schema={choice.schema} />}

      {canAnswerHere ? (
        <>
          {choice.multi && !isForm ? (
            <ChoiceCheckboxes
              options={controls}
              picked={picked}
              busy={sending || sent}
              onToggle={toggle}
              onSend={() => void send(picked)}
            />
          ) : (
            <ChoiceButtons
              options={controls}
              busy={sending || sent}
              onPick={(option) => void send([option.id])}
            />
          )}
          {suggestions.length > 0 && (
            <ChoiceSuggestions options={suggestions} onOpenTerminal={onOpenTerminal} />
          )}
          {isForm && (
            <p className="text-muted-foreground text-xs" data-testid="agent-choice-form-note">
              Filling this form in has to be done in the terminal — Crowbar cannot compose the
              answer it is asking for.
            </p>
          )}
          {sent && (
            <p className="text-muted-foreground text-xs" data-testid="agent-choice-sent">
              Answer sent. Waiting for {providerLabel} to confirm it.
            </p>
          )}
        </>
      ) : (
        <ChoiceReadOnly
          options={choice.options}
          note={
            answerable
              ? `Crowbar has no answer it can send for this one — ${providerLabel} is asking at its own terminal, and that is where it has to be decided.`
              : `Answer this in the terminal — ${providerLabel} is asking there, and nothing sent from here would reach it.`
          }
          onOpenTerminal={onOpenTerminal}
        />
      )}

      {error && (
        <p className="text-destructive text-xs" role="alert" data-testid="agent-choice-error">
          {error}
        </p>
      )}
    </div>
  )
}

/** One button per option — the single-pick case, where choosing IS answering. */
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

/** The multi-select case: several ids travel in ONE answer, so the send is its own
 *  step rather than a side effect of ticking a box. */
function ChoiceCheckboxes({
  options,
  picked,
  busy,
  onToggle,
  onSend,
}: {
  options: AgentChoiceOption[]
  picked: string[]
  busy: boolean
  onToggle: (option: AgentChoiceOption) => void
  onSend: () => void
}) {
  const ticked = new Set(picked)
  return (
    <div className="flex flex-col gap-2" data-testid="agent-choice-options">
      {options.map((option) => (
        // The description sits OUTSIDE the label on purpose: a wrapping label is
        // what names the checkbox, and folding a sentence of explanation into that
        // name is what a screen reader would then have to read out on every tab.
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
      <Button
        className="self-start"
        size="sm"
        variant="secondary"
        disabled={busy || picked.length === 0}
        onClick={onSend}
      >
        Send answer
      </Button>
    </div>
  )
}

/** What the provider offered but Crowbar cannot send.
 *
 *  Drawn, and drawn as unsendable. The backend declares no answer template for a
 *  suggestion — claude's broader grants ride a field that was never measured — and
 *  a button that quietly narrowed one to a plain allow would grant something other
 *  than what it said. */
function ChoiceSuggestions({
  options,
  onOpenTerminal,
}: {
  options: AgentChoiceOption[]
  onOpenTerminal?: () => void
}) {
  return (
    <div className="flex flex-col gap-1" data-testid="agent-choice-suggestions">
      <div className="flex flex-wrap gap-2">
        {options.map((option) => (
          <Button
            key={option.id}
            size="sm"
            variant="outline"
            disabled
            title="Crowbar cannot send this one."
          >
            {optionLabel(option)}
          </Button>
        ))}
      </div>
      <p className="text-muted-foreground text-xs">
        Crowbar cannot send the broader grants this provider offered — it declares no shape for
        them, and one that narrowed to a plain allow would grant something else. Use the terminal
        for those.
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
  note,
  onOpenTerminal,
}: {
  options: AgentChoiceOption[]
  note: string
  onOpenTerminal?: () => void
}) {
  return (
    <div className="flex flex-col gap-1">
      {options.length > 0 && (
        <ul className="flex flex-col gap-0.5 text-xs" data-testid="agent-choice-options-readonly">
          {options.map((option) => (
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

/** An option with no label is named by its kind — Crowbar's own word for it — so a
 *  provider that labels nothing still gets a legible button rather than a blank. */
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
