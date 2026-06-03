import { Warning as AlertTriangle } from "@phosphor-icons/react";
import { Button } from "@/components/ui/button";
import { AppDialog as Dialog } from "@/components/ui/dialog";

interface AlertDialogState {
  title: string;
  message: string;
}

interface OpenAllFilesDialogState {
  filePaths: string[];
}

interface DeleteCandidateState {
  path: string;
  isDir: boolean;
}

interface FileExplorerDialogsProps {
  alertDialog: AlertDialogState | null;
  onCloseAlertDialog: () => void;
  openAllFilesDialog: OpenAllFilesDialogState | null;
  isOpeningAllFiles: boolean;
  onCloseOpenAllFilesDialog: () => void;
  onConfirmOpenAllFiles: () => void;
  deleteCandidate: DeleteCandidateState | null;
  isDeletingPath: boolean;
  onCloseDeleteDialog: () => void;
  onConfirmDelete: () => void;
  getPathBaseName: (path: string) => string;
}

export function FileExplorerDialogs({
  alertDialog,
  onCloseAlertDialog,
  openAllFilesDialog,
  isOpeningAllFiles,
  onCloseOpenAllFilesDialog,
  onConfirmOpenAllFiles,
  deleteCandidate,
  isDeletingPath,
  onCloseDeleteDialog,
  onConfirmDelete,
  getPathBaseName,
}: FileExplorerDialogsProps) {
  return (
    <>
      {alertDialog && (
        <Dialog
          title={alertDialog.title}
          icon={AlertTriangle}
          onClose={onCloseAlertDialog}
          footer={
            <Button onClick={onCloseAlertDialog} variant="ghost" compact>
              OK
            </Button>
          }
        >
          <p className="text-foreground ui-text-xs">{alertDialog.message}</p>
        </Dialog>
      )}
      {openAllFilesDialog && (
        <Dialog
          title="Open All Files"
          icon={AlertTriangle}
          onClose={() => {
            if (!isOpeningAllFiles) onCloseOpenAllFilesDialog();
          }}
          footer={
            <>
              <Button
                onClick={onCloseOpenAllFilesDialog}
                disabled={isOpeningAllFiles}
                variant="default"
              >
                Cancel
              </Button>
              <Button
                onClick={onConfirmOpenAllFiles}
                disabled={isOpeningAllFiles}
                variant="secondary"
              >
                {isOpeningAllFiles ? "Opening..." : "Open"}
              </Button>
            </>
          }
        >
          <p className="text-foreground ui-text-xs">
            {openAllFilesDialog.filePaths.length} files will be opened in tabs. Continue?
          </p>
        </Dialog>
      )}
      {deleteCandidate && (
        <Dialog
          title={deleteCandidate.isDir ? "Delete Folder" : "Delete File"}
          icon={AlertTriangle}
          onClose={() => {
            if (!isDeletingPath) onCloseDeleteDialog();
          }}
          footer={
            <>
              <Button
                onClick={onCloseDeleteDialog}
                disabled={isDeletingPath}
                variant="default"
                className="disabled:cursor-not-allowed disabled:opacity-50"
              >
                Cancel
              </Button>
              <Button
                onClick={onConfirmDelete}
                disabled={isDeletingPath}
                variant="destructive"
                className="disabled:cursor-not-allowed disabled:opacity-50"
              >
                {isDeletingPath ? "Deleting..." : "Delete"}
              </Button>
            </>
          }
        >
          <p className="text-foreground ui-text-xs">
            {deleteCandidate.isDir
              ? `Are you sure you want to delete the folder "${getPathBaseName(deleteCandidate.path)}" and all its contents? This action cannot be undone.`
              : `Are you sure you want to delete the file "${getPathBaseName(deleteCandidate.path)}"? This action cannot be undone.`}
          </p>
        </Dialog>
      )}
    </>
  );
}
