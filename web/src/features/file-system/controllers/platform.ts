// Stub: file-system platform controllers are out of scope for this session.
export async function openFile(): Promise<string | null> {
  return null
}

export async function openDirectory(): Promise<string | null> {
  return null
}

export async function writeFile(_path: string, _content: string): Promise<void> {
  console.warn('writeFile is a no-op in Crowbar web mode')
}

export async function readFile(_path: string): Promise<string> {
  return ""
}

export async function readDirectory(_path: string): Promise<Array<{ name: string; path: string; isDirectory: boolean; is_dir: boolean; isFile: boolean }>> {
  return []
}

export async function exists(_path: string): Promise<boolean> {
  return false
}

export function getHomePath(): string {
  return "/home"
}

export function getSeparator(): string {
  return "/"
}

export async function moveFile(_src: string, _dest: string): Promise<void> {}
export async function copyFile(_src: string, _dest: string): Promise<void> {}
export async function deleteFile(_path: string): Promise<void> {}
export async function createDirectory(_path: string): Promise<void> {}
