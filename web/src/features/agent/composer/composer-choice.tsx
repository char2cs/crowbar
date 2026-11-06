import { useEffect, useEffectEvent, useState, type KeyboardEvent } from 'react'
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
import { actionErrorMessage } from '@/features/hosts/lib/host-failure-notice'
import {
  ChoiceQuestions,
  ChoiceButtons,
  ChoiceReadOnly,
  ChoiceSchema,
} from '@/features/agent/composer/composer-choice-controls'
import { ChoiceRequest } from '@/features/agent/composer/composer-choice-request'
import {
  foldCode,
  foldedCue,
  requestView,
  type AskRequest,
} from '@/features/agent/composer/lib/ask-request'

// MCP's own elicitation verbs, sent as `optionIds` since such a prompt has no options to read.
// No `accept`: it must carry a filled-in form Crowbar can't build, so a button would send it empty.
const ELICITATION_VERBS = [
  { id: 'decline', label: 'Decline' },
  { id: 'cancel', label: 'Cancel' },
] as const

type CardState = {
  choiceId: string
  /** Picks for a prompt whose controls are one flat row (a permission, an elicitation). */
  picked: string[]
  /** Picks for a question prompt, keyed by question id. */
  byQuestion: Record<string, string[]>
  sending: boolean
  sent: boolean
  error: string
  /** A 409: the relay let go after the poll said "answerable". The view is stale, so the
   *  controls come down now rather than at the next poll. */
  gateClosed: boolean
  /** A 400: the relay refuses this shape of answer, so retrying cannot help; only the
   *  terminal can answer it. */
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

// A question the provider CLI is waiting on. Pending-but-not-answerable is drawn without
// controls; a sent answer stays "sent" until the server resolves it (the terminal may race);
// every question needs a pick before send; terminal-only answers get a Terminal button.
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
  const hiddenCue = view && !expanded ? foldedCue(foldCode(view.code)) : ''
  // Read through the stamp rather than resetting in an effect: an effect runs AFTER the paint, so
  // there would be one frame in which the previous prompt's picks were on screen and pressable.
  const card = stored.choiceId === choice.id ? stored : freshCard(choice.id)
  const update = (patch: Partial<CardState>) =>
    setStored((current) => ({
      ...(current.choiceId === choice.id ? current : freshCard(choice.id)),
      ...patch,
      choiceId: choice.id,
    }))

  const answerable = choice.answerable && !card.gateClosed
  const questions = choiceQuestions(choice)
  const isForm = choice.options.length === 0 && choice.kind === 'elicitation'
  // The flat row is what is left once questions are their own groups: a
  // permission's allow, deny and broader grants, or an elicitation's verbs.
  const controls: AgentChoiceOption[] = isForm
    ? ELICITATION_VERBS.map((verb) => ({ id: verb.id, kind: verb.id, label: verb.label }))
    : choice.options
  // Answerable is necessary and not sufficient: a relay can be holding the gate
  // for a prompt whose only options are ones Crowbar declines to send, and an
  // empty row of buttons would be a control surface that answers nothing.
  const canAnswerHere = answerable && (questions.length > 0 || controls.length > 0)
  // Unanswerable for any reason, or answerable but just 400'd on the shape sent —
  // either way the normal controls would only repeat the same dead end, so the
  // bar shows nothing but the one thing that still works.
  const terminalOnly = !canAnswerHere || card.unsupportedHere
  // Answerable, but with something the terminal covers and this card cannot: a
  // form Crowbar can't compose.
  const needsTerminal = isForm
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
        // A 409 or 400 shows as a Terminal button (gateClosed/unsupportedHere), never as this text.
        error: actionErrorMessage(failure, String(failure)),
        gateClosed: failure instanceof ApiError && failure.status === 409,
        unsupportedHere: failure instanceof ApiError && failure.status === 400,
      })
    }
  }

  // A digit picks that option; Enter allows once, only from the card itself so
  // an Enter meant for the composer never grants a permission.
  const pickByKey = (event: globalThis.KeyboardEvent, enter: boolean) => {
    if (event.repeat || event.isComposing || !flat || busy) return
    if (event.metaKey || event.ctrlKey || event.altKey || (!enter && event.key === 'Enter')) return
    const picked = keyedOption(controls, event.key)
    if (!picked) return
    event.preventDefault()
    void send([picked.id])
  }
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    // Only from the card itself: a key on a control inside it is that control's.
    if (event.target === event.currentTarget) pickByKey(event.nativeEvent, true)
  }
  // The composer gives way to the card and focus falls to the page: a digit typed
  // then is for the card. Not an Enter, which may be the end of a message.
  const onPageKey = useEffectEvent((event: globalThis.KeyboardEvent) => {
    if (event.target === document.body && document.activeElement === document.body)
      pickByKey(event, false)
  })
  useEffect(() => {
    window.addEventListener('keydown', onPageKey)
    return () => window.removeEventListener('keydown', onPageKey)
  }, [])

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
      {!canAnswerHere && <ChoiceReadOnly options={choice.options} questions={questions} />}

      {/* ── the bar itself ── */}
      <div className="pill asking">
        <span className="q">
          {/* Not "working…": waiting on a person is its own state. `alert`, like the
              interruption banner: it appears unasked and will not move until they act. */}
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
          {hiddenCue && (
            <span className="sub choice-hidden-lines" data-testid="agent-choice-hidden-lines">
              {hiddenCue}
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
                    keyed={flat}
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

/** The option a key names: a digit its place, Enter allowing once. */
function keyedOption(controls: AgentChoiceOption[], key: string) {
  if (key === 'Enter') return controls.find((option) => option.kind === 'allow')
  return /^[1-9]$/.test(key) ? controls[Number(key) - 1] : undefined
}

function TerminalLink({ onOpenTerminal }: { onOpenTerminal: () => void }) {
  return (
    <Button size="sm" variant="outline" onClick={onOpenTerminal}>
      <TerminalIcon /> Terminal
    </Button>
  )
}

/** Every question's picks as the one list the answer endpoint takes. Read off the questions,
 *  not the pick map, so a pick for a question gone from the prompt is never sent. */
function flatPicks(questions: AgentChoiceQuestion[], picked: Record<string, string[]>): string[] {
  return questions.flatMap((question) => picked[question.id] ?? [])
}
