// Crowbar web mode — use browser clipboard API directly
export async function readEditorClipboardText(): Promise<string> {
  return navigator.clipboard.readText()
}

export async function writeEditorClipboardText(text: string): Promise<void> {
  await navigator.clipboard.writeText(text)
}
