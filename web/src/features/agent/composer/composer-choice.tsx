import type { ToolFamily } from '@/features/agent/transcript/lib/tool-family'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import type { AgentChoice } from '@/features/agent/api/agent-chat-activity'
import { choiceDetail, describeChoice } from '@/features/agent/composer/lib/ask-labels'
import {
  ChoiceButtons,
  ChoiceQuestion,
  ChoiceReadOnly,
} from '@/features/agent/composer/composer-choice-controls'
import { useChoiceCard } from '@/features/agent/composer/use-choice-card'
import { ChoiceForm } from '@/features/agent/composer/composer-choice-form'
import { ChoiceRequest } from '@/features/agent/composer/composer-choice-request'
import { formContent } from '@/features/agent/composer/lib/ask-form'
import {
  foldCode,
  foldedCue,
  requestView,
  type AskRequest,
} from '@/features/agent/composer/lib/ask-request'

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
  const {
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
  } = useChoiceCard({ wsId, chatId, choice, active })
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
