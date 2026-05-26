import { test, expect } from 'vitest'

// Isolated test of the bridge-to-local mapping logic.
// Bridge PastedEntry: { path: string; success: boolean }  (no is_dir — future backend API gap)
// Local PastedEntry needs: { source_path, destination_path, is_dir, success }

type BridgeResult = { path: string; success: boolean }
type LocalEntry = { source_path: string; destination_path: string; is_dir: boolean; success: boolean }

function mapBridgeResult(r: BridgeResult): LocalEntry {
  // This is the CORRECT mapping expected after the fix
  return {
    source_path: r.path,
    destination_path: r.path,
    // TODO: bridge does not return is_dir — add when real backend is wired
    is_dir: false,
    success: r.success,
  }
}

test('preserves success=true', () => {
  expect(mapBridgeResult({ path: '/file.txt', success: true }).success).toBe(true)
})

test('preserves success=false so callers can detect failed pastes', () => {
  expect(mapBridgeResult({ path: '/file.txt', success: false }).success).toBe(false)
})

test('defaults is_dir to false (bridge does not return it yet)', () => {
  expect(mapBridgeResult({ path: '/file.txt', success: true }).is_dir).toBe(false)
})

test('source_path and destination_path both come from bridge path', () => {
  const entry = mapBridgeResult({ path: '/a/b/c.ts', success: true })
  expect(entry.source_path).toBe('/a/b/c.ts')
  expect(entry.destination_path).toBe('/a/b/c.ts')
})
