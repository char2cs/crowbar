import type { CSSProperties } from 'react'

/** A chat message keeps a single newline as a line break, as the editor it was
 *  typed or streamed in showed it; a document collapses it, per CommonMark. */
export const CHAT_PROSE_STYLE: CSSProperties = { whiteSpace: 'pre-wrap' }
