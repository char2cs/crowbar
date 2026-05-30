import { useCallback, useState } from "react";
import { findFileInTree } from "@/features/file-system/controllers/file-tree-utils";
import { useFileTreeStore } from "@/features/file-explorer/stores/file-explorer-tree-store";
import type { FileEntry } from "@/features/file-system/types/app";
import { getDirName, joinPath, stripTrailingPathSeparators } from "@/utils/path-helpers";

interface UseFileExplorerInlineEditingProps {
  files: FileEntry[];
  rootFolderPath?: string;
  onUpdateFiles?: (files: FileEntry[]) => void;
  onRenamePath?: (path: string, newName?: string) => void;
  onCreateNewFileInDirectory: (directoryPath: string, fileName: string) => void | string | Promise<string | undefined>;
  onCreateNewFolderInDirectory?: (directoryPath: string, folderName: string) => void;
  showAlertDialog: (title: string, message: string) => void;
}

export function useFileExplorerInlineEditing({
  files,
  rootFolderPath,
  onUpdateFiles,
  onRenamePath,
  onCreateNewFileInDirectory,
  onCreateNewFolderInDirectory,
  showAlertDialog,
}: UseFileExplorerInlineEditingProps) {
  const [editingValue, setEditingValue] = useState("");

  const startInlineEditing = useCallback(
    (parentPath: string, isFolder: boolean) => {
      if (!onUpdateFiles) return;

      const newItem: FileEntry = {
        name: "",
        path: `${parentPath}/`,
        isDir: isFolder,
        isEditing: true,
        isNewItem: true,
      };

      const addNewItemToTree = (items: FileEntry[], targetPath: string): FileEntry[] =>
        items.map((item) => {
          if (item.path === targetPath && item.isDir) {
            return { ...item, children: [...(item.children || []), newItem] };
          }
          if (item.children) {
            return { ...item, children: addNewItemToTree(item.children, targetPath) };
          }
          return item;
        });

      if (parentPath === getDirName(files[0]?.path ?? "") || !parentPath) {
        onUpdateFiles([...files, newItem]);
      } else {
        onUpdateFiles(addNewItemToTree(files, parentPath));
      }

      try {
        const current = useFileTreeStore.getState().getExpandedPaths();
        const next = new Set(current);
        next.add(parentPath);
        useFileTreeStore.getState().setExpandedPaths(next);
      } catch {}

      setEditingValue("");
    },
    [files, onUpdateFiles],
  );

  const finishInlineEditing = useCallback(
    (item: FileEntry, newName: string) => {
      if (!onUpdateFiles) return;

      if (newName.trim()) {
        let parentPath = stripTrailingPathSeparators(item.path);
        if (!parentPath && rootFolderPath) parentPath = rootFolderPath;

        if (!parentPath) {
          showAlertDialog("Cannot Create File", "Cannot determine where to create the file.");
          return;
        }

        if (item.isRenaming) {
          onRenamePath?.(item.path, newName.trim());
          return;
        }

        if (item.isDir) {
          const folder = findFileInTree(files, joinPath(parentPath, newName.trim()));
          if (folder) {
            showAlertDialog("Folder Already Exists", "A folder with this name already exists.");
            return;
          }
          onCreateNewFolderInDirectory?.(parentPath, newName.trim());
        } else {
          const file = findFileInTree(files, joinPath(parentPath, newName.trim()));
          if (file) {
            showAlertDialog("File Already Exists", "A file with this name already exists.");
            return;
          }
          onCreateNewFileInDirectory(parentPath, newName.trim());
        }
      }

      const removeNewItemFromTree = (items: FileEntry[]): FileEntry[] =>
        items
          .filter((i) => !(i.isNewItem && i.isEditing))
          .map((i) => ({ ...i, children: i.children ? removeNewItemFromTree(i.children) : undefined }));

      onUpdateFiles(removeNewItemFromTree(files));
      setEditingValue("");
    },
    [files, onUpdateFiles, rootFolderPath, showAlertDialog, onRenamePath, onCreateNewFileInDirectory, onCreateNewFolderInDirectory],
  );

  const cancelInlineEditing = useCallback(
    (file: FileEntry) => {
      if (!onUpdateFiles) return;

      if (file.isRenaming) {
        onRenamePath?.(file.path);
        return;
      }

      const removeNewItemFromTree = (items: FileEntry[]): FileEntry[] =>
        items
          .filter((i) => !(i.isNewItem && i.isEditing))
          .map((i) => ({ ...i, children: i.children ? removeNewItemFromTree(i.children) : undefined }));

      onUpdateFiles(removeNewItemFromTree(files));
      setEditingValue("");
    },
    [files, onUpdateFiles, onRenamePath],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent, file: FileEntry) => {
      if (e.key === "Enter") {
        e.preventDefault();
        e.stopPropagation();
        finishInlineEditing(file, editingValue);
      } else if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        cancelInlineEditing(file);
      }
    },
    [editingValue, finishInlineEditing, cancelInlineEditing],
  );

  const handleBlur = useCallback(
    (file: FileEntry) => {
      if (editingValue.trim()) finishInlineEditing(file, editingValue);
      else cancelInlineEditing(file);
    },
    [editingValue, finishInlineEditing, cancelInlineEditing],
  );

  return {
    editingValue,
    setEditingValue,
    startInlineEditing,
    handleKeyDown,
    handleBlur,
  };
}
