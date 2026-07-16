import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { joinPath } from '@/utils/path-helpers'
import { createFileNode, writeFileContent } from './file-tree-api'

// Present the OS file picker and return the chosen File objects. Resolves to an
// empty array when the user cancels. The <input> is not attached to the DOM —
// modern browsers fire `change`/`cancel` on a detached input clicked
// synchronously from a user gesture.
function pickFiles(): Promise<File[]> {
  return new Promise((resolve) => {
    const input = document.createElement('input')
    input.type = 'file'
    input.multiple = true
    input.onchange = () => resolve(input.files ? Array.from(input.files) : [])
    input.oncancel = () => resolve([])
    input.click()
  })
}

// Read a File's bytes as a base64 string. FileReader.readAsDataURL yields a
// "data:<mime>;base64,<payload>" URL; we return just the payload. Going through
// bytes (not file.text()) is what keeps binary uploads byte-faithful — decoding
// the file as UTF-8 text and re-encoding would mangle any non-UTF-8 byte.
export function readFileAsBase64(file: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const result = reader.result
      if (typeof result !== 'string') {
        reject(new Error('file-upload: unexpected FileReader result'))
        return
      }
      const comma = result.indexOf(',')
      resolve(comma === -1 ? '' : result.slice(comma + 1))
    }
    reader.onerror = () => reject(reader.error ?? new Error('file-upload: read failed'))
    reader.readAsDataURL(file)
  })
}

// Upload picked files into `directoryPath` (workspace-relative; '' is the root)
// via the daemon's create + write-content endpoints. Files are sent base64
// (encoding: 'base64') so their exact bytes land on disk — text and binary
// alike survive intact. Returns the workspace-relative paths that were written.
export async function pickAndUploadFiles(directoryPath: string): Promise<string[]> {
  const wsId = getActiveWorkspaceId()
  if (!wsId) return []

  const files = await pickFiles()
  const written: string[] = []
  for (const file of files) {
    // react-doctor-disable-next-line async-await-in-loop -- sequential upload keeps per-file create-then-write ordering and matches the daemon's serialised FS mutations. Cold path (manual picker), not hot.
    const content = await readFileAsBase64(file)
    const dest = directoryPath ? joinPath(directoryPath, file.name) : file.name
    await createFileNode(wsId, dest, 'file')
    await writeFileContent(wsId, dest, content, 'base64')
    written.push(dest)
  }
  return written
}
