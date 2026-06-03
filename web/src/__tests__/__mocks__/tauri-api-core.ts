// Stub for @tauri-apps/api/core used in the test environment.
// The real module is only available inside a Tauri desktop shell.
export async function invoke(_cmd: string, _args?: Record<string, unknown>): Promise<void> {
  // no-op in tests
}
