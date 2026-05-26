import { IS_MAC, normalizeKey } from "@/utils/platform";

const MODIFIER_KEYS = ["ctrl", "cmd", "alt", "shift", "meta"];
const SPECIAL_KEYS: Record<string, string> = {
  enter: "Enter",
  return: "Enter",
  tab: "Tab",
  space: " ",
  backspace: "Backspace",
  delete: "Delete",
  escape: "Escape",
  esc: "Escape",
  up: "ArrowUp",
  down: "ArrowDown",
  left: "ArrowLeft",
  right: "ArrowRight",
  pageup: "PageUp",
  pagedown: "PageDown",
  home: "Home",
  end: "End",
  insert: "Insert",
};

interface ParsedKey {
  modifiers: string[];
  key: string;
}

interface ParsedKeybinding {
  parts: ParsedKey[];
  isChord: boolean;
}

function parseKeyCombination(combo: string): ParsedKey {
  const parts = combo
    .toLowerCase()
    .split("+")
    .map((p) => p.trim());
  const modifiers: string[] = [];
  let key = "";

  for (const part of parts) {
    if (MODIFIER_KEYS.includes(part)) {
      if (!modifiers.includes(part)) {
        modifiers.push(part);
      }
    } else {
      key = part;
    }
  }

  // Normalize special keys
  if (SPECIAL_KEYS[key]) {
    key = SPECIAL_KEYS[key];
  }

  // Sort modifiers for consistent comparison
  modifiers.sort();

  return { modifiers, key };
}

function parseKeybinding(binding: string): ParsedKeybinding {
  // Normalize for platform (cmd -> ctrl on Windows/Linux)
  const normalized = normalizeKey(binding);

  // Split by space to detect chords
  const parts = normalized
    .split(/\s+/)
    .filter((p) => p.length > 0)
    .map(parseKeyCombination);

  return {
    parts,
    isChord: parts.length > 1,
  };
}

function formatModifier(modifier: string) {
  if (modifier === "cmd" && IS_MAC) return "⌘";
  if (modifier === "cmd") return "Ctrl";
  if (modifier === "ctrl") return "Ctrl";
  if (modifier === "alt") return IS_MAC ? "⌥" : "Alt";
  if (modifier === "shift") return IS_MAC ? "⇧" : "Shift";
  if (modifier === "meta") return IS_MAC ? "⌘" : "Meta";
  return modifier;
}

function formatKey(key: string) {
  if (key === " ") return "Space";
  if (key.startsWith("Arrow")) return key.replace("Arrow", "");
  if (key.length === 1) return key.toUpperCase();
  if (/^f\d{1,2}$/.test(key)) return key.toUpperCase();
  return key;
}

export function keybindingToDisplay(binding: string): string[] {
  const parsed = parseKeybinding(binding.replace(/\bmod\b/gi, "cmd"));
  const keys: string[] = [];

  for (const part of parsed.parts) {
    keys.push(...part.modifiers.map(formatModifier));
    if (part.key) {
      keys.push(formatKey(part.key));
    }
  }

  return keys;
}
