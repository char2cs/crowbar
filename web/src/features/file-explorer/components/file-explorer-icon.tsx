import DOMPurify from 'dompurify'
import { cloneElement, isValidElement } from 'react'
import { File, Folder, FolderOpen } from '@phosphor-icons/react'
import { iconThemeRegistry } from '@/extensions/icon-themes/icon-theme-registry'
import { useIconThemeRegistryVersion } from '@/extensions/icon-themes/use-icon-theme-registry-version'
import type { FileIconResult } from '@/extensions/icon-themes/types'
import { useSettingsStore } from '@/features/settings/store'

interface FileExplorerIconProps {
  fileName: string
  isDir?: boolean
  isExpanded?: boolean
  isSymlink?: boolean
  size?: number
  className?: string
}

function resolveIcon(
  themeId: string,
  fileName: string,
  isDir: boolean,
  isExpanded: boolean,
  isSymlink: boolean,
): FileIconResult | null {
  const theme = iconThemeRegistry.getTheme(themeId) ?? iconThemeRegistry.getAllThemes()[0]
  if (!theme) return null
  try {
    return theme.getFileIcon(fileName, isDir, isExpanded, isSymlink)
  } catch {
    return null
  }
}

function ThemedIcon({
  icon,
  isDir,
  isExpanded,
  size,
  className,
}: {
  icon: FileIconResult | null
  isDir: boolean
  isExpanded: boolean
  size: number
  className?: string
}) {
  if (icon?.component) {
    if (isValidElement(icon.component)) {
      return cloneElement(icon.component, { className, size } as React.Attributes & {
        className?: string
        size: number
      })
    }
    return <span className={className}>{icon.component}</span>
  }
  if (icon?.svg) {
    const sanitizedSvg = DOMPurify.sanitize(icon.svg, {
      USE_PROFILES: { svg: true, svgFilters: true },
    })
    return (
      <span
        className={className}
        style={{ width: size, height: size, display: 'inline-block', lineHeight: 0 }}
        dangerouslySetInnerHTML={{ __html: sanitizedSvg }}
      />
    )
  }
  // No theme, or the theme has nothing for this file (e.g. its icon set is still loading).
  const Fallback = isDir ? (isExpanded ? FolderOpen : Folder) : File
  return <Fallback size={size} className={className} weight="duotone" />
}

export function FileExplorerIcon({
  fileName,
  isDir = false,
  isExpanded = false,
  isSymlink = false,
  size = 16,
  className,
}: FileExplorerIconProps) {
  const iconThemeId = useSettingsStore((s) => s.settings.iconTheme)
  // Re-render when a theme changes or its lazily loaded icons arrive.
  useIconThemeRegistryVersion()
  const icon = (
    <ThemedIcon
      icon={resolveIcon(iconThemeId, fileName, isDir, isExpanded, isSymlink)}
      isDir={isDir}
      isExpanded={isExpanded}
      size={size}
      className={className}
    />
  )
  if (!isSymlink) return icon

  return (
    <span className="relative inline-block">
      {icon}
      <svg
        width="8"
        height="8"
        viewBox="0 0 16 16"
        className="-bottom-0.5 -right-0.5 absolute text-secondary"
        role="img"
        aria-label="Symlink"
      >
        <title>Symlink</title>
        <path
          fill="currentColor"
          d="M6.879 9.934a.81.81 0 0 1-.575-.238 3.818 3.818 0 0 1 0-5.392l3-3C10.024.584 10.982.187 12 .187s1.976.397 2.696 1.117a3.818 3.818 0 0 1 0 5.392l-1.371 1.371a.813.813 0 0 1-1.149-1.149l1.371-1.371A2.19 2.19 0 0 0 12 1.812c-.584 0-1.134.228-1.547.641l-3 3a2.19 2.19 0 0 0 0 3.094.813.813 0 0 1-.575 1.387z"
        />
        <path
          fill="currentColor"
          d="M4 15.813a3.789 3.789 0 0 1-2.696-1.117 3.818 3.818 0 0 1 0-5.392l1.371-1.371a.813.813 0 0 1 1.149 1.149l-1.371 1.371A2.19 2.19 0 0 0 4 14.188c.585 0 1.134-.228 1.547-.641l3-3a2.19 2.19 0 0 0 0-3.094.813.813 0 0 1 1.149-1.149 3.818 3.818 0 0 1 0 5.392l-3 3A3.789 3.789 0 0 1 4 15.813z"
        />
      </svg>
    </span>
  )
}
