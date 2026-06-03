import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { EditorView } from '@codemirror/view'
import { cn } from '@/utils/cn'

/** Transparent CodeMirror theme so the editor blends into its container. */
export const transparentMarkdownTheme = EditorView.theme({
  '&': { backgroundColor: 'transparent !important', color: 'var(--foreground)' },
  '&.cm-focused': { outline: 'none !important', backgroundColor: 'transparent !important' },
  '.cm-content': { caretColor: 'var(--foreground)', padding: '0' },
  '.cm-cursor': { borderLeftColor: 'var(--foreground)' },
  '.cm-placeholder': { color: 'var(--muted-foreground)', opacity: '0.4' },
  '.cm-line': { padding: '0' },
  '.cm-scroller': { fontFamily: 'inherit', backgroundColor: 'transparent !important' },
  '.cm-gutters': { backgroundColor: 'transparent !important', border: 'none' },
  '.cm-activeLine': { backgroundColor: 'transparent !important' },
  '.cm-activeLineGutter': { backgroundColor: 'transparent !important' },
  '.cm-selectionBackground': { backgroundColor: 'color-mix(in srgb, var(--primary) 20%, transparent) !important' },
  '&.cm-focused .cm-selectionBackground': { backgroundColor: 'color-mix(in srgb, var(--primary) 30%, transparent) !important' },
})

/** Shared prose styling for rendered markdown across the branch-review feature. */
export const MARKDOWN_PROSE_CLASS =
  'prose prose-sm prose-invert max-w-none text-sm text-foreground ' +
  '[&_h1]:text-base [&_h1]:font-semibold [&_h1]:mb-2 [&_h1]:mt-3 ' +
  '[&_h2]:text-sm [&_h2]:font-semibold [&_h2]:mb-1.5 [&_h2]:mt-3 ' +
  '[&_h3]:text-sm [&_h3]:font-medium [&_h3]:mb-1 [&_h3]:mt-2 ' +
  '[&_p]:mb-2 [&_p]:leading-relaxed ' +
  '[&_ul]:my-1.5 [&_ul]:pl-4 [&_li]:my-0.5 ' +
  '[&_ol]:my-1.5 [&_ol]:pl-4 ' +
  '[&_code]:rounded [&_code]:bg-muted/60 [&_code]:px-1 [&_code]:py-0.5 [&_code]:text-xs [&_code]:font-mono ' +
  '[&_pre]:rounded-lg [&_pre]:bg-muted/60 [&_pre]:p-3 [&_pre]:text-xs [&_pre]:overflow-x-auto ' +
  '[&_pre_code]:bg-transparent [&_pre_code]:p-0 ' +
  '[&_strong]:font-semibold [&_strong]:text-foreground ' +
  '[&_em]:italic ' +
  '[&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground ' +
  '[&_hr]:border-border [&_hr]:my-3 ' +
  '[&_a]:text-primary [&_a]:underline-offset-2 [&_a]:hover:underline'

export function MarkdownPreview({ children, className }: { children: string; className?: string }) {
  return (
    <div className={cn(MARKDOWN_PROSE_CLASS, className)}>
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{children}</ReactMarkdown>
    </div>
  )
}
