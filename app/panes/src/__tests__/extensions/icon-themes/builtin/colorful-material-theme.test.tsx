import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { colorfulMaterialIconTheme } from '@/extensions/icon-themes/builtin/colorful-material-theme'
import { loadMaterialIcons } from '@/extensions/icon-themes/builtin/material-icons'

describe('colorfulMaterialIconTheme folder color', () => {
  it('colors a closed folder gold via the color prop, not className', () => {
    const { svg: _svg, component } = colorfulMaterialIconTheme.getFileIcon('src', true, false)
    const { container } = render(<>{component}</>)
    const svg = container.querySelector('svg')
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
