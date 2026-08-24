/**
 * The blank page a chat starts on.
 *
 * It is a DOCUMENT, not a prompt box with a hint under it. A good first message
 * to an agent looks like a short spec — context, then the ask — and an empty
 * document invites that in a way an input caret never has. The typography is the
 * app's markdown measure, so what someone writes here sets the way it will read
 * once it is sent.
 *
 * It says almost nothing on purpose. Instructions about what to type are the
 * thing a user reads once and resents on every chat afterwards.
 */
export function AgentEmptyDocument({ title }: { title?: string }) {
  return (
    <div className="doc" data-testid="agent-empty-document">
      <div className="sheet">
        <h1>{title || 'Untitled'}</h1>
        <p>Describe what you want to happen.</p>
      </div>
    </div>
  )
}
