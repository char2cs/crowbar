//! `--surface keybinding` — one keycap over a parsed shortcut, and the first
//! surface whose `empty` cell **emits no snapshot on purpose**.
//!
//! The default cell is the tab bar's close-button tooltip, measured live at a
//! 1714px viewport: `37.844 × 16`, `bg #1f1f1eff`, `fg #a4a4a4ff`, `radius 8`,
//! **`border.w 1`**, `CalSansUI` 12/12 at weight 400, painting `⌘W`. See
//! `native/mapping/keybinding.md` and `/tmp/p3-ref-keybinding.json`.
//!
//! # `empty` is real, and the honest outcome is **no snapshot at all**
//!
//! `keybinding.tsx` returns `null` for an empty legend, so the DOM has no
//! element and `ANCHORS.md` v1.11 says there is no anchor. This surface's root
//! *is* that anchor, so an `empty` cell records nothing and the binary refuses:
//!
//! ```text
//! $ CROWBAR_ROW_SNAPSHOT=… crowbar-app --surface keybinding --flags empty
//! crowbar-app: no snapshot: the root anchor "keybinding" was not recorded
//!              this frame; the anchors that were: []
//! ```
//!
//! **That refusal is the correct result and not a defect.** The reference emits
//! nothing for the same cell, so the two sides agree; what they agree on is that
//! there is nothing to compare. The alternative — synthesising a zero-rect
//! anchor here — would be writing the reference's own output into the port,
//! which is the repair v1.11 explicitly rejects.
//!
//! The flag is therefore **modelled rather than declared unmodelled**: it is a
//! real branch of a real component, and a surface that hid it would be quieter
//! and less true.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--content` | **real** — the cap has no `min-w-*` floor at all, so its width is its legend's advance width plus 14px of padding and border, and the three lengths move it by 40px end to end |
//! | `--theme` | **real**: `bg-card`, `border-border` and `text-muted-foreground` are all different tokens in the two tables |
//! | `--width` | **vacuous.** Nothing here is a percentage or a stretch |
//! | `--viewport-width` | **vacuous.** `keybinding.tsx` contains no `sm:` variant at all |
//!
//! # The state axis
//!
//! Five of the six are unmodelled, and the reason is `kbd`'s: **`keybinding.tsx`
//! has no interaction rule of any kind.** No `hover:`, no `focus`, no `data-[…]`
//! and no `disabled:` — the whole class list is one `cva` with no variants.
//! `empty` is the exception, above.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::keybinding::{self, Keybinding, Platform, Source};
use crowbar_ui::components::{AnchorSink, ContentLength};
use gpui::{AnyElement, IntoElement as _, ParentElement as _, SharedString, Styled as _, div};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "keybinding",
    root: keybinding::ID_KEYBINDING,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        // `keybinding.tsx` has no interaction rule of any kind — see the module
        // docs.
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // A 16px cap and `CAPTION_HEIGHT`'s 29. 72 holds both with room, and is a
    // floor rather than a ceiling: this surface drives no height, `min-h-4`
    // authoring the cap's.
    min_window_height: 72,
    // A keycap sits inside a tooltip — never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// `--binding`'s default: the live call site's own `shortcut="mod+w"`.
pub const DEFAULT_BINDING: &str = "mod+w";

/// This surface's own options.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Params {
    /// `--keys <a,b,…>`: the literal `keys` prop, taken without parsing.
    ///
    /// `None` selects the `binding` prop, which is what every **reachable** call
    /// site passes. `tab-context-menu.tsx` builds a `keys` node and
    /// `context-menu.tsx` never renders it — see [`Keybinding`]'s docs — so this
    /// arm has no reference.
    pub keys: Option<Vec<SharedString>>,
    /// `--platform`: which spelling. Only `mac` has a reference, because
    /// `platform.ts` returns `'macos'` on every path that has a `window`.
    pub platform: Platform,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            keys: None,
            platform: Platform::Mac,
        }
    }
}

impl Params {
    /// The cap this cell describes.
    ///
    /// `empty` empties it, whichever prop is driving: §8.3's word is "a surface
    /// with nothing in it", and both props can express that.
    #[must_use]
    pub fn keybinding(&self, cell: &Cell) -> Keybinding {
        let empty = cell.has(StateFlag::Empty);
        let source = match (&self.keys, empty) {
            (_, true) => Source::Keys(Vec::new()),
            (Some(keys), false) => Source::Keys(keys.clone()),
            (None, false) => Source::Binding(binding_of(cell.content)),
        };
        Keybinding {
            source,
            platform: self.platform,
        }
    }
}

/// The binding a content length shows.
///
/// A translation rather than a shared type, for `kbd`'s reason: the cell's
/// vocabulary and the component's carry different strings on purpose. `mod+w` is
/// the captured one; the other two straddle it, and with no `min-w-*` floor to
/// bind against, all three are genuinely different widths.
fn binding_of(content: ContentLength) -> SharedString {
    match content {
        // One cap, one glyph — the narrowest legend the component can paint.
        ContentLength::Short => SharedString::new_static("w"),
        // The captured cell: `⌘W`.
        ContentLength::Normal => SharedString::new_static(DEFAULT_BINDING),
        // Three caps, one of them a whole word — `⌘⇧Backspace`.
        ContentLength::Overflow => SharedString::new_static("mod+shift+backspace"),
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--keys" => {
                self.keys = Some(
                    value(args, option)?
                        .split(',')
                        .filter(|key| !key.is_empty())
                        .map(|key| SharedString::from(key.to_owned()))
                        .collect(),
                );
            }
            "--platform" => self.platform = parse_platform(&value(args, option)?)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** A keycap's height is `min-h-4` over a 12px line box, and no
    /// option here moves it.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let keybinding = self.keybinding(cell);
        match keybinding.label() {
            None => out.push_str(
                " · empty: the component returns null, so there is no anchor and no snapshot \
                 (ANCHORS.md v1.11) — which is what the reference does too",
            ),
            Some(label) => {
                let _ = write!(out, " · legend \"{label}\"");
            }
        }
        if self.keys.is_some() {
            out.push_str(
                " · keys: context-menu.tsx declares this prop and never renders it, \
                 so this cell has no reference",
            );
        }
        if self.platform == Platform::Other {
            out.push_str(
                " · platform other: platform.ts always reports macos in a webview, \
                 so this cell has no reference",
            );
        }
    }

    /// The cap, inside the flex row that makes it a flex item.
    ///
    /// `RowSurface` draws every surface into a gpui **block** container, and a
    /// cap drawn straight into one would be a block-level flex box. The live cap
    /// is a flex item of the tooltip's `flex items-center gap-2` — which is also
    /// why the reference's computed `display` is `flex` rather than
    /// `inline-flex`, CSS having blockified it. The row carries no anchor, so it
    /// cannot reach a snapshot.
    ///
    /// An empty legend renders the row and nothing in it, which is how the
    /// binary comes to refuse the cell for want of a root — see the module docs.
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let row = div().flex().flex_row().items_center();
        match self.keybinding(cell).render_root(theme, anchors) {
            Some(cap) => row.child(cap).into_any_element(),
            None => row.into_any_element(),
        }
    }
}

/// `--platform`'s closed vocabulary.
fn parse_platform(raw: &str) -> Result<Platform, ParseError> {
    match raw {
        "mac" => Ok(Platform::Mac),
        "other" => Ok(Platform::Other),
        other => Err(ParseError::Rejected(format!(
            "--platform takes mac or other, not {other}",
        ))),
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--keys <a,b,…>".to_owned(),
            "the literal `keys` prop, unparsed; context-menu.tsx never renders \
             the one live call site that passes it, so it has no reference"
                .to_owned(),
        ),
        (
            "--platform <mac|other>".to_owned(),
            "which spelling the separator and the modifier glyphs take; \
             platform.ts always reports macos in a webview [mac]"
                .to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_BINDING, Params, SURFACE, binding_of, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crowbar_ui::components::ContentLength;
    use crowbar_ui::components::keybinding::{Platform, Source};

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "keybinding"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a keybinding cell carries this surface's bag")
    }

    /// The default is the live tooltip cap: `mod+w`, painting `⌘W`.
    #[test]
    fn the_default_is_the_live_close_tab_cap() {
        let bag = Params::default();
        assert_eq!(bag.keys, None);
        assert_eq!(bag.platform, Platform::Mac);
        assert_eq!(binding_of(ContentLength::Normal), DEFAULT_BINDING);

        let cell = cell(&[]);
        let keybinding = params_of(&cell).keybinding(&cell);
        assert_eq!(
            keybinding.source,
            Source::Binding(DEFAULT_BINDING.to_owned().into()),
        );
        assert_eq!(
            keybinding.label().map(|l| l.to_string()),
            Some("\u{2318}W".into()),
        );
    }

    /// **`--content` is real here**, and more so than on `kbd`: there is no
    /// `min-w-*` floor, so all three lengths are genuinely different widths.
    #[test]
    fn every_content_length_paints_a_different_legend() {
        let mut seen = Vec::new();
        for length in ["short", "normal", "overflow"] {
            let cell = cell(&["--content", length]);
            let label = params_of(&cell)
                .keybinding(&cell)
                .label()
                .expect("a non-empty legend")
                .to_string();
            assert!(!seen.contains(&label), "{length} repeats {label}");
            seen.push(label);
        }
        assert_eq!(seen, vec!["W", "\u{2318}W", "\u{2318}\u{21e7}Backspace"]);
        // And they are strictly increasing in cap count, which is what makes the
        // axis move the box rather than only the string.
        assert!(seen[0].chars().count() < seen[1].chars().count());
        assert!(seen[1].chars().count() < seen[2].chars().count());
    }

    /// **`empty` produces no legend and therefore no anchor** — `ANCHORS.md`
    /// v1.11 — and it reaches that through *both* props, so neither spelling can
    /// smuggle a box in.
    #[test]
    fn the_empty_flag_leaves_nothing_to_anchor() {
        for args in [
            vec!["--flags", "empty"],
            vec!["--flags", "empty", "--keys", "Cmd,W"],
            vec!["--flags", "empty", "--content", "overflow"],
        ] {
            let cell = cell(&args);
            assert_eq!(params_of(&cell).keybinding(&cell).label(), None, "{args:?}");
            assert!(cell.describe().contains("no anchor and no snapshot"));
        }
        // And it is modelled rather than declared away.
        assert!(!SURFACE.unmodelled(StateFlag::Empty));
    }

    /// The other five are declared unmodelled, which is what makes the binary
    /// say so on stderr rather than drawing a cell that cannot fail.
    #[test]
    fn the_five_interaction_flags_are_declared_unmodelled() {
        for flag in [
            StateFlag::Loading,
            StateFlag::Error,
            StateFlag::Hover,
            StateFlag::Focus,
            StateFlag::Selected,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
            assert!(
                cell(&["--flags", flag.name()])
                    .unmodelled_flags()
                    .contains(&flag),
                "{flag:?} should be reported on stderr",
            );
        }
    }

    /// `--platform other` is a **different picture**, not a second spelling of
    /// the same one: it moves the glyphs and the separator at once.
    #[test]
    fn the_other_platform_moves_the_glyphs_and_the_separator() {
        let mac = cell(&[]);
        let other = cell(&["--platform", "other"]);

        assert_eq!(
            params_of(&other)
                .keybinding(&other)
                .label()
                .map(|l| l.to_string()),
            Some("Ctrl+W".into()),
        );
        assert_ne!(
            params_of(&mac).keybinding(&mac).label(),
            params_of(&other).keybinding(&other).label(),
        );
        assert!(other.describe().contains("no reference"));
    }

    /// `--keys` is taken literally, and the cell says it has no reference.
    #[test]
    fn the_keys_arm_is_literal_and_declared_unreachable() {
        let cell = cell(&["--keys", "Cmd,W"]);
        assert_eq!(
            params_of(&cell)
                .keybinding(&cell)
                .label()
                .map(|l| l.to_string()),
            Some("CmdW".into()),
        );
        assert!(cell.describe().contains("no reference"));

        // An all-empty list is the empty legend by another route: `split(',')`
        // over `",,"` is three blanks, and the filter drops every one.
        let blank = super::tests::cell(&["--keys", ",,"]);
        assert_eq!(params_of(&blank).keybinding(&blank).label(), None);
    }

    /// The vocabulary is closed and every rejection names what was wanted.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--platform", "windows"],
            vec!["--platform"],
            vec!["--keys"],
            vec!["--binding", "mod+w"],
        ] {
            let mut full = vec!["--surface", "keybinding"];
            full.extend_from_slice(&line);
            assert!(
                matches!(
                    Cell::parse(full.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }
    }

    /// **These options belong to this surface and to no other**, which is the
    /// property the registry exists for.
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--keys", "--platform"] {
            let line = ["--surface", "git-status-row", option, "mac"];
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{option} should not be a git-status-row option",
            );
        }
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();

        for option in ["--keys", "--platform"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("keybinding"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The registry entry's two contract fields, which a snapshot carries
    /// verbatim.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "keybinding");
        assert_eq!(SURFACE.root, "keybinding");
    }
}
