/**
 * A browser `DataTransfer` never carries a real host filesystem path — no
 * browser exposes one on a `File`, dev-mode or not. On the Tauri desktop
 * build this function is not even reachable for a real OS file drop: Tauri's
 * webview intercepts that before a DOM `drop` event fires at all (see
 * `useTauriFileDrop`, `features/file-system/lib/tauri-file-drop.ts`, for the
 * real path-yielding mechanism). This stays `[]` because that is the honest
 * answer for what a `DataTransfer` alone can ever provide, not because the
 * feature is unimplemented — the real implementation lives in the hook.
 */
export function extractDroppedFilePaths(_dataTransfer: DataTransfer): string[] {
  return []
}
