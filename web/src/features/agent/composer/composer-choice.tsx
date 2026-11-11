import type { ToolFamily } from '@/features/agent/transcript/lib/tool-family'
import {
  useEffect,
  useEffectEvent,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from 'react'
import { Button } from '@/components/ui/button'
import { answerChoice } from '@/features/agent/api/agent-chat-commands'
import type { AgentChoice, AgentChoiceOption } from '@/features/agent/api/agent-chat-activity'
import {
  choiceDetail,
  choiceQuestions,
  describeChoice,
} from '@/features/agent/composer/lib/ask-labels'
import { ApiError } from '@/lib/errors/api-error'
import { actionErrorMessage } from '@/features/hosts/lib/host-failure-notice'
import {
  ChoiceButtons,
  ChoiceQuestion,
  ChoiceReadOnly,
} from '@/features/agent/composer/composer-choice-controls'
import { ChoiceForm } from '@/features/agent/composer/composer-choice-form'
import { ChoiceRequest } from '@/features/agent/composer/composer-choice-request'
import {
  fieldError,
  formContent,
  initialValues,
  parseForm,
  type FormValues,
} from '@/features/agent/composer/lib/ask-form'
import {
  foldCode,
  foldedCue,
  requestView,
  type AskRequest,
} from '@/features/agent/composer/lib/ask-request'

type CardState = {
  choiceId: string
  /** Picks for a prompt, keyed by question id. */
  byQuestion: Record<string, string[]>
  /** Which of the prompt's questions is on screen. */
  step: number
  /** An elicitation form's values, once the person has changed one. */
  values: FormValues | null
  /** Form fields whose errors are drawn: those the person has left. */
  shown: string[]
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
    byQuestion: {},
    step: 0,
    values: null,
    shown: [],
    sending: false,
    sent: false,
    error: '',
    gateClosed: false,
    unsupportedHere: false,
  }
}

/** The option a key names: a digit its place, Enter allowing once. */
function keyedOption(controls: AgentChoiceOption[], key: string) {
  if (key === 'Enter') return controls.find((option) => option.kind === 'allow')
  return /^[1-9]$/.test(key) ? controls[Number(key) - 1] : undefined
}

// The request the CLI is waiting on, drawn as the input box itself. Pending-but-not-answerable
// has no controls; a sent answer stays "sent" until the server resolves it (the terminal may
// race); terminal-only answers get a Terminal button.
export function ComposerChoice({
  wsId,
  chatId,
  choice,
  toolTarget,
  providerLabel,
  onOpenTerminal,
  request,
  family,
  active = true,
  stepper,
  onSkip,
}: {
  wsId: string
  chatId: string
  choice: AgentChoice
  /** What a permission is about to touch, from the call it gates. */
  toolTarget?: string
  providerLabel: string
  onOpenTerminal?: () => void
  /** What a permission would let run, from the ask's own copy of the call. */
  request?: AskRequest
  /** The gated call's family, which names what the permission is for. */
  family?: ToolFamily
  /** False while another card of the carousel is the one on screen: it stays mounted, with its
   *  picks, but answers no key. */
  active?: boolean
  stepper?: ReactNode
  /** Moves the carousel on without answering; absent when there is nowhere to go. */
  onSkip?: () => void
}) {
  const [stored, setStored] = useState<CardState>(() => freshCard(choice.id))
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  // The card replaces the composer the person sent from; with that focus gone to the page, the
  // card takes it so its number keys answer. Focus held anywhere else is left alone.
  useEffect(() => {
    if (active && (!document.activeElement || document.activeElement === document.body))
      cardRef.current?.focus({ preventScroll: true })
  }, [choice.id, active])
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
  const isElicitation = choice.options.length === 0 && choice.kind === 'elicitation'
  const fields = useMemo(
    () => (isElicitation ? parseForm(choice.schema) : null),
    [isElicitation, choice.schema],
  )
  const values = card.values ?? (fields ? initialValues(fields) : {})
  const formError = fields?.map((f) => fieldError(f, values[f.key])).find(Boolean) ?? ''
  // The flat row is what is left once questions are their own groups: a permission's allow, deny
  // and broader grants. An elicitation's only verb without a form is Decline.
  const controls: AgentChoiceOption[] = isElicitation
    ? [{ id: 'decline', kind: 'decline', label: 'Decline' }]
    : choice.options
  // Answerable is necessary and not sufficient: a relay can be holding the gate for a prompt whose
  // only options are ones Crowbar declines to send, and an empty row of buttons answers nothing.
  const canAnswerHere = answerable && (questions.length > 0 || controls.length > 0)
  // Unanswerable for any reason, or answerable but just 400'd on the shape sent: the normal
  // controls would only repeat the same dead end, so only the one thing that still works shows.
  const terminalOnly = !canAnswerHere || card.unsupportedHere
  // A form Crowbar can't compose is the terminal's to answer.
  const needsTerminal = isElicitation && !fields
  const busy = card.sending || card.sent
  const flat = canAnswerHere && !card.unsupportedHere && questions.length === 0 && !isElicitation

  const send = async (optionIds: string[], content?: unknown) => {
    update({ error: '', sending: true })
    try {
      await answerChoice(wsId, chatId, choice.id, content ? { optionIds, content } : { optionIds })
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
    if (!active || event.repeat || event.isComposing || busy) return
    if (event.metaKey || event.ctrlKey || event.altKey || (!enter && event.key === 'Enter')) return
    if (canAnswerHere && question) return pickQuestionByKey(event)
    if (!flat) return
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

  const step = Math.min(card.step, Math.max(questions.length - 1, 0))
  const lastStep = step >= questions.length - 1
  const question = questions[step]
  const picks = question ? (card.byQuestion[question.id] ?? []) : []
  // A digit names an option of the question on screen: it picks it, or ticks it when several may be.
  const pickQuestionByKey = (event: globalThis.KeyboardEvent) => {
    const option = /^[1-9]$/.test(event.key) ? question?.options[Number(event.key) - 1] : undefined
    if (!question || !option) return
    event.preventDefault()
    const next = !question.multi
      ? [option.id]
      : picks.includes(option.id)
        ? picks.filter((id) => id !== option.id)
        : [...picks, option.id]
    update({ byQuestion: { ...card.byQuestion, [question.id]: next } })
  }
  const answered = questions.every((q) => (card.byQuestion[q.id] ?? []).length > 0)
  // Every question's picks as the one list the answer endpoint takes, read off the questions so a
  // pick for a question gone from the prompt is never sent.
  const allPicks = questions.flatMap((q) => card.byQuestion[q.id] ?? [])
  const detail = choiceDetail(choice, toolTarget)
  const title = choice.kind === 'question' ? 'Question' : describeChoice(choice, family)
  const subtitle =
    questions.length > 1
      ? `${step + 1} of ${questions.length} questions`
      : choice.kind === 'elicitation' || view
        ? ''
        : detail

  return (
    <div
      ref={cardRef}
      className="request"
      role="group"
      aria-label={`${providerLabel} is waiting for your answer`}
      hidden={!active}
      data-testid="agent-choice-prompt"
      data-choice-id={choice.id}
      data-choice-kind={choice.kind}
      data-answerable={String(answerable)}
      tabIndex={0}
      onKeyDown={onKeyDown}
    >
      <div className="rq-head">
        <div className="rq-titles">
          {/* `alert`, like the interruption banner: it appears unasked and will not move until
              they act. */}
          <span className="rq-title" role="alert">
            {title}
          </span>
          {subtitle && (
            <span className="rq-sub" data-testid="agent-choice-detail">
              {subtitle}
            </span>
          )}
        </div>
        {stepper}
      </div>

      <div className="rq-body">
        {view && (
          <ChoiceRequest
            view={view}
            expanded={expanded}
            onExpand={() => setExpandedId(choice.id)}
          />
        )}
        {canAnswerHere && question && (
          <ChoiceQuestion
            key={question.id}
            question={question}
            picked={picks}
            busy={busy}
            onPick={(optionIds) =>
              update({ byQuestion: { ...card.byQuestion, [question.id]: optionIds } })
            }
          />
        )}
        {canAnswerHere && fields && !card.unsupportedHere && (
          <ChoiceForm
            fields={fields}
            values={values}
            shown={new Set(card.shown)}
            busy={busy}
            onChange={(key, value) => update({ values: { ...values, [key]: value } })}
            onLeave={(key) => update({ shown: [...new Set([...card.shown, key])] })}
          />
        )}
        {needsTerminal && canAnswerHere && !card.unsupportedHere && (
          <p className="rq-note">This form can only be filled in the terminal.</p>
        )}
        {!canAnswerHere && <ChoiceReadOnly options={choice.options} questions={questions} />}
      </div>

      {hiddenCue && (
        <span className="rq-hint warn" data-testid="agent-choice-hidden-lines">
          {hiddenCue}
        </span>
      )}
      {card.sent && (
        <span className="rq-hint" data-testid="agent-choice-sent">
          Answer sent. Waiting for {providerLabel} to confirm it.
        </span>
      )}
      {card.error && !terminalOnly && (
        <p className="rq-error" role="alert" data-testid="agent-choice-error">
          {card.error}
        </p>
      )}

      {(onOpenTerminal || !terminalOnly) && (
        <div className="rq-acts">
          {terminalOnly ? (
            onOpenTerminal && <TerminalLink onOpenTerminal={onOpenTerminal} />
          ) : question ? (
            <>
              {step > 0 ? (
                <Button
                  variant="outline"
                  disabled={busy}
                  onClick={() => update({ step: step - 1 })}
                >
                  Back
                </Button>
              ) : (
                onSkip && (
                  <Button variant="outline" disabled={busy} onClick={onSkip}>
                    Skip
                  </Button>
                )
              )}
              <span className="rq-grow" />
              {lastStep ? (
                <Button disabled={busy || !answered} onClick={() => void send(allPicks)}>
                  {questions.length > 1 ? 'Send answers' : 'Send answer'}
                </Button>
              ) : (
                <Button
                  disabled={busy || picks.length === 0}
                  onClick={() => update({ step: step + 1 })}
                >
                  Next
                </Button>
              )}
            </>
          ) : (
            <>
              {fields ? (
                <>
                  <Button
                    data-role="refuse"
                    variant="outline"
                    disabled={busy}
                    onClick={() => void send(['decline'])}
                  >
                    Decline
                  </Button>
                  <span className="rq-grow" />
                  {formError && <span className="rq-field-error">{formError}</span>}
                  <Button
                    disabled={busy || formError !== ''}
                    onClick={() => void send(['accept'], formContent(fields, values))}
                  >
                    Submit
                  </Button>
                </>
              ) : (
                <>
                  <ChoiceButtons
                    options={controls}
                    keyed={flat}
                    busy={busy}
                    onPick={(option) => void send([option.id])}
                  />
                  {needsTerminal && onOpenTerminal && (
                    <TerminalLink onOpenTerminal={onOpenTerminal} />
                  )}
                </>
              )}
            </>
          )}
        </div>
      )}
    </div>
  )
}

function TerminalLink({ onOpenTerminal }: { onOpenTerminal: () => void }) {
  return (
    <Button variant="outline" onClick={onOpenTerminal}>
      Terminal
    </Button>
  )
}
