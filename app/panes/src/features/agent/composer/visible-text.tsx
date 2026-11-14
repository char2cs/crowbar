import { visibleParts } from '@/features/agent/composer/lib/ask-request'

/** Text with every character that would not draw as itself written out. */
export function Visible({ text }: { text: string }) {
  return visibleParts(text).map((part, i) =>
    part.escaped ? (
      // react-doctor-disable-next-line no-array-index-as-key -- parts of one fixed string, never reordered
      <span key={i} className="choice-escape" title="A character that does not draw">
        {part.text}
      </span>
    ) : (
      part.text
    ),
  )
}
