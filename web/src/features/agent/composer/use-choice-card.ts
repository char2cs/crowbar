import { useEffect, useEffectEvent, useMemo, useState, type KeyboardEvent } from 'react'
import { answerChoice } from '@/features/agent/api/agent-chat-commands'
import type { AgentChoice, AgentChoiceOption } from '@/features/agent/api/agent-chat-activity'
import { choiceQuestions } from '@/features/agent/composer/lib/ask-labels'
import { ApiError } from '@/lib/errors/api-error'
import { actionErrorMessage } from '@/features/hosts/lib/host-failure-notice'
import {
  fieldError,
  formContent,
  initialValues,
  parseForm,
  type FormValues,
} from '@/features/agent/composer/lib/ask-form'

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

export function useChoiceCard({
  wsId,
  chatId,
  choice,
  active,
}: {
  wsId: string
  chatId: string
  choice: AgentChoice
  active: boolean
}) {
  const [stored, setStored] = useState<CardState>(() => freshCard(choice.id))
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

  const step = Math.min(card.step, Math.max(questions.length - 1, 0))
  const lastStep = step >= questions.length - 1
  const question = questions[step]
  const picks = question ? (card.byQuestion[question.id] ?? []) : []
  // A digit names an option of the question on screen: it picks it, or ticks it when several may be.
  const pickQuestionByKey = (event: globalThis.KeyboardEvent) => {
    if (event.key === 'Enter') {
      if (picks.length === 0) return
      event.preventDefault()
      if (!lastStep) update({ step: step + 1 })
      else if (answered) void send(allPicks)
      return
    }
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
  const allPicks = questions.flatMap((q) => card.byQuestion[q.id] ?? []) // A digit picks that option; Enter allows once, only from the card itself so
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
    // From the card itself, a key answers; from a control inside it the control keeps its keys,
    // except that a form's Escape declines and Enter in a field submits.
    if (event.target === event.currentTarget) return pickByKey(event.nativeEvent, true)
    if (!active || busy || !fields || card.unsupportedHere || event.nativeEvent.isComposing) return
    if (event.key === 'Escape') {
      event.preventDefault()
      void send(['decline'])
    } else if (event.key === 'Enter' && event.target instanceof HTMLInputElement && !formError) {
      event.preventDefault()
      void send(['accept'], formContent(fields, values))
    }
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

  return {
    card,
    update,
    answerable,
    questions,
    fields,
    values,
    formError,
    controls,
    canAnswerHere,
    terminalOnly,
    needsTerminal,
    busy,
    flat,
    send,
    onKeyDown,
    step,
    lastStep,
    question,
    picks,
    answered,
    allPicks,
  }
}
