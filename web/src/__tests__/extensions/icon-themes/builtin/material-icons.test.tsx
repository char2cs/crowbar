import { act, render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { iconThemeRegistry } from '@/extensions/icon-themes/icon-theme-registry'
import { materialIconTheme } from '@/extensions/icon-themes/builtin/material-theme'
import { loadMaterialIcons } from '@/extensions/icon-themes/builtin/material-icons'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import { useSettingsStore } from '@/features/settings/store'

// material-file-icons is ~500 KB of SVG source: it loads on first use, not at
// boot. Until it lands the theme returns nothing (callers draw a fallback);
// when it lands the registry announces a change and icons re-render.
describe('material icon theme, lazily loaded', () => {
  it('returns nothing before the icon set loads, then the material svg — and re-renders mounted icons', async () => {
    iconThemeRegistry.registerTheme(materialIconTheme)
    useSettingsStore.setState((s) => ({ settings: { ...s.settings, iconTheme: 'material' } }))

    expect(materialIconTheme.getFileIcon('main.go', false)).toEqual({})
    const { container } = render(<FileExplorerIcon fileName="main.go" />)
    const fallback = container.innerHTML

    await act(() => loadMaterialIcons())

    expect(materialIconTheme.getFileIcon('main.go', false).svg).toContain('<svg')
    expect(container.innerHTML).not.toBe(fallback)
    expect(container.querySelector('span svg')).not.toBeNull()
  })
})
