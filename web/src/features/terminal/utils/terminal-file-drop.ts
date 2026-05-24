function quoteTerminalPath(path: string): string {
  const escaped = path.replace(/(["\\$`])/g, "\\$1");
  return /[\s"'\\$`]/.test(path) ? `"${escaped}"` : escaped;
}

export function formatDroppedPathsForTerminal(rawPaths: string[]): string {
  if (rawPaths.length === 0) return "";
  return `${rawPaths.map(quoteTerminalPath).join(" ")} `;
}
