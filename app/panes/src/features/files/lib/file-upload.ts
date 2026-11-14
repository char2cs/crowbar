import { workspaceRef, writeDocument } from '@/lib/api/documents'
import { joinPath } from '@/utils/path-helpers'
import { createFileNode } from './file-tree-api'

// Show the OS file picker; resolves to [] on cancel. The <input> stays detached from the DOM:
// browsers fire `change`/`cancel` on a detached input clicked synchronously from a user gesture.
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

// A File's bytes as base64 (the payload of readAsDataURL's data: URL). Reading bytes rather than
// file.text() keeps binary uploads byte-faithful; a UTF-8 round trip mangles non-UTF-8 bytes.
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

// Upload picked files into `directoryPath` ('' is the workspace root) via the daemon's create
// and document-write endpoints, as base64 so the exact bytes land on disk. Returns the
// workspace-relative paths written.
export async function pickAndUploadFiles(wsId: string, directoryPath: string): Promise<string[]> {
  const files = await pickFiles()
  const written: string[] = []
  for (const file of files) {
    // react-doctor-disable-next-line async-await-in-loop -- sequential upload keeps per-file create-then-write ordering and matches the daemon's serialised FS mutations. Cold path (manual picker), not hot.
    const content = await readFileAsBase64(file)
    const dest = directoryPath ? joinPath(directoryPath, file.name) : file.name
    await createFileNode(wsId, dest, 'file')
    await writeDocument(workspaceRef(wsId, dest), content, { encoding: 'base64' })
    written.push(dest)
  }
  return written
}
