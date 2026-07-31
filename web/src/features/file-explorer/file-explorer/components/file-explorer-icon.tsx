import DOMPurify from 'dompurify'
import { cloneElement, isValidElement } from 'react'
import { File, Folder, FolderOpen } from '@phosphor-icons/react'
import { iconThemeRegistry } from '@/extensions/icon-themes/icon-theme-registry'
import { useSettingsStore } from '@/features/settings/store'

interface FileExplorerIconProps {
  fileName: string
  isDir: boolean
  isExpanded?: boolean
  isSymlink?: boolean
  size?: number
  className?: string
  /**
   * Oracle anchor id — see `native/oracle/ANCHORS.md`. Threaded as a prop rather
   * than set from outside because the painted node is chosen in here (icon
   * theme, Phosphor fallback, symlink wrapper) and callers cannot reach it.
   * Inert: a marker attribute, no styling and no behaviour.
   */
  'data-oracle-id'?: string
}

export function FileExplorerIcon({
  fileName,
  isDir,
  isExpanded = false,
  isSymlink = false,
  size = 14,
  className = 'text-muted-foreground',
  'data-oracle-id': oracleId,
}: FileExplorerIconProps) {
  const iconThemeValue = useSettingsStore((s) => s.settings.iconTheme)
  const iconTheme = iconThemeRegistry.getTheme(iconThemeValue)

  // When no icon theme is registered, use Phosphor Icons as built-in fallback
  if (!iconTheme) {
    const iconProps = { size, className, weight: 'duotone' } as const
    if (isDir) {
      const FolderIcon = isExpanded ? FolderOpen : Folder
      const folderNode = <FolderIcon {...iconProps} data-oracle-id={oracleId} />
      if (!isSymlink) return folderNode
    } else {
      const fileNode = <File {...iconProps} data-oracle-id={oracleId} />
      if (!isSymlink) return fileNode
    }
    // Symlink: wrap with badge
    const baseIcon = isDir ? (
      isExpanded ? (
        <FolderOpen {...iconProps} />
      ) : (
        <Folder {...iconProps} />
      )
    ) : (
      <File {...iconProps} />
    )
    return (
      // The anchor goes on the wrapper, not the base icon: the wrapper is the
      // box the row lays out, and the badge hangs outside the icon's own bounds.
      <span className="relative inline-block" data-oracle-id={oracleId}>
        {baseIcon}
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

  const iconResult = iconTheme.getFileIcon(fileName, isDir, isExpanded, isSymlink)
  const sanitizedSvg = iconResult.svg
    ? DOMPurify.sanitize(iconResult.svg, {
        USE_PROFILES: { svg: true, svgFilters: true },
      })
    : null

  // A symlink wraps the icon, so the anchor moves to the wrapper: it is the box
  // the row lays out. `undefined` on the inner node keeps the id unique.
  const innerOracleId = isSymlink ? undefined : oracleId

  const renderIcon = () => {
    if (iconResult.component) {
      if (isValidElement(iconResult.component)) {
        return cloneElement(iconResult.component, {
          className,
          'data-oracle-id': innerOracleId,
        } as React.Attributes & {
          className: string
        })
      }
      return (
        <span className={className} data-oracle-id={innerOracleId}>
          {iconResult.component}
        </span>
      )
    }

    if (sanitizedSvg) {
      return (
        <span
          className={className}
          data-oracle-id={innerOracleId}
          style={{
            width: `${size}px`,
            height: `${size}px`,
            display: 'inline-block',
            lineHeight: 0,
          }}
          dangerouslySetInnerHTML={{ __html: sanitizedSvg }}
        />
      )
    }

    // Final fallback: Phosphor icon
    if (isDir) {
      const FolderIcon = isExpanded ? FolderOpen : Folder
      return (
        <FolderIcon
          size={size}
          className={className}
          weight="duotone"
          data-oracle-id={innerOracleId}
        />
      )
    }
    return (
      <File size={size} className={className} weight="duotone" data-oracle-id={innerOracleId} />
    )
  }

  if (isSymlink) {
    return (
      <span className="relative inline-block" data-oracle-id={oracleId}>
        {renderIcon()}
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

  return renderIcon()
}
