import { useState, type KeyboardEvent } from 'react'
import { TerminalIcon } from '@/features/agent/shared/agent-icons'
import { Button } from '@/components/ui/button'
import { answerChoice } from '@/features/agent/api/agent-chat-commands'
import type {
  AgentChoice,
  AgentChoiceOption,
  AgentChoiceQuestion,
} from '@/features/agent/api/agent-chat-activity'
import type { PermissionLevel } from '@/features/agent/api/agent-providers-api'
import {
  choiceDetail,
  choiceQuestions,
  describeChoice,
} from '@/features/agent/composer/lib/ask-labels'
import { PermissionLevelSwitcher } from '@/features/agent/composer/permission-level-switcher'
import { ApiError } from '@/lib/errors/api-error'
import {
  ChoiceQuestions,
  ChoiceButtons,
  ChoiceSuggestions,
  ChoiceReadOnly,
  ChoiceSchema,
} from '@/features/agent/composer/composer-choice-controls'
import { ChoiceRequest } from '@/features/agent/composer/composer-choice-request'
import {
  requestView,
  SHOWN_LINES,
  type AskRequest,
} from '@/features/agent/composer/lib/ask-request'

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
  /** A 400: this provider's relay does not accept THIS shape of answer for this
   *  prompt. Retrying the same controls would only produce the same 400 — the
   *  terminal is the one place left that can still answer it. */
  unsupportedHere: boolean
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
    unsupportedHere: false,
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
 *
 *  5. Nothing here explains WHY Crowbar can't act — a permission card is not an
 *     incident report. Whatever the reason (a suggestion with no answer template,
 *     a form only the terminal can fill, a gate that closed, a shape the relay
 *     just 400'd on), the bar offers the one place that still works: a Terminal
 *     button in the same row as its other controls, not a paragraph about it.
 */
export function ComposerChoice({
  wsId,
  chatId,
  pending,
  choice,
  toolTarget,
  providerLabel,
  permissionLevels,
  onOpenTerminal,
  position = 1,
  request,
}: {
  wsId: string
  chatId: string
  /** How many asks are pending, this one included. */
  pending: number
  choice: AgentChoice
  /** What a permission is about to touch, from the call it gates. */
  toolTarget?: string
  providerLabel: string
  /** This chat's current provider's own declared levels — see
   *  PermissionLevelSwitcher's own doc for why an absent/empty list hides it. */
  permissionLevels?: PermissionLevel[]
  onOpenTerminal?: () => void
  /** Where this ask stands among the pending ones, from 1. */
  position?: number
  /** What a permission would let run, from the ask's own copy of the call. */
  request?: AskRequest
}) {
  const detail = choiceDetail(choice, toolTarget)
  const [stored, setStored] = useState<CardState>(() => freshCard(choice.id))
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const expanded = expandedId === choice.id
  const view = request && choice.kind === 'tool_permission' ? requestView(request) : null
  const hiddenLines = view && !expanded ? Math.max(0, view.code.length - SHOWN_LINES) : 0
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
  // Unanswerable for any reason, or answerable but just 400'd on the shape sent —
  // either way the normal controls would only repeat the same dead end, so the
  // bar shows nothing but the one thing that still works.
  const terminalOnly = !canAnswerHere || card.unsupportedHere
  // Answerable, but with something the terminal covers and this card cannot: a
  // form Crowbar can't compose, or suggestions with no declared answer shape.
  const needsTerminal = isForm || suggestions.length > 0
  const busy = card.sending || card.sent
  const flat = canAnswerHere && !card.unsupportedHere && questions.length === 0

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
        unsupportedHere: failure instanceof ApiError && failure.status === 400,
      })
    }
  }

  // A digit picks that option; Enter allows once, only from the card itself so
  // an Enter meant for the composer never grants a permission.
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!flat || busy || event.metaKey || event.ctrlKey || event.altKey) return
    const picked = keyedOption(controls, event.key, event.target === event.currentTarget)
    if (!picked) return
    event.preventDefault()
    void send([picked.id])
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
      tabIndex={0}
      onKeyDown={onKeyDown}
    >
      {/* ── above the bar: everything there is to read ── */}
      {choice.schema && <ChoiceSchema schema={choice.schema} />}
      {view && (
        <ChoiceRequest view={view} expanded={expanded} onExpand={() => setExpandedId(choice.id)} />
      )}
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
      {canAnswerHere && suggestions.length > 0 && <ChoiceSuggestions options={suggestions} />}
      {!canAnswerHere && <ChoiceReadOnly options={choice.options} questions={questions} />}

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
          {hiddenLines > 0 && (
            <span className="sub choice-hidden-lines" data-testid="agent-choice-hidden-lines">
              {hiddenLines} more lines not shown
            </span>
          )}
          {pending > 1 && (
            <span className="sub" data-testid="agent-choice-position">
              {position}/{pending}
            </span>
          )}
        </span>
        {(onOpenTerminal || !terminalOnly) && (
          <span className="acts">
            {terminalOnly ? (
              onOpenTerminal && <TerminalLink onOpenTerminal={onOpenTerminal} />
            ) : (
              <>
                {needsTerminal && onOpenTerminal && (
                  <TerminalLink onOpenTerminal={onOpenTerminal} />
                )}
                {questions.length > 0 ? (
                  <Button
                    size="sm"
                    variant="secondary"
                    disabled={
                      busy ||
                      !questions.every(
                        (question) => (card.byQuestion[question.id] ?? []).length > 0,
                      )
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
                <PermissionLevelSwitcher
                  wsId={wsId}
                  chatId={chatId}
                  availableLevels={permissionLevels}
                />
              </>
            )}
          </span>
        )}
      </div>

      {card.error && !terminalOnly && (
        <p className="text-destructive text-xs" role="alert" data-testid="agent-choice-error">
          {card.error}
        </p>
      )}
    </div>
  )
}

/** The option a key names: a digit its place, Enter (from the card) allowing once. */
function keyedOption(controls: AgentChoiceOption[], key: string, onCard: boolean) {
  if (key === 'Enter')
    return onCard ? controls.find((option) => option.kind === 'allow') : undefined
  return /^[1-9]$/.test(key) ? controls[Number(key) - 1] : undefined
}

function TerminalLink({ onOpenTerminal }: { onOpenTerminal: () => void }) {
  return (
    <Button size="sm" variant="outline" onClick={onOpenTerminal}>
      <TerminalIcon /> Terminal
    </Button>
  )
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

/**
 * Say what the server said, in the user's terms.
 *
 * A 409 (the gate closed) or a 400 (the shape isn't one this relay accepts) are
 * ordinary outcomes, not bugs to explain — `gateClosed`/`unsupportedHere` already
 * turn those into a Terminal button instead of a retry, so this text is never
 * shown for either. What's left here is whatever else actually went wrong.
 */
function answerErrorMessage(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure)
}
