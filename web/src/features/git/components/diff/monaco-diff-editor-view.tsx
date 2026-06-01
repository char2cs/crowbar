import { memo, useEffect, useRef } from "react";
import { editor as monacoEditor, Uri } from "monaco-editor";
import { detectLanguageFromPath } from "@/features/editor/utils/language-detection";

interface MonacoDiffEditorViewProps {
  originalContent: string;
  modifiedContent: string;
  filePath: string;
  renderSideBySide: boolean;
  height: number | string;
  cacheKey: string;
  scrollable?: boolean;
}

function MonacoDiffEditorViewComponent({
  originalContent,
  modifiedContent,
  filePath,
  renderSideBySide,
  height,
  cacheKey,
  scrollable = false,
}: MonacoDiffEditorViewProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<monacoEditor.IStandaloneDiffEditor | null>(null);
  const language = detectLanguageFromPath(filePath);

  // Create the diff editor once per cacheKey; dispose on unmount or cacheKey change.
  // language and initial content are captured at mount time — cacheKey drives identity.
  useEffect(() => {
    if (!containerRef.current) return;

    const originalModel = monacoEditor.createModel(
      originalContent,
      language,
      Uri.parse(`diff-original://${cacheKey}`),
    );
    const modifiedModel = monacoEditor.createModel(
      modifiedContent,
      language,
      Uri.parse(`diff-modified://${cacheKey}`),
    );

    const diffEditor = monacoEditor.createDiffEditor(containerRef.current, {
      readOnly: true,
      renderSideBySide,
      scrollBeyondLastLine: false,
      minimap: { enabled: false },
      renderOverviewRuler: false,
      scrollbar: {
        vertical: scrollable ? "auto" : "hidden",
        horizontal: "auto",
        alwaysConsumeMouseWheel: scrollable,
      },
      lineNumbers: "on",
    });

    diffEditor.setModel({ original: originalModel, modified: modifiedModel });
    editorRef.current = diffEditor;

    return () => {
      editorRef.current = null;
      diffEditor.dispose();
      originalModel.dispose();
      modifiedModel.dispose();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cacheKey]);

  // Update model content without recreating the editor.
  useEffect(() => {
    const editor = editorRef.current;
    if (!editor) return;
    const models = editor.getModel();
    if (!models) return;
    if (models.original.getValue() !== originalContent) {
      models.original.setValue(originalContent);
    }
    if (models.modified.getValue() !== modifiedContent) {
      models.modified.setValue(modifiedContent);
    }
  }, [originalContent, modifiedContent]);

  // Toggle unified/split without a remount.
  useEffect(() => {
    editorRef.current?.updateOptions({ renderSideBySide });
  }, [renderSideBySide]);

  return (
    <div
      ref={containerRef}
      className="overflow-hidden"
      style={{ height: typeof height === "number" ? `${height}px` : height }}
    />
  );
}

export default memo(MonacoDiffEditorViewComponent);
