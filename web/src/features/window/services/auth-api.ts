// Stub: auth API is out of scope for this session.
export async function updateCollaborationChannelNote(): Promise<void> {}
export async function getAuthToken(): Promise<string | null> { return null }
export async function refreshAuthToken(): Promise<string | null> { return null }
export type CloudSettingsSyncSnapshot = {
  settings: Record<string, unknown>
  version: number
  updatedAt?: string
  schemaVersion?: number
}
export async function fetchSettingsSyncSnapshot(): Promise<CloudSettingsSyncSnapshot | null> { return null }
export async function pushSettingsSyncSnapshot(_snapshot: CloudSettingsSyncSnapshot): Promise<CloudSettingsSyncSnapshot> {
  return { settings: {}, version: 0, updatedAt: new Date().toISOString() }
}
export function isAuthInvalidError(_error: unknown): boolean { return false }
