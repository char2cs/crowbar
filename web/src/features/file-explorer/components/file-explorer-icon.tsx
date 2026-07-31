import DOMPurify from 'dompurify'
import { FileText } from '@phosphor-icons/react'
import { useMemo } from 'react'
import { iconThemeRegistry } from '@/extensions/icon-themes/icon-theme-registry'
import { useSettingsStore } from '@/features/settings/store'

export interface FileExplorerIconProps {
  fileName?: string
  filePath?: string
  isDirectory?: boolean
  /** Alias for isDirectory. */
  isDir?: boolean
  isExpanded?: boolean
  className?: string
  size?: number
  /**
   * Oracle anchor id — see `native/oracle/ANCHORS.md`. Threaded as a prop rather
   * than set from outside because the painted node is chosen in here and callers
   * cannot reach it. Inert: a marker attribute, no styling or behaviour.
   */
  'data-oracle-id'?: string
}

export function FileExplorerIcon({
  fileName = '',
  isDirectory,
  isDir,
  isExpanded = false,
  className,
  size = 16,
  'data-oracle-id': oracleId,
}: FileExplorerIconProps) {
  const iconThemeId = useSettingsStore((state) => state.settings.iconTheme)

  const iconResult = useMemo(() => {
    const theme = iconThemeRegistry.getTheme(iconThemeId) ?? iconThemeRegistry.getAllThemes()[0]
    if (!theme) return null
    try {
      return theme.getFileIcon(fileName, isDirectory ?? isDir ?? false, isExpanded)
    } catch {
      return null
    }
  }, [iconThemeId, fileName, isDirectory, isDir, isExpanded])

  const iconSpanStyle = {
    display: 'inline-flex',
    alignItems: 'center',
    width: size,
    height: size,
    flexShrink: 0,
  } as const

  if (!iconResult) {
    return <FileText className={className} size={size} data-oracle-id={oracleId} />
  }

  if (iconResult.component) {
    return (
      <span className={className} style={iconSpanStyle} data-oracle-id={oracleId}>
        {iconResult.component}
      </span>
    )
  }

  if (iconResult.svg) {
    const sanitizedSvg = DOMPurify.sanitize(iconResult.svg, {
      USE_PROFILES: { svg: true, svgFilters: true },
    })
    return (
      <span
        className={className}
        style={iconSpanStyle}
        data-oracle-id={oracleId}
        dangerouslySetInnerHTML={{ __html: sanitizedSvg }}
      />
    )
  }

  return <FileText className={className} size={size} data-oracle-id={oracleId} />
}
