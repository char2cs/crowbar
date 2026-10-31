import { afterEach, describe, expect, it } from 'vitest'
import { readNativePalette } from '@/lib/native-palette'

function styleSheet(css: string) {
  const style = document.createElement('style')
  style.textContent = css
  document.head.append(style)
  return style
}

const PAGE =
  ':root { --background: #fcfcfa; --muted: #f1f0ea; --foreground: #1b1b19; --muted-foreground: #666666; --brand-light: #606c38; --brand-dark: #98a86a; }'
const CHROME_SURFACE =
  '.chrome-surface { --foreground: #f2f0ea; --primary: #98a86a; --primary-foreground: #1b1b19; }'
// Every theme.css theme defines the raised controls (Frosted: `var(--background)`), so they
// cannot tell a dark sidebar from a plain one.
const CONTROLS = ':root, .chrome-surface { --control-surface: var(--background); }'

afterEach(() => {
  document.head.replaceChildren()
})

describe('readNativePalette', () => {
  it('reads the page palette from the root', () => {
    styleSheet(PAGE)
    expect(readNativePalette()).toMatchObject({
      surface: '#fcfcfa',
      raised: '#f1f0ea',
      text: '#1b1b19',
      secondaryText: '#666666',
      brandLight: '#606c38',
      brandDark: '#98a86a',
    })
  })

  it('reads the sidebar palette from the chrome surface, with the raised control colours', () => {
    styleSheet(PAGE + CHROME_SURFACE + CONTROLS)
    expect(readNativePalette().sidebar).toEqual({
      text: '#f2f0ea',
      brand: '#98a86a',
      brandText: '#1b1b19',
    })
  })

  it('has no sidebar palette where the sidebar keeps the plain chrome, as in Frosted light', () => {
    styleSheet(PAGE + CONTROLS)
    expect(readNativePalette().sidebar).toBeNull()
  })

  it('sends every colour as one the phone parses, whatever CSS function the theme wrote it in', () => {
    styleSheet(PAGE + ':root { --muted: hsl(60 20% 50%); --muted-foreground: rebeccapurple; }')
    expect(readNativePalette()).toMatchObject({ raised: '#999966', secondaryText: '#663399' })
  })

  it('leaves no probe in the document', () => {
    styleSheet(PAGE + CHROME_SURFACE + CONTROLS)
    readNativePalette()
    expect(document.querySelector('.chrome-surface')).toBeNull()
  })
})
