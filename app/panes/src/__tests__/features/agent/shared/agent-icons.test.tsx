import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { FileIcon, PencilIcon, PlusIcon } from '@/features/agent/shared/agent-icons'

afterEach(cleanup)

describe('PlusIcon / FileIcon / PencilIcon', () => {
  it('render on the shared 24-unit grid at the requested size', () => {
    const { container: plus } = render(<PlusIcon size={16} />)
    const plusSvg = plus.querySelector('svg')
    expect(plusSvg).toHaveAttribute('viewBox', '0 0 24 24')
    expect(plusSvg).toHaveAttribute('width', '16')

    const { container: file } = render(<FileIcon />)
    expect(file.querySelector('svg')).toHaveAttribute('width', '14')

    const { container: pencil } = render(<PencilIcon size={14} />)
    expect(pencil.querySelector('svg')).toHaveAttribute('width', '14')
  })

  it('draws distinct glyphs for each symbol', () => {
    const { container: plus } = render(<PlusIcon />)
    const { container: file } = render(<FileIcon />)
    const { container: pencil } = render(<PencilIcon />)

    const plusPaths = Array.from(plus.querySelectorAll('path')).map((p) => p.getAttribute('d'))
    const filePaths = Array.from(file.querySelectorAll('path')).map((p) => p.getAttribute('d'))
    const pencilPaths = Array.from(pencil.querySelectorAll('path')).map((p) => p.getAttribute('d'))

    expect(plusPaths.length).toBeGreaterThan(0)
    expect(filePaths.length).toBeGreaterThan(0)
    expect(pencilPaths.length).toBeGreaterThan(0)

    const all = [...plusPaths, ...filePaths, ...pencilPaths]
    expect(new Set(all).size).toBe(all.length)
  })
})
