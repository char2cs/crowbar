import { useMemo } from 'react'
import { CodeView } from '@pierre/diffs/react'
import type { CodeViewHandle, CodeViewProps } from '@pierre/diffs/react'
import type { Ref } from 'react'
import { isDarkMode, useThemeVersion } from '@/features/editor/theme/use-theme-version'

export function DiffCodeView<Annotation = undefined>({
  viewKey,
  ref,
  options,
  ...props
}: CodeViewProps<Annotation, undefined> & {
  viewKey?: string
  ref?: Ref<CodeViewHandle<Annotation, undefined>>
}) {
  useThemeVersion()
  const themeType = isDarkMode() ? ('dark' as const) : ('light' as const)
  const themed = useMemo(() => ({ ...options, themeType }), [options, themeType])
  return <CodeView<Annotation> key={viewKey} ref={ref} options={themed} {...props} />
}
