import { lazy, Suspense } from "react";

const ImageViewer = lazy(() =>
  import("@/features/image-viewer/components/image-viewer").then((m) => ({
    default: m.ImageViewer,
  })),
);

interface ImagePaneProps {
  filePath: string;
  fileName: string;
  bufferId: string;
}

export function ImagePane({ filePath, fileName, bufferId }: ImagePaneProps) {
  return (
    <Suspense fallback={null}>
      <ImageViewer filePath={filePath} fileName={fileName} bufferId={bufferId} />
    </Suspense>
  );
}
