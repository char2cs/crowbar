import { currentPlatform } from '@/utils/platform'

export function paintPaneSurface(
  body: HTMLElement,
  typeId: string,
  platform: typeof currentPlatform = currentPlatform,
): void {
  const editorView = ['core.editor', 'core.review'].includes(typeId)
  const chrome = platform === 'macos' ? 'transparent' : 'var(--pane-chrome-bg)'
  body.style.backgroundColor = editorView ? 'var(--pane-background)' : chrome
}
