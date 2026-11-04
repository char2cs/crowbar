import type { NativePalette } from '@/lib/native-chrome'

const token = (style: CSSStyleDeclaration, name: string) => style.getPropertyValue(name).trim()

/** The current theme's palette as CSS gives it: the Settings page's surface and its cards. */
export function readNativePalette(): NativePalette {
  const style = getComputedStyle(document.documentElement)
  return {
    surface: token(style, '--background'),
    raised: token(style, '--muted'),
    text: token(style, '--foreground'),
    secondaryText: token(style, '--muted-foreground'),
  }
}
