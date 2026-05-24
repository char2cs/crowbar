import { readFileContent } from "@/features/file-system/controllers/file-operations"
import { useBufferStore } from "@/features/editor/stores/buffer-store"

// Stub
export const FILE_COMMANDS: unknown[] = []
export function registerFileCommands(): void {}

export async function revertActiveFile(): Promise<void> {
  const { activeBufferId, buffers } = useBufferStore.getState()
  const buffer = buffers.find(b => b.id === activeBufferId)
  if (!buffer || buffer.type !== 'editor' || buffer.isVirtual) return
  try {
    const content = await readFileContent(buffer.path)
    useBufferStore.setState({
      buffers: buffers.map(b =>
        b.id === buffer.id
          ? { ...b, content, savedContent: content, isDirty: false }
          : b,
      ),
    })
  } catch {
    // ignore read errors
  }
}
