import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { minimalIconTheme } from '@/extensions/icon-themes/builtin/minimal-theme'
import { iconThemeRegistry } from '@/extensions/icon-themes/icon-theme-registry'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import { useSettingsStore } from '@/features/settings/store'

describe('FileExplorerIcon', () => {
  afterEach(() => iconThemeRegistry.unregisterTheme(minimalIconTheme.id))

  it('renders the active theme icon at the requested size', () => {
    iconThemeRegistry.registerTheme(minimalIconTheme)
    useSettingsStore.setState((s) => ({ settings: { ...s.settings, iconTheme: 'minimal' } }))

    const { container } = render(<FileExplorerIcon fileName="main.go" size={28} />)

    const svg = container.querySelector('svg')
    expect(svg?.getAttribute('width')).toBe('28')
    expect(svg?.getAttribute('height')).toBe('28')
  })

  it('badges a symlink', () => {
    render(<FileExplorerIcon fileName="link" isSymlink />)

    expect(screen.getByRole('img', { name: 'Symlink' })).toBeTruthy()
  })

  it('draws a plain icon for a non-symlink', () => {
    render(<FileExplorerIcon fileName="plain.txt" />)

    expect(screen.queryByRole('img', { name: 'Symlink' })).toBeNull()
  })
})
