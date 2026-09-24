import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { colorfulMaterialIconTheme } from '@/extensions/icon-themes/builtin/colorful-material-theme'
import { loadMaterialIcons } from '@/extensions/icon-themes/builtin/material-icons'

// Folders used to render as plain `text-muted-foreground` grey under this
// theme even though its file icons keep their original colors — the theme
// only ever returned `{ component: <Folder /> }` with no color of its own,
// so there was nothing for a caller's className override to lose. Folders
// should be colored like Athas's, independent of whatever className the
// caller (FileExplorerIcon) applies.
describe('colorfulMaterialIconTheme folder color', () => {
  it('colors a closed folder gold via the color prop, not className', () => {
    const { svg: _svg, component } = colorfulMaterialIconTheme.getFileIcon('src', true, false)
    const { container } = render(<>{component}</>)
    const svg = container.querySelector('svg')
    // Phosphor's IconBase turns the `color` prop into the svg's `fill`
    // attribute — a caller's later cloneElement({ className }) only ever
    // touches `className`, so this survives it.
    expect(svg).toHaveAttribute('fill', '#f2c14e')
  })

  it('colors an expanded folder gold too', () => {
    const { component } = colorfulMaterialIconTheme.getFileIcon('src', true, true)
    const { container } = render(<>{component}</>)
    const svg = container.querySelector('svg')
    expect(svg).toHaveAttribute('fill', '#f2c14e')
  })

  it('still keeps a file icon its own original-color svg, untouched', async () => {
    await loadMaterialIcons()
    const result = colorfulMaterialIconTheme.getFileIcon('README.md', false, false)
    expect(result.component).toBeUndefined()
    expect(result.svg).toBeTruthy()
  })
})
