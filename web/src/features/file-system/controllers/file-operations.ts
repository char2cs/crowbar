// Stub: file-system feature is out of scope for this session.
export async function readFileContent(_path: string): Promise<string> {
  return ''
}

export async function writeFileContent(_path: string, _content: string): Promise<void> {}

export async function deleteFile(_path: string): Promise<void> {}

export async function renameFile(_oldPath: string, _newPath: string): Promise<void> {}

export async function createDirectory(_path: string): Promise<void> {}
