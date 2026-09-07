/**
 * The boxed-card footprint every non-image attachment kind shares — a
 * text-attachment pill and a file card were each their own thin, single-line
 * pill/row before this; reported live as needing to look like a real
 * attachment (Claude's own image-attachment card was the reference point:
 * a sizeable, rounded, bordered box), not a tag lost in the text.
 *
 * Deliberately a plain constant, not a component: a text-attachment pill
 * renders a `<button>`, a file card renders an `<a>` (via `PlateElement`'s
 * own `as` prop) — sharing the CLASS STRING keeps both kinds visually
 * identical without forcing either into a wrapper element it doesn't
 * otherwise need.
 */
export const ATTACHMENT_BOX_CLASS =
  'chat-attachment-box flex size-28 flex-col items-center justify-center gap-1 rounded-xl border border-border bg-muted/40 p-3 text-center no-underline hover:bg-accent/50'
