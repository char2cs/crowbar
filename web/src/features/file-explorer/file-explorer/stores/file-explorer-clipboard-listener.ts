// Tauri event listeners replaced with no-ops (Crowbar web mode)
// FUTURE: subscribe to Go backend WebSocket events for clipboard changes

export async function initializeFileClipboardListener(): Promise<void> {
  // no-op in web mode
}

export async function cleanupFileClipboardListener(): Promise<void> {
  // no-op in web mode
}
