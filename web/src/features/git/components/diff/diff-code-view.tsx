import { useMemo } from 'react'
import { CodeView } from '@pierre/diffs/react'
import type { CodeViewHandle, CodeViewProps } from '@pierre/diffs/react'
import type { Ref } from 'react'
import { isDarkMode, useThemeVersion } from '@/features/editor/theme/use-theme-version'

/** Shared @pierre/diffs surface used by branch review and inline chat diffs. */
export function DiffCodeView<Annotation = undefined>({
  viewKey,
  ref,
  options,
  ...props
}: CodeViewProps<Annotation, undefined> & {
  viewKey?: string
  ref?: Ref<CodeViewHandle<Annotation, undefined>>
}) {
  // The library's default 'system' follows the OS, not the app's own light/dark
  // toggle, so the two disagree whenever they are set differently.
  useThemeVersion()
  const themeType = isDarkMode() ? ('dark' as const) : ('light' as const)
  const themed = useMemo(() => ({ ...options, themeType }), [options, themeType])
  return <CodeView<Annotation> key={viewKey} ref={ref} options={themed} {...props} />
}
