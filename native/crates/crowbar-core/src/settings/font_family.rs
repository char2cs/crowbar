//! Font-family parse/normalize/resolve-against-available.
//!
//! Ported from `web/src/features/settings/lib/font-family-resolution.ts`,
//! all three exports. `fontFamily`/`terminalFontFamily`/`uiFontFamily` are
//! stored as full CSS font stacks (`"Geist Mono", Menlo, monospace`), not
//! single names — these functions extract/validate the primary (first)
//! family in that stack.
//!
//! [`resolve_available_font_family`]'s only current TS consumer
//! (`font-selector.tsx`) is CONDITIONAL (Settings dialog), but this whole
//! file is LIVE (`native/mapping/tier-a-denominator.md` §4:
//! `normalizeConfiguredFontFamily` is called directly from
//! `settings-normalization.ts`'s boot-time path) — ported at file
//! granularity, matching this crate's existing precedent
//! (`crate::git::git_diff_helpers`... — no, see `crate::keymap::chord`'s
//! module doc for the same "whole LIVE file, including exports whose only
//! current caller is CONDITIONAL" reasoning) rather than trimming to only
//! the boot-reachable export.

/// The first font family in a CSS stack, with surrounding whitespace and a
/// matched pair of leading/trailing quotes stripped.
///
/// Mirrors `getPrimaryFontFamily`. The TS source's `?.trim()` on the result
/// of `.split(',')[0]` can never actually be `undefined` (splitting any
/// string, including `""`, always yields a non-empty array), so the `?.` is
/// defensive TS boilerplate with no reachable `undefined` branch — ported as
/// a plain (non-`Option`) `&str` return.
#[must_use]
pub fn get_primary_font_family(font_family: &str) -> &str {
    let first = font_family.split(',').next().unwrap_or("");
    first.trim().trim_matches(|c| c == '\'' || c == '"')
}

/// Falls back to `fallback` when `font_family`'s primary family is empty
/// (e.g. `""`, `"   "`, or a stack starting with a bare comma); otherwise
/// returns `font_family` unchanged (the *whole* stack, not just the primary
/// family — matching the TS source exactly: this is a validity gate, not a
/// rewrite).
///
/// Mirrors `normalizeConfiguredFontFamily`.
#[must_use]
pub fn normalize_configured_font_family(font_family: &str, fallback: &str) -> String {
    if get_primary_font_family(font_family).is_empty() {
        return fallback.to_string();
    }

    font_family.to_string()
}

/// Resolves a configured font stack against a set of fonts actually present
/// on the system, case-insensitively. Falls back to `fallback` when the
/// configured stack is empty (via [`normalize_configured_font_family`]) or
/// when its primary family is not in `available_fonts`/`always_available_fonts`.
///
/// Mirrors `resolveAvailableFontFamily`.
#[must_use]
pub fn resolve_available_font_family<'a>(
    font_family: &'a str,
    fallback: &'a str,
    available_fonts: impl IntoIterator<Item = &'a str>,
    always_available_fonts: impl IntoIterator<Item = &'a str>,
) -> String {
    let normalized = normalize_configured_font_family(font_family, fallback);
    let primary = get_primary_font_family(&normalized);
    if primary.is_empty() {
        return fallback.to_string();
    }

    let available: std::collections::HashSet<String> = available_fonts
        .into_iter()
        .chain(always_available_fonts)
        .map(|family| family.trim().to_lowercase())
        .collect();

    if available.contains(&primary.to_lowercase()) {
        return normalized;
    }

    fallback.to_string()
}

#[cfg(test)]
mod tests {
    use super::{
        get_primary_font_family, normalize_configured_font_family, resolve_available_font_family,
    };

    // --- ported from web/src/__tests__/features/settings/font-family-resolution.test.ts ---

    #[test]
    fn extracts_the_primary_font_family_from_a_stack() {
        assert_eq!(
            get_primary_font_family("\"Geist Mono\", Menlo, monospace"),
            "Geist Mono"
        );
    }

    #[test]
    fn preserves_configured_font_names_that_may_exist_on_the_system() {
        assert_eq!(
            normalize_configured_font_family("Geist Mono", "JetBrains Mono Variable"),
            "Geist Mono"
        );
        assert_eq!(
            normalize_configured_font_family("Geist Sans", "CalSansUI"),
            "Geist Sans"
        );
    }

    #[test]
    fn falls_back_when_the_configured_font_is_empty() {
        assert_eq!(
            normalize_configured_font_family("   ", "JetBrains Mono Variable"),
            "JetBrains Mono Variable"
        );
    }

    #[test]
    fn falls_back_when_the_requested_font_is_unavailable() {
        assert_eq!(
            resolve_available_font_family(
                "\"Missing Mono\", Menlo, monospace",
                "JetBrains Mono Variable",
                ["menlo", "monaco"],
                ["JetBrains Mono Variable"],
            ),
            "JetBrains Mono Variable"
        );
    }

    #[test]
    fn keeps_custom_fonts_that_exist_on_the_system() {
        assert_eq!(
            resolve_available_font_family(
                "Berkeley Mono",
                "JetBrains Mono Variable",
                ["berkeley mono"],
                [],
            ),
            "Berkeley Mono"
        );
    }

    #[test]
    fn keeps_geist_mono_when_it_is_available_as_a_system_font() {
        assert_eq!(
            resolve_available_font_family(
                "Geist Mono",
                "JetBrains Mono Variable",
                ["geist mono"],
                [],
            ),
            "Geist Mono"
        );
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn get_primary_font_family_strips_single_quotes_too() {
        // The TS regex `/^['"]+|['"]+$/g` strips either quote character; the
        // ported TS suite only exercises the double-quote case above.
        assert_eq!(
            get_primary_font_family("'Fira Code', monospace"),
            "Fira Code"
        );
    }

    #[test]
    fn empty_stack_falls_back_in_resolve_available_font_family_too() {
        assert_eq!(
            resolve_available_font_family("", "JetBrains Mono Variable", ["menlo"], []),
            "JetBrains Mono Variable"
        );
    }

    #[test]
    fn an_empty_fallback_with_an_empty_configured_stack_resolves_to_the_empty_fallback() {
        // `normalize_configured_font_family("", "")` returns "" (its own
        // fallback), so `resolve_available_font_family`'s *second* empty
        // check — the one guarding a still-empty primary after that first
        // substitution — only fires when the fallback itself has no usable
        // primary family either. A degenerate case, but a real branch: not
        // exercised by the case above, where the fallback is non-empty.
        assert_eq!(resolve_available_font_family("", "", ["menlo"], []), "");
    }
}
