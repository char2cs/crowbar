declare global {
  interface Window {
    electron?: {
      shell: {
        showItemInFolder: (path: string) => void
        openPath: (path: string) => Promise<void>
        openExternal: (url: string) => Promise<void>
      }
      ipcRenderer?: unknown
    }
  }
}

export {}
