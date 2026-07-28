/**
 * Shared prose styling for rendered markdown across the branch-review feature.
 *
 * Used by BOTH the renderer (`MarkdownPreview`, for a posted comment) and the
 * comment editor's editable, so what you type is styled identically to what
 * appears once you post it. That equality is the point of a WYSIWYG composer;
 * if the two ever drift, the composer stops being a preview of anything.
 *
 * Its own module rather than living beside `MarkdownPreview`: that file pulls
 * react-markdown, DOMPurify and a lazy shiki singleton, none of which the
 * editor needs to know one class string.
 */
export const MARKDOWN_PROSE_CLASS =
  'prose prose-sm prose-invert max-w-none text-sm text-foreground ' +
  '[&_h1]:text-base [&_h1]:font-semibold [&_h1]:mb-2 [&_h1]:mt-3 ' +
  '[&_h2]:text-sm [&_h2]:font-semibold [&_h2]:mb-1.5 [&_h2]:mt-3 ' +
  '[&_h3]:text-sm [&_h3]:font-medium [&_h3]:mb-1 [&_h3]:mt-2 ' +
  '[&_p]:mb-2 [&_p]:leading-relaxed ' +
  // Markers are explicit because Tailwind's preflight sets `list-style: none`
  // on every list, so a comment's bullets and numbers were invisible — two
  // points and three points looked like one paragraph with odd spacing. Task
  // lists opt back out: their marker is the checkbox.
  '[&_ul]:my-1.5 [&_ul]:pl-4 [&_ul]:list-disc [&_li]:my-0.5 ' +
  '[&_ol]:my-1.5 [&_ol]:pl-4 [&_ol]:list-decimal ' +
  '[&_.contains-task-list]:list-none [&_.task-list-item]:list-none ' +
  '[&_code]:rounded [&_code]:bg-muted/60 [&_code]:px-1 [&_code]:py-0.5 [&_code]:text-xs [&_code]:font-mono ' +
  '[&_pre]:rounded-lg [&_pre]:bg-muted/60 [&_pre]:p-3 [&_pre]:text-xs [&_pre]:overflow-x-auto ' +
  '[&_pre_code]:bg-transparent [&_pre_code]:p-0 ' +
  '[&_strong]:font-semibold [&_strong]:text-foreground ' +
  '[&_em]:italic ' +
  '[&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground ' +
  '[&_hr]:border-border [&_hr]:my-3 ' +
  '[&_a]:text-primary [&_a]:underline-offset-2 [&_a]:hover:underline'
