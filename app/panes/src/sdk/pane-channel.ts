export interface PaneChannel {
  post(text: string): void
  onmessage: ((text: string) => void) | null
}
