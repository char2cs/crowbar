import { memo } from 'react'
import { cn } from '@/lib/utils'
import type { AgentChatMessage, AgentProvider } from '@/features/agent/api/agent-api'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'

function providerName(providers: AgentProvider[], id: string): string {
  return providers.find((provider) => provider.id === id)?.displayName ?? id
}

/**
 * One recorded turn.
 *
 * Four roles reach here, and the discriminator used to be a single `user`
 * boolean because there were only two. The other two are both "text in this
 * conversation that NOBODY in it typed", and they are separate roles because
 * they answer to different people:
 *
 *   - `harness` is the provider's own machinery talking to its own model — a
 *     background-subagent completion report is the measured case. The agent
 *     genuinely received it, so it is shown; the human did not write it, so it
 *     must never wear the user's bubble. Anything else quotes a person saying
 *     something they never said.
 *   - `notice` is Crowbar relaying a provider's OWN words about why a chat
 *     stopped ("You've hit your usage limit…").
 *
 * A role this build has never heard of still renders its text, unstyled and
 * verbatim. A message that vanishes is strictly worse than one that looks plain:
 * the ledger recorded something the agent acted on, and a client too old to name
 * it is not a reason to hide it.
 *
 * Everything here is derived from props. The pane this list lives in is RETAINED
 * across chat selection, so a row holding state of its own would carry one
 * chat's UI onto another chat's transcript.
 */
function MessageRowComponent({
  message,
  showProvider,
  providers,
  firstTurn = false,
}: {
  message: AgentChatMessage
  showProvider: boolean
  providers: AgentProvider[]
  /** This is the chat's very first turn — kept in the empty document's own
   *  typography rather than switched to a bubble the instant it is sent. Only
   *  ever true for a `user` message; see `agent-transcript.tsx` for how the
   *  ABSOLUTE first turn is told apart from merely the first one loaded. */
  firstTurn?: boolean
}) {
  const user = message.role === 'user'
  const assistant = message.role === 'assistant'
  const harness = message.role === 'harness'
  const notice = message.role === 'notice'
  const frozen = user && firstTurn
  // A sent message keeps the shape of the box it was typed in: stadium while it
  // fits on one line, 18px once it wraps.
  const multi = message.text.length > 60 || message.text.includes('\n')
  // A harness payload can run well past one line — a subagent completion
  // report carries several attributes plus a multi-line body. Collapse those
  // behind the same <details> pattern ChoiceSchema already uses for a
  // permission card's JSON Schema, rather than letting it push the rest of
  // the turn down. Length/line-count only — this has no idea what the
  // payload means, only how big it is.
  const bigHarness = harness && (message.text.length > 160 || message.text.split('\n').length > 3)

  return (
    <article
      className={cn('row', user && !frozen && 'me')}
      data-sequence={message.sequence}
      data-testid={`agent-message-${message.sequence}`}
      data-role={message.role}
      data-first-turn={frozen ? 'true' : undefined}
    >
      <div
        // A notice is a live announcement, not prose — the reader may already be
        // scrolled away when the provider stops the chat, and it is the only row
        // here that anybody needs told about.
        role={notice ? 'alert' : undefined}
        className={cn(
          user && !frozen && 'bubble',
          user && !frozen && multi && 'multi',
          frozen && 'frozen',
          harness && 'harness',
          notice && 'notice',
          assistant && 'assistant',
          !user && !harness && !notice && !assistant && 'assistant',
        )}
      >
        {showProvider && <p className="meta">{providerName(providers, message.providerId)}</p>}
        {/* Said in words, not only in styling. The body of one of these is raw
            provider markup a reader has every reason to mistake for their own
            last message, and a muted box alone does not say whose words they are. */}
        {harness && (
          <p className="meta" data-testid="message-harness-label">
            Sent to the agent by {providerName(providers, message.providerId)} — not by you
          </p>
        )}
        {assistant || user ? (
          // BOTH SIDES OF THE CONVERSATION, through the one engine. A prompt is
          // composed in a rich markdown editor and SENT as markdown, so showing
          // it back as source is the box's own content un-rendering the instant
          // it is sent — the same table, the same fenced block, two appearances
          // one line apart.
          <MarkdownMessage className="break-words">{message.text}</MarkdownMessage>
        ) : harness ? (
          // Verbatim, in <code> — it's markup (`<task-notification>` is an HTML
          // tag to a markdown parser, which would swallow the row whole), and
          // literal machine text reads as data, not prose, whichever provider's
          // harness produced it. Never parsed for meaning: reading `status=`, or
          // picking a label from the tag name, would mean this renderer learning
          // one provider's own wire vocabulary.
          bigHarness ? (
            <details>
              <summary>Show payload</summary>
              <pre>
                <code>{message.text}</code>
              </pre>
            </details>
          ) : (
            <code>{message.text}</code>
          )
        ) : (
          // Verbatim for the one role left: a notice is the provider's exact
          // sentence, which is the one thing about it worth showing.
          <span>{message.text}</span>
        )}
        {/* Provenance, not a headline: what the CLI ITSELF said it ran this turn
            at. It is a different fact from the chat's requested selection — the
            two can legitimately disagree — so it is only ever shown for a turn
            the provider actually reported one on. */}
        {assistant && message.effort && (
          <p
            className="meta"
            title="Reasoning effort the provider reported for this turn"
            data-testid="message-effort"
          >
            {message.effort} effort
          </p>
        )}
      </div>
    </article>
  )
}

export const MessageRow = memo(MessageRowComponent)
