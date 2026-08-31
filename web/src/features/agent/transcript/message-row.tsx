import { memo } from 'react'
import { cn } from '@/lib/utils'
import type { AgentChatMessage, AgentProvider, AgentToolCall } from '@/features/agent/api/agent-api'
import { Button } from '@/components/ui/button'
import { ProviderIcon } from '@/components/ui/provider-icon'
import { CopyIcon } from '@/features/agent/shared/agent-icons'
import {
  turnLatencyLabel,
  turnTimeLabel,
  turnTimestampLabel,
  turnTimeTitle,
} from '@/features/agent/lib/turn-time'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { AgentTurnTools } from '@/features/agent/transcript/turn-tools'
import { toast } from '@/features/window/stores/toast-store'

function providerName(providers: AgentProvider[], id: string): string {
  return providers.find((provider) => provider.id === id)?.displayName ?? id
}

function providerGlyph(providers: AgentProvider[], id: string): string | undefined {
  return providers.find((provider) => provider.id === id)?.icon
}

/** Copy a turn's own text forward — the box already sends it as markdown, so
 *  what a reader gets back is exactly what the provider produced. */
async function copyTurn(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast.success('Copied to clipboard')
  } catch {
    toast.error('Could not copy to clipboard')
  }
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
  providers,
  firstTurn = false,
  firstReply = false,
  streaming = false,
  toolCallsByTurn,
  precedingUserAt,
}: {
  message: AgentChatMessage
  providers: AgentProvider[]
  /** This is the chat's very first turn — kept in the empty document's own
   *  typography rather than switched to a bubble the instant it is sent. Only
   *  ever true for a `user` message; see `agent-transcript.tsx` for how the
   *  ABSOLUTE first turn is told apart from merely the first one loaded. */
  firstTurn?: boolean
  /** The assistant's answer to the frozen first turn — kept at the frozen
   *  document's own larger size rather than dropping to ordinary reply
   *  prose the instant the turn is over. Only ever true for an `assistant`
   *  message; see `agent-transcript.tsx` for how it is told apart from
   *  merely being the first assistant message loaded. */
  firstReply?: boolean
  /** This is a still-open streaming bubble, not a closed, ledger-confirmed
   *  turn. The turnbar (provider icon, copy, elapsed time) reports on a
   *  FINISHED turn — showing it mid-stream would offer to copy text that is
   *  still changing and time a turn that has not ended yet. Also the switch
   *  between the interactive `MarkdownMessage` (needed for
   *  `applyStreamedValue`'s patch-in-place) and the cheaper, static
   *  `MarkdownMessageStatic` every settled message renders through. */
  streaming?: boolean
  /** Finished tool calls for every turn in this transcript, keyed by turnId —
   *  only ever passed for closed (non-streaming) assistant messages, since a
   *  still-running call belongs to the working line, not a turn already
   *  answered. */
  toolCallsByTurn?: Map<string, AgentToolCall[]>
  /** The `at` of the user turn this reply actually answers — what the
   *  turnbar times ITSELF against: how long the agent took to answer, not
   *  how long ago that was. Absent for a reply with no user turn before it
   *  in the loaded window (a harness-injected one, say), where the turnbar
   *  falls back to reporting how long ago it happened instead. */
  precedingUserAt?: string
}) {
  const user = message.role === 'user'
  const assistant = message.role === 'assistant'
  const harness = message.role === 'harness'
  const notice = message.role === 'notice'
  const frozen = user && firstTurn
  const frozenReply = assistant && firstReply
  const glyph = assistant ? providerGlyph(providers, message.providerId) : undefined
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
      // `stacked`: a user row's timestamp sits BELOW its message, not beside
      // it — column direction, the message and the <time> as two siblings
      // rather than one nested inside the other. `me` still does the actual
      // left/right call: end-aligned for an ordinary bubble, left where it
      // falls for the frozen full-width first turn.
      className={cn('row', user && 'stacked', user && !frozen && 'me')}
      data-sequence={message.sequence}
      data-testid={`agent-message-${message.sequence}`}
      data-role={message.role}
      data-first-turn={frozen ? 'true' : undefined}
      data-first-reply={frozenReply ? 'true' : undefined}
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
          frozenReply && 'frozen',
          !user && !harness && !notice && !assistant && 'assistant',
        )}
      >
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
          //
          // Only the still-streaming bubble gets the interactive editor —
          // `applyStreamedValue` patches an existing Plate document in place
          // as tokens arrive, which only works on a real `usePlateEditor`
          // instance. Every settled message (every non-streaming row, on
          // either side) is static content and renders through `PlateStatic`
          // instead, at a fraction of the interactive editor's cost.
          streaming ? (
            <MarkdownMessage className="break-words">{message.text}</MarkdownMessage>
          ) : (
            <MarkdownMessageStatic className="break-words">{message.text}</MarkdownMessageStatic>
          )
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
        {/* What the turn actually DID, before what a reader can do about it —
            tool calls are the turn's own work, the turnbar below is a
            reader's actions on the finished result. Neither exists until the
            turn has actually closed: a streaming bubble's calls still belong
            to the working line, and copying or timing text that is still
            changing offers a reader something that isn't real yet. */}
        {assistant && !streaming && toolCallsByTurn && (
          <AgentTurnTools callsByTurn={toolCallsByTurn} turnId={message.turnId ?? ''} />
        )}
        {assistant && !streaming && (
          <div className="turnbar" data-testid="message-turn-actions">
            {glyph && <ProviderIcon svg={glyph} className="size-3" />}
            <Button
              size="icon-sm"
              variant="ghost"
              tooltip="Copy"
              aria-label="Copy this message"
              onClick={() => void copyTurn(message.text)}
            >
              <CopyIcon size={12} className="size-3" />
            </Button>
            <time className="turn-time" dateTime={message.at} title={turnTimeTitle(message.at)}>
              {precedingUserAt
                ? turnLatencyLabel(precedingUserAt, message.at)
                : turnTimeLabel(message.at)}
            </time>
          </div>
        )}
      </div>
      {/* Below the bubble, not inside it — a fact ABOUT the turn, not part of
          what was said. Sibling of the message div so `.row.stacked` can lay
          the two out as separate lines regardless of which shape the message
          itself takes (bubble or frozen). Timestamp only — a user's own turn
          has nothing to time itself against, unlike a reply's latency. */}
      {user && (
        <time className="turn-time" dateTime={message.at} title={turnTimeTitle(message.at)}>
          {turnTimestampLabel(message.at)}
        </time>
      )}
    </article>
  )
}

export const MessageRow = memo(MessageRowComponent)
