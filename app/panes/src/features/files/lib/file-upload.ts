import { workspaceRef, writeDocument } from '@/lib/api/documents'
import { joinPath } from '@/utils/path-helpers'
import { createFileNode } from './file-tree-api'

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

function readFileAsBase64(file: Blob): Promise<string> {
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
