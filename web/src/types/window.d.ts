declare global {
  interface Window {
    __fileDragData?: {
      type: string;
      path?: string;
      name: string;
      isDir?: boolean;
      [key: string]: unknown;
    } | null;
    electron?: {
      shell: {
        showItemInFolder: (path: string) => void;
        openPath: (path: string) => Promise<void>;
        openExternal: (url: string) => Promise<void>;
      };
      ipcRenderer?: unknown;
    };
  }
}

export {};
