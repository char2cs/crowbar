import { test, expect } from 'vitest'

type BridgeResult = { path: string; success: boolean }
type LocalEntry = {
  source_path: string
  destination_path: string
  is_dir: boolean
  success: boolean
}

function mapBridgeResult(r: BridgeResult): LocalEntry {
  return {
    source_path: r.path,
    destination_path: r.path,
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
