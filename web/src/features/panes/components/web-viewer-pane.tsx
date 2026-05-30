import { lazy, Suspense } from "react";

const WebViewer = lazy(() =>
  import("@/features/web-viewer/components/web-viewer").then((m) => ({
    default: m.WebViewer,
  })),
);

interface WebViewerPaneProps {
  url: string;
  bufferId: string;
  profileKey?: string;
  history?: string[];
  historyIndex?: number;
  isActive: boolean;
  isVisible?: boolean;
}

export function WebViewerPane({
  url,
  bufferId,
  profileKey,
  history,
  historyIndex,
  isActive,
  isVisible,
}: WebViewerPaneProps) {
  return (
    <Suspense fallback={null}>
      <WebViewer
        url={url}
        bufferId={bufferId}
        profileKey={profileKey}
        history={history}
        historyIndex={historyIndex}
        isActive={isActive}
        isVisible={isVisible}
      />
    </Suspense>
  );
}
