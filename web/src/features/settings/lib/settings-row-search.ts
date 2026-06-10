import type { ReactNode } from "react";

/**
 * Case-insensitive match of the settings search query against a setting row's
 * label and (string) description. An empty/whitespace query matches everything.
 */
export function settingRowMatchesQuery(
  query: string,
  label: string,
  description?: ReactNode,
): boolean {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return true;
  if (label.toLowerCase().includes(normalized)) return true;
  return typeof description === "string" && description.toLowerCase().includes(normalized);
}
