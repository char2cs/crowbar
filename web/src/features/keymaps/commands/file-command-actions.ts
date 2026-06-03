import { readFileContent } from "@/features/file-system/controllers/file-operations"
import { getActiveWorkspaceStoreRef } from "@/features/workspace/stores/workspace-store-ref"

// Stub
export const FILE_COMMANDS: unknown[] = []
export function registerFileCommands(): void {}

export async function revertActiveFile(): Promise<void> {
  const wsStore = getActiveWorkspaceStoreRef()
  if (!wsStore) return
  const state = wsStore.getState()
  const activeBufferId = state.paneActions.getActivePane()?.activeBufferId ?? null
  const buffer = state.buffers.find(b => b.id === activeBufferId)
  if (!buffer || buffer.type !== 'editor' || buffer.isVirtual) return
  try {
    const content = await readFileContent(buffer.path)
    wsStore.setState((state) => ({
      ...state,
      buffers: state.buffers.map(b =>
        b.id === buffer.id && b.type === 'editor'
          ? { ...b, content, savedContent: content, isDirty: false }
          : b,
      ),
    }))
  } catch {
    // ignore read errors
  }
}
