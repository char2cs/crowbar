// localStorage holds per-viewer conveniences only. Storage that throws (private
// mode, blocked site data, a full quota) reads as absent and writes as not written.

/** The stored value, or null when it is absent or storage is unavailable. */
export function readLocal(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

/** Whether the value was stored. */
export function writeLocal(key: string, value: string): boolean {
  try {
    localStorage.setItem(key, value)
    return true
  } catch {
    return false
  }
}

/** Removes the value; storage that is unavailable holds nothing to remove. */
export function removeLocal(key: string): void {
  try {
    localStorage.removeItem(key)
  } catch {
    return
  }
}
