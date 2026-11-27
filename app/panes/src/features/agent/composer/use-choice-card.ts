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
  byQuestion: Record<string, string[]>
  step: number
  values: FormValues | null
  shown: string[]
  sending: boolean
  sent: boolean
  error: string
  gateClosed: boolean
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
  const controls: AgentChoiceOption[] = isElicitation
    ? [{ id: 'decline', kind: 'decline', label: 'Decline' }]
    : choice.options
  const canAnswerHere = answerable && (questions.length > 0 || controls.length > 0)
  const terminalOnly = !canAnswerHere || card.unsupportedHere
  const needsTerminal = isElicitation && !fields
  const busy = card.sending || card.sent
  const flat = canAnswerHere && !card.unsupportedHere && questions.length === 0 && !isElicitation

  const send = async (optionIds: string[], content?: unknown) => {
    update({ error: '', sending: true })
    try {
      await answerChoice(wsId, chatId, choice.id, content ? { optionIds, content } : { optionIds })
      update({ sent: true, sending: false })
    } catch (failure) {
      update({
        sending: false,
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
  const allPicks = questions.flatMap((q) => card.byQuestion[q.id] ?? [])
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
