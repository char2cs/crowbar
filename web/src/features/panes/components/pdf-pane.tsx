import { lazy, Suspense } from "react";

const PdfViewer = lazy(() =>
  import("@/features/pdf-viewer/components/pdf-viewer").then((m) => ({
    default: m.PdfViewer,
  })),
);

interface PdfPaneProps {
  filePath: string;
  fileName: string;
  bufferId: string;
}

export function PdfPane({ filePath, fileName, bufferId }: PdfPaneProps) {
  return (
    <Suspense fallback={null}>
      <PdfViewer filePath={filePath} fileName={fileName} bufferId={bufferId} />
    </Suspense>
  );
}
