// Stub
export function isRemotePath(_path: string): boolean { return false }
export function parseRemotePath(_path: string): { host: string; path: string } | null { return null }
export function buildRemotePath(_host: string, _path: string): string { return "" }
export interface RemoteConnection { connectionId: string; host: string; path: string }
