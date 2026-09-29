import { CodeView } from '@pierre/diffs/react'
import type { CodeViewHandle, CodeViewProps } from '@pierre/diffs/react'
import type { Ref } from 'react'

/** Shared @pierre/diffs surface used by branch review and inline chat diffs. */
export function DiffCodeView<Annotation = undefined>({
  viewKey,
  ref,
  ...props
}: CodeViewProps<Annotation, undefined> & {
  viewKey?: string
  ref?: Ref<CodeViewHandle<Annotation, undefined>>
}) {
  return <CodeView<Annotation> key={viewKey} ref={ref} {...props} />
}
