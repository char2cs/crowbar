import DOMPurify from 'dompurify'
import { FileText } from '@phosphor-icons/react'
import { iconThemeRegistry } from '@/extensions/icon-themes/icon-theme-registry'
import { useIconThemeRegistryVersion } from '@/extensions/icon-themes/use-icon-theme-registry-version'
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
}

function resolveIcon(themeId: string, fileName: string, isDir: boolean, isExpanded: boolean) {
  const theme = iconThemeRegistry.getTheme(themeId) ?? iconThemeRegistry.getAllThemes()[0]
  if (!theme) return null
  try {
    return theme.getFileIcon(fileName, isDir, isExpanded)
  } catch {
    return null
  }
}

export function FileExplorerIcon({
  fileName = '',
  isDirectory,
  isDir,
  isExpanded = false,
  className,
  size = 16,
}: FileExplorerIconProps) {
  const iconThemeId = useSettingsStore((state) => state.settings.iconTheme)
  // Re-render when a theme changes or its lazily loaded icons arrive.
  useIconThemeRegistryVersion()
  const iconResult = resolveIcon(iconThemeId, fileName, isDirectory ?? isDir ?? false, isExpanded)

  const iconSpanStyle = {
    display: 'inline-flex',
    alignItems: 'center',
    width: size,
    height: size,
    flexShrink: 0,
  } as const

  if (!iconResult) {
    return <FileText className={className} size={size} />
  }

  if (iconResult.component) {
    return (
      <span className={className} style={iconSpanStyle}>
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
        dangerouslySetInnerHTML={{ __html: sanitizedSvg }}
      />
    )
  }

  return <FileText className={className} size={size} />
}
