// Tauri invoke removed — linting is a no-op in Crowbar web mode
import { extensionRegistry } from "@/extensions/registry/extension-registry";
import { logger } from "@/features/editor/utils/logger";
import { useFileSystemStore } from "@/features/file-system/controllers/store";

export interface LintOptions {
  filePath: string;
  content: string;
  languageId?: string;
}

export interface Diagnostic {
  line: number;
  column: number;
  endLine?: number;
  endColumn?: number;
  severity: "error" | "warning" | "info" | "hint";
  message: string;
  code?: string;
  source?: string;
}

export interface LintResult {
  success: boolean;
  diagnostics?: Diagnostic[];
  error?: string;
}

/**
 * Lint content using the configured linter for the file type
 */
export async function lintContent(options: LintOptions): Promise<LintResult> {
  const { filePath, languageId } = options;

  try {
    // Try to get linter by file path first, then by language ID
    let linterConfig = extensionRegistry.getLinterForFile(filePath);

    if (!linterConfig && languageId) {
      linterConfig = extensionRegistry.getLinterForLanguage(languageId);
    }

    if (!linterConfig) {
      logger.debug("LinterService", `No linter configured for ${filePath}`);
      return {
        success: true,
        diagnostics: [],
      };
    }

    logger.debug("LinterService", `Linting ${filePath} with ${linterConfig.command}`);

    const _language = languageId || extensionRegistry.getLanguageId(filePath) || "unknown";

    // Linting via Tauri invoke is not available in Crowbar web mode — return empty result
    logger.debug("LinterService", `Linting is a no-op in web mode for ${filePath}`);
    return { success: true, diagnostics: [] };
  } catch (error) {
    logger.error("LinterService", `Failed to lint ${filePath}:`, error);
    return {
      success: false,
      error: error instanceof Error ? error.message : String(error),
      diagnostics: [],
    };
  }
}

/**
 * Check if linting is available for a file
 */
export function isLintingAvailable(filePath: string, languageId?: string): boolean {
  const linterConfig = extensionRegistry.getLinterForFile(filePath);
  if (linterConfig) return true;

  if (languageId) {
    const langLinterConfig = extensionRegistry.getLinterForLanguage(languageId);
    return langLinterConfig !== null;
  }

  return false;
}

/**
 * Get workspace folder from file path
 */
function _getWorkspaceFolder(filePath: string): string | undefined {
  const rootFolderPath = useFileSystemStore.getState().rootFolderPath;
  if (rootFolderPath) return rootFolderPath;

  // Fallback to file's directory if no project is open
  const lastSlash = filePath.lastIndexOf("/");
  if (lastSlash > 0) {
    return filePath.slice(0, lastSlash);
  }
  return undefined;
}
