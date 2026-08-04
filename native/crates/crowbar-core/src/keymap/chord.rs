//! Pure chord parsing, formatting and matching.
//!
//! Ported from `web/src/features/keymaps/utils/chord.ts` — 4 of its 6 value
//! exports: [`parse_chord`], [`stringify_chord`], [`normalize_chord`],
//! [`format_chord`]. The other two, `chordFromEvent` and `eventMatchesChord`,
//! take a DOM `KeyboardEvent` directly and are **not ported**: the grammar
//! they call into (`parseChord`/`stringifyChord`) is portable, but the
//! event-field extraction (`e.metaKey`/`e.ctrlKey`/`e.shiftKey`/`e.altKey`/
//! `e.key`) is a reimplementation-at-the-boundary, not a port — GPUI
//! delivers its own `KeyDownEvent`/`Modifiers` shape (see the gpui skill).
//! That responsibility has to live wherever `crowbar-core`'s eventual GPUI
//! caller turns a `KeyDownEvent` into a chord string to feed
//! [`super::effective_keymaps::find_conflicting_commands`] or a dispatch
//! table — see `native/mapping/core-keymap.md` for the fuller account.
//!
//! **A seventh export, `MOD_ORDER`, is not ported either — but for a
//! different reason.** It has zero non-test importers anywhere in `web/src`
//! (confirmed by direct grep), including within `chord.ts` itself past its
//! own declaration. `tier-a-denominator.md` §3's "4 of 6" count does not
//! mention it at all (interfaces aren't counted as value exports, and this
//! one apparently fell out of the count too) — it is dead exported data in a
//! live file, the same shape as `hooks/use-command-shortcut.ts`'s stub next
//! to it in the same directory, and is skipped for the same reason: nothing
//! reaches it.
//!
//! Normalized chord string grammar (lowercase, `+`-separated, modifiers
//! first in canonical order): `[mod+][shift+][alt+]<key>`
//!  - `mod`   -> Cmd on macOS, Ctrl elsewhere
//!  - `shift` -> Shift
//!  - `alt`   -> Alt / Option
//!  - `<key>` -> a single key, lowercased (e.g. `t`, `\`, `arrowleft`)
//!
//! Examples: `mod+\`, `mod+shift+t`, `mod+alt+arrowleft`.
//!
//! # A shape difference: `formatChord` takes `is_mac` explicitly
//!
//! The TS `formatChord` reads a module-level `IS_MAC` constant
//! (`@/utils/platform`) to choose glyphs (`⌘⇧⌥`) vs. words (`Ctrl`/`Shift`/
//! `Alt`) and separators (`''` vs `'+'`). Its own test file has to
//! `vi.mock('@/utils/platform', ...)` to get deterministic non-mac output —
//! proof that the hidden global is a testing liability even in the source.
//! [`format_chord`] takes `is_mac: bool` as an explicit parameter instead:
//! it stays pure and needs no mocking, and the eventual GPUI call site (which
//! already has to know the current platform for its own key-event handling)
//! decides the value rather than this module owning a hidden fact about the
//! machine it happens to be running on.

/// Structured form of a parsed chord. `r#mod` (not `mod` — a Rust keyword)
/// mirrors the TS field name exactly.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct ParsedChord {
    pub r#mod: bool,
    pub shift: bool,
    pub alt: bool,
    pub key: String,
}

/// Mirrors `parseChord`. `None` for empty input or a bare modifier with no
/// key.
#[must_use]
pub fn parse_chord(chord: &str) -> Option<ParsedChord> {
    let lowered = chord.trim().to_lowercase();
    let parts: Vec<&str> = lowered
        .split('+')
        .map(str::trim)
        .filter(|p| !p.is_empty())
        .collect();
    if parts.is_empty() {
        return None;
    }

    let mut result = ParsedChord::default();
    for part in parts {
        match part {
            "mod" | "cmd" | "ctrl" | "control" | "meta" => result.r#mod = true,
            "shift" => result.shift = true,
            "alt" | "option" | "opt" => result.alt = true,
            // The last non-modifier token wins, matching the TS source's
            // plain reassignment in the loop — not documented as intentional
            // there, but a chord string only sensibly carries one key, so
            // this is unreachable in practice via any grammar-producing call
            // site (parseChord's own output always has exactly one).
            _ => result.key = part.to_string(),
        }
    }
    if result.key.is_empty() {
        return None;
    }
    Some(result)
}

/// Mirrors `stringifyChord`. Produces the canonical normalized chord string
/// from structured form.
#[must_use]
pub fn stringify_chord(parsed: &ParsedChord) -> String {
    let mut out: Vec<String> = Vec::with_capacity(4);
    if parsed.r#mod {
        out.push("mod".to_string());
    }
    if parsed.shift {
        out.push("shift".to_string());
    }
    if parsed.alt {
        out.push("alt".to_string());
    }
    // Defensively re-lowercased, matching the TS source: unlike parse_chord's
    // output (already all-lowercase), a hand-built ParsedChord passed
    // straight to this function is not guaranteed to be.
    out.push(parsed.key.to_lowercase());
    out.join("+")
}

/// Mirrors `normalizeChord`. Normalizes an arbitrary chord string to
/// canonical form (stable modifier order). Empty string for unparseable
/// input, matching the TS source's `parsed ? stringifyChord(parsed) : ''`.
#[must_use]
pub fn normalize_chord(chord: &str) -> String {
    parse_chord(chord)
        .map(|parsed| stringify_chord(&parsed))
        .unwrap_or_default()
}

/// Human-readable label for a chord's key half — the one piece of
/// `formatChord` cleanly worth its own named table (mirrors TS
/// `KEY_LABELS`).
fn key_label(key: &str) -> String {
    match key {
        "arrowleft" => "\u{2190}".to_string(),
        "arrowright" => "\u{2192}".to_string(),
        "arrowup" => "\u{2191}".to_string(),
        "arrowdown" => "\u{2193}".to_string(),
        "\\" => "\\".to_string(),
        other => other.to_uppercase(),
    }
}

/// Mirrors `formatChord`, with `is_mac` as an explicit parameter — see the
/// module doc. Empty string for an unparseable chord.
#[must_use]
pub fn format_chord(chord: &str, is_mac: bool) -> String {
    let Some(parsed) = parse_chord(chord) else {
        return String::new();
    };
    let mut parts: Vec<String> = Vec::with_capacity(4);
    if parsed.r#mod {
        parts.push(if is_mac { "\u{2318}" } else { "Ctrl" }.to_string());
    }
    if parsed.shift {
        parts.push(if is_mac { "\u{21e7}" } else { "Shift" }.to_string());
    }
    if parsed.alt {
        parts.push(if is_mac { "\u{2325}" } else { "Alt" }.to_string());
    }
    parts.push(key_label(&parsed.key));
    if is_mac {
        parts.concat()
    } else {
        parts.join("+")
    }
}

#[cfg(test)]
mod tests {
    use super::{ParsedChord, format_chord, normalize_chord, parse_chord, stringify_chord};

    // --- ported from web/src/__tests__/features/keymaps/chord.test.ts ---
    // (the TS suite force-mocks IS_MAC to false; the port's equivalent is
    // passing is_mac: false explicitly to format_chord/eventMatchesChord-
    // adjacent calls, see the module doc.)

    #[test]
    fn parses_modifiers_in_any_order_and_lowercases_the_key() {
        assert_eq!(
            parse_chord("Shift+Mod+T"),
            Some(ParsedChord {
                r#mod: true,
                shift: true,
                alt: false,
                key: "t".to_string(),
            })
        );
    }

    #[test]
    fn treats_cmd_ctrl_meta_and_option_opt_as_their_canonical_modifiers() {
        assert_eq!(
            parse_chord("cmd+\\"),
            Some(ParsedChord {
                r#mod: true,
                shift: false,
                alt: false,
                key: "\\".to_string(),
            })
        );
        assert_eq!(
            parse_chord("option+arrowleft"),
            Some(ParsedChord {
                r#mod: false,
                shift: false,
                alt: true,
                key: "arrowleft".to_string(),
            })
        );
    }

    #[test]
    fn returns_none_for_empty_input_or_a_bare_modifier_with_no_key() {
        assert_eq!(parse_chord(""), None);
        assert_eq!(parse_chord("mod+shift"), None);
    }

    #[test]
    fn normalizes_to_a_canonical_modifier_order_mod_shift_alt() {
        assert_eq!(normalize_chord("alt+shift+mod+k"), "mod+shift+alt+k");
        assert_eq!(
            stringify_chord(&ParsedChord {
                r#mod: true,
                shift: false,
                alt: true,
                key: "arrowright".to_string(),
            }),
            "mod+alt+arrowright"
        );
    }

    // eventMatchesChord/chordFromEvent are not ported (see module doc) so
    // the TS `describe('eventMatchesChord (non-mac: mod = ctrl)', ...)`
    // block has no equivalent here.

    #[test]
    fn renders_ctrl_shift_alt_with_arrow_glyphs_and_a_plus_separator_non_mac() {
        assert_eq!(format_chord("mod+shift+t", false), "Ctrl+Shift+T");
        assert_eq!(
            format_chord("mod+alt+arrowleft", false),
            "Ctrl+Alt+\u{2190}"
        );
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn returns_empty_string_for_an_unparseable_chord() {
        assert_eq!(normalize_chord(""), "");
        assert_eq!(format_chord("", false), "");
        assert_eq!(format_chord("shift+alt", false), "");
    }

    #[test]
    fn format_chord_uses_symbol_glyphs_and_no_separator_on_mac() {
        // The non-mac half is ported from the TS suite above; the mac half
        // has no TS test at all (only vi.mock(false) is exercised there),
        // so this is new coverage of a real branch (is_mac: true) rather
        // than a translation.
        assert_eq!(format_chord("mod+shift+t", true), "\u{2318}\u{21e7}T");
    }

    #[test]
    fn format_chord_on_mac_without_mod_omits_the_command_glyph() {
        // Every other mac-formatting test pairs is_mac: true with a chord
        // that includes mod, leaving "mod absent, is_mac: true" untested —
        // caught by this crate's coverage gate, not by inspection.
        assert_eq!(format_chord("shift+alt+t", true), "\u{21e7}\u{2325}T");
    }

    #[test]
    fn a_backslash_key_formats_as_itself() {
        assert_eq!(format_chord("mod+\\", false), "Ctrl+\\");
    }

    #[test]
    fn trailing_and_repeated_plus_separators_do_not_produce_phantom_modifiers() {
        // chord.ts's own `.filter(Boolean)` exists for exactly this: a
        // stray "++" must not parse as a key made of an empty string.
        assert_eq!(
            parse_chord("mod++k"),
            Some(ParsedChord {
                r#mod: true,
                shift: false,
                alt: false,
                key: "k".to_string(),
            })
        );
    }
}
