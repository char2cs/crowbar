//! `keybinding` — one keycap over a parsed shortcut, and the **third** reading
//! of the border trap in three components.
//!
//! The native half of `web/src/components/ui/keybinding.tsx`, which renders a
//! single `<kbd>` whose legend is `keybindingToDisplay(binding)` or a literal
//! `keys` array, joined by `''` on macOS and `'+'` elsewhere — and which
//! **returns `null` when the list is empty**.
//!
//! # Why this is not [`Kbd`](super::kbd::Kbd), measured rather than assumed
//!
//! `kbd` is already ported and the instruction was to reuse it. **Every box
//! property differs**, so reuse would have meant restyling `kbd` into something
//! neither component is. Both class lists were compiled by the app's own
//! Tailwind and both keycaps were then measured on the live app in the same
//! frame — the command palette's `Esc` cap and the tab bar's `⌘W` tooltip cap:
//!
//! | | `kbd.tsx` | `keybinding.tsx` |
//! |---|---|---|
//! | background | `bg-muted` → `#ffffff0a` | `bg-card` → `#1f1f1eff` |
//! | border | **none** → `0px` | **`border`** → **`1px`** `#ffffff0f` |
//! | radius | `rounded` → `4` | `rounded-md` → `8` |
//! | inline padding | `px-1` → `4` | `px-1.5` → `6` |
//! | height | `h-5` → authored `20` | `min-h-4` → floored `16` |
//! | width floor | `min-w-5` → `20` | **none** |
//! | weight | `font-medium` → 500 | (unset) → **400** |
//! | line box | `text-xs` → `12/16` | `leading-none` → `12/12` |
//!
//! Eight rows, eight differences. Sharing a shell would have required eight
//! parameters over a two-field struct, which is a second component wearing the
//! first one's name. **What is shared is the [`TypeStep`] vocabulary** and the
//! rule the table's second row states, below.
//!
//! # `border.w` is **1** here, and `kbd`'s is **0** — the trap in both directions
//!
//! `native/MAPPING.md` records `border` as "measure, never infer" because it is
//! 1px on some primitives and 0 on others. These two are the sharpest pair yet:
//! **the same element name, in the same directory, one module apart.**
//! Preflight sets `border: 0 solid` on every element; `kbd.tsx` never puts it
//! back and `keybinding.tsx` does, with a bare `border`.
//!
//! Measured live on the `⌘W` cap: `borderTopWidth: "1px"`,
//! `borderTopColor: "oklch(1 0 0 / 0.06)"` — the `--border` token. `ANCHORS.md`
//! v1.1 compares `border.w` **exactly**, so a port that carried `kbd`'s 0 across
//! would fail every cell by a whole pixel on each of four edges.
//!
//! # Not `line_sized` — `badge`'s precedent, a third time
//!
//! `min-h-4` floors the box at 16px around a `leading-none` **12px** line box.
//! `ANCHORS.md` v1.6 is explicit that the test is whether the height is
//! *derived from* the line box, not whether the element paints text; declaring
//! it here would compare 16 against 12 and manufacture a **4px delta on the
//! surface's only anchor**. Measured: `bounds.h 16`, `line-height 12px`.
//!
//! # The empty list has **no anchor**, which is `ANCHORS.md` v1.11
//!
//! `if (displayKeys.length === 0) return null` — React renders no element, so
//! the DOM has no box, so v1.11 says there is no anchor rather than a zero-sized
//! one. [`Keybinding::render`] returns [`Option::None`] for that case rather
//! than an empty box, which is the only shape that keeps the two sides agreeing:
//! a native zero-rect anchor against a reference that emits nothing is a
//! structural delta caused by the port.
//!
//! # The platform branch is **ported, not resolved**
//!
//! The separator is `''` on macOS and `'+'` elsewhere, and `keybindingToDisplay`
//! branches on the same flag six more times (`⌘`/`Ctrl`, `⌥`/`Alt`, `⇧`/`Shift`,
//! `⌘`/`Meta`, and `normalizeKey`'s `cmd`→`ctrl` rewrite). [`Platform`] carries
//! it so both arms are expressible and both are tested.
//!
//! **A reading worth recording:** `web/src/utils/platform.ts`'s `detectPlatform`
//! returns the literal `'macos'` on every path that has a `window`, so the
//! running webview's `IS_MAC` is **unconditionally true** and only
//! [`Platform::Mac`] has a live reference. The other arm is modelled because the
//! source branches, not because a capture reaches it.

use gpui::{AnyElement, Div, FontWeight, ParentElement as _, Pixels, Rems, SharedString};
use gpui::{Styled as _, div, px, relative};

use super::badge::TypeStep;
use crate::anchor::{AnchorId, AnchorSink};
use crate::theme::Theme;

/// The keycap anchor — the only one this surface carries.
pub const ID_KEYBINDING: &str = "keybinding";

/// The cap's used width is its legend's advance width plus `px-1.5` and two
/// borders, with **no `min-w-*` floor at all** — so it is content-sized in the
/// purest form `ANCHORS.md` v1.5 describes.
///
/// Confirmed against the live cap: advance `23.843` + 2×6 padding + 2×1 border
/// = `37.843`, and the reference's box is `37.844`.
pub const CONTENT_SIZED: [&str; 1] = [ID_KEYBINDING];

/// **Nothing.** `min-h-4` floors the height at 16 around a 12px line box — see
/// the module docs.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `min-h-4` — `calc(var(--spacing) * 4)`, measured live as `min-height: 16px`.
///
/// A **floor**, not an authored height: the box is the taller of this and the
/// content, and on every live cap the floor binds because `leading-none` puts
/// the line box at 12.
pub const MIN_HEIGHT: Pixels = px(SPACING * 4.0);

/// `px-1.5` — `calc(var(--spacing) * 1.5)`, measured live as
/// `padding-inline: 6px`. There is no block padding: `paddingTop` and
/// `paddingBottom` both read `0px`.
pub const PADDING_X: Pixels = px(SPACING * 1.5);

/// `border` — a **real 1px border**, where [`super::kbd`]'s is 0.
///
/// See the module docs: this is the same trap `popover` records against
/// `dropdown_menu`, one module apart instead of two, and `ANCHORS.md` v1.1
/// compares this field exactly.
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `ui-text-sm` at `leading-none` — 12px text in a **12px** line box.
///
/// `--ui-text-sm` is `0.75rem`, which is the same 12px as `kbd`'s `text-xs`;
/// the line boxes are what differ, and that difference is the whole reason
/// neither component is `line_sized` for the same arithmetic.
pub const TYPE_STEP: TypeStep = TypeStep {
    size: Rems(0.75),
    line_height: 1.0,
};

/// The cap paints at the stylesheet's default weight — **400**, measured live.
///
/// `kbd.tsx` carries `font-medium`; this class list carries no weight utility at
/// all, so the `<kbd>` inherits the document's. Named explicitly because a port
/// that reached for `kbd`'s `MEDIUM` would move `font.weight`, which
/// `ANCHORS.md` §5 compares **exactly**.
pub const WEIGHT: FontWeight = FontWeight::NORMAL;

/// Which platform's spelling a cap uses.
///
/// A vocabulary rather than a `bool` so both arms name themselves at the call
/// site: `Platform::Mac` reads as a fact where `true` reads as a flag.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Platform {
    /// `IS_MAC` — the only arm the running webview can produce.
    #[default]
    Mac,
    /// Windows and Linux, which `platform.ts` treats identically.
    Other,
}

impl Platform {
    /// What `displayKeys.join(IS_MAC ? '' : '+')` puts between two caps' worth
    /// of legend.
    #[must_use]
    pub const fn separator(self) -> &'static str {
        match self {
            Self::Mac => "",
            Self::Other => "+",
        }
    }

    /// Whether this is the macOS arm.
    #[must_use]
    pub const fn is_mac(self) -> bool {
        matches!(self, Self::Mac)
    }
}

/// `MODIFIER_KEYS` — the five words `parseKeyCombination` treats as modifiers.
const MODIFIER_KEYS: [&str; 5] = ["ctrl", "cmd", "alt", "shift", "meta"];

/// `SPECIAL_KEYS` — the normalisation table, in the source's own order.
const SPECIAL_KEYS: [(&str, &str); 18] = [
    ("enter", "Enter"),
    ("return", "Enter"),
    ("tab", "Tab"),
    ("space", " "),
    ("backspace", "Backspace"),
    ("delete", "Delete"),
    ("escape", "Escape"),
    ("esc", "Escape"),
    ("up", "ArrowUp"),
    ("down", "ArrowDown"),
    ("left", "ArrowLeft"),
    ("right", "ArrowRight"),
    ("pageup", "PageUp"),
    ("pagedown", "PageDown"),
    ("home", "Home"),
    ("end", "End"),
    ("insert", "Insert"),
    // `formatKey`'s own inverse for `space`, which the table above maps to a
    // literal blank: the display step turns it back into the word.
    (" ", " "),
];

/// `normalizeKey` — `cmd` → `ctrl` off macOS, whole words only.
fn normalize_key(binding: &str, platform: Platform) -> String {
    if platform.is_mac() {
        return binding.to_owned();
    }
    replace_word(binding, "cmd", "ctrl")
}

/// `binding.replace(/\bmod\b/gi, 'cmd')` and `normalizeKey`'s `\bcmd\b` share
/// one implementation, because they are the same rewrite with different words.
///
/// "Word" is JavaScript's `\b`: a boundary between `[A-Za-z0-9_]` and anything
/// else. Case-insensitive, as both regexes are (`gi` / `gi`).
fn replace_word(haystack: &str, needle: &str, replacement: &str) -> String {
    let lower = haystack.to_lowercase();
    let mut out = String::with_capacity(haystack.len());
    let mut cursor = 0;
    while let Some(hit) = lower[cursor..].find(needle) {
        let start = cursor + hit;
        let end = start + needle.len();
        let before_is_word = lower[..start]
            .chars()
            .next_back()
            .is_some_and(|c| c.is_alphanumeric() || c == '_');
        let after_is_word = lower[end..]
            .chars()
            .next()
            .is_some_and(|c| c.is_alphanumeric() || c == '_');
        if before_is_word || after_is_word {
            out.push_str(&haystack[cursor..end]);
        } else {
            out.push_str(&haystack[cursor..start]);
            out.push_str(replacement);
        }
        cursor = end;
    }
    out.push_str(&haystack[cursor..]);
    out
}

/// `formatModifier`.
fn format_modifier(modifier: &str, platform: Platform) -> String {
    let mac = platform.is_mac();
    match modifier {
        "cmd" if mac => "\u{2318}".to_owned(),
        "cmd" | "ctrl" => "Ctrl".to_owned(),
        "alt" => if mac { "\u{2325}" } else { "Alt" }.to_owned(),
        "shift" => if mac { "\u{21e7}" } else { "Shift" }.to_owned(),
        "meta" => if mac { "\u{2318}" } else { "Meta" }.to_owned(),
        other => other.to_owned(),
    }
}

/// `formatKey`.
fn format_key(key: &str) -> String {
    if key == " " {
        return "Space".to_owned();
    }
    if let Some(rest) = key.strip_prefix("Arrow") {
        return rest.to_owned();
    }
    if key.chars().count() == 1 {
        return key.to_uppercase();
    }
    if is_function_key(key) {
        return key.to_uppercase();
    }
    key.to_owned()
}

/// `/^f\d{1,2}$/` — `f` then one or two digits, lowercase `f` only, as the
/// regex is written without the `i` flag.
fn is_function_key(key: &str) -> bool {
    let Some(digits) = key.strip_prefix('f') else {
        return false;
    };
    (1..=2).contains(&digits.len()) && digits.bytes().all(|b| b.is_ascii_digit())
}

/// `parseKeyCombination` — one `+`-separated combo into sorted modifiers and a
/// key.
fn parse_combination(combo: &str) -> (Vec<String>, String) {
    let mut modifiers: Vec<String> = Vec::new();
    let mut key = String::new();

    for part in combo.to_lowercase().split('+') {
        let part = part.trim();
        if MODIFIER_KEYS.contains(&part) {
            if !modifiers.iter().any(|held| held == part) {
                modifiers.push(part.to_owned());
            }
        } else {
            key.clear();
            key.push_str(part);
        }
    }

    if let Some((_, normalised)) = SPECIAL_KEYS.iter().find(|(raw, _)| *raw == key) {
        key = (*normalised).to_owned();
    }

    // `modifiers.sort()` — JavaScript's default sort is by UTF-16 code unit,
    // which for these five ASCII words is the same order as Rust's byte sort.
    modifiers.sort_unstable();
    (modifiers, key)
}

/// `keybindingToDisplay` — the legend a `binding` string paints, cap by cap.
#[must_use]
pub fn keybinding_to_display(binding: &str, platform: Platform) -> Vec<SharedString> {
    let normalised = normalize_key(&replace_word(binding, "mod", "cmd"), platform);
    let mut keys: Vec<SharedString> = Vec::new();

    for combo in normalised.split_whitespace() {
        let (modifiers, key) = parse_combination(combo);
        keys.extend(
            modifiers
                .iter()
                .map(|modifier| SharedString::from(format_modifier(modifier, platform))),
        );
        if !key.is_empty() {
            keys.push(SharedString::from(format_key(&key)));
        }
    }

    keys
}

/// Where a cap's legend comes from — the component's two mutually exclusive
/// props.
///
/// `binding` wins when both are given (`binding ? keybindingToDisplay(binding)
/// : (keys ?? [])`), which this type makes unrepresentable rather than
/// re-deciding at render time.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum Source {
    /// `binding` — parsed through [`keybinding_to_display`]. The live cap is
    /// `mod+w`, which the reachable call site (`button.tsx`'s tooltip on the tab
    /// bar's close button) passes.
    Binding(SharedString),
    /// `keys` — taken literally, never parsed. `tab-context-menu.tsx` builds
    /// `['Cmd', 'W']` this way; see [`Keybinding`] for why that call site is
    /// unreachable.
    Keys(Vec<SharedString>),
}

/// One `<Keybinding>`.
///
/// # The `keys` arm is real but **dead**, and that is a finding
///
/// `tab-context-menu.tsx` builds `keybinding: <Keybinding keys={closeKeys} />`,
/// and `context-menu.tsx` declares `keybinding?: React.ReactNode` on its item
/// type — **and never renders it**. Only `item.shortcut` reaches the DOM. So the
/// node is constructed on every context menu and mounted by nothing.
///
/// The arm is modelled because the component genuinely has it, and named as
/// dead rather than quietly rendered — the call `popover` made about its
/// `tooltipStyle` variant.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Keybinding {
    /// Where the legend comes from.
    pub source: Source,
    /// Which platform's spelling. Only [`Platform::Mac`] has a reference — see
    /// the module docs.
    pub platform: Platform,
}

impl Keybinding {
    /// The captured cell: the tab bar's close-button tooltip at a 1714px
    /// viewport — `binding="mod+w"`, painting `⌘W` in a `37.844 × 16` box.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            source: Source::Binding(SharedString::new_static("mod+w")),
            platform: Platform::Mac,
        }
    }

    /// `displayKeys` — the caps this binding resolves to, before joining.
    #[must_use]
    pub fn display_keys(&self) -> Vec<SharedString> {
        match &self.source {
            Source::Binding(binding) => keybinding_to_display(binding, self.platform),
            Source::Keys(keys) => keys.clone(),
        }
    }

    /// The string the `<kbd>` paints, or [`None`] where the component returns
    /// `null`.
    ///
    /// The separator is the platform's, which is the one piece of this
    /// component's behaviour that is neither a length nor a colour: `⌘W` on
    /// macOS against `Ctrl+W` everywhere else.
    #[must_use]
    pub fn label(&self) -> Option<SharedString> {
        let keys = self.display_keys();
        if keys.is_empty() {
            return None;
        }
        Some(SharedString::from(
            keys.iter()
                .map(SharedString::as_ref)
                .collect::<Vec<_>>()
                .join(self.platform.separator()),
        ))
    }

    /// The keycap's own box.
    ///
    /// `inline-flex` becomes `.flex()` for `kbd`'s reason: gpui has no inline
    /// flow, and the live computed `display` is **`flex`** anyway, because the
    /// cap is a flex item of the tooltip's `flex items-center gap-2` and CSS
    /// blockifies those.
    ///
    /// `font-sans` is not in this class list, but `ui-font` is, and the live cap
    /// resolves to **`CalSansUI`** — the same family. It is named explicitly
    /// because `ANCHORS.md` v1.2 ruling 5 makes `font.family` the *declared*
    /// first family on both sides, and an inherited `.SystemUIFont` is a string
    /// the DOM will never produce.
    fn shell(theme: &Theme) -> Div {
        let family = theme.font_sans.primary().unwrap_or("sans-serif");
        div()
            .font_family(family)
            .flex()
            .items_center()
            .justify_center()
            .min_h(MIN_HEIGHT)
            .px(PADDING_X)
            .rounded(theme.radius_md.value())
            .border(BORDER_WIDTH)
            .border_color(theme.border)
            .bg(theme.card)
            .text_size(TYPE_STEP.size)
            .line_height(relative(TYPE_STEP.line_height))
            .font_weight(WEIGHT)
            .text_color(theme.muted_foreground)
    }

    /// The element and its one anchor, or [`None`] where the legend is empty.
    ///
    /// **`None` rather than an empty box**: `ANCHORS.md` v1.11 makes an element
    /// that generates no box not an anchor, and React returns `null` here — so
    /// a zero-rect anchor on this side would be a structural delta the port
    /// invented.
    ///
    /// `shadow-[inset_0_-1px_0_rgba(0,0,0,0.12)]` is **not painted**, and that
    /// is a deliberate omission rather than an oversight. It is `ANCHORS.md` §6
    /// material — no field on either side — and gpui has no inset-shadow preset,
    /// so painting it would mean minting `rgba(0,0,0,0.12)` outside
    /// `crate::theme`, which `scripts/check-invariants.sh` rule 4 fails the
    /// build on. The design system has no token for it either. `popover` makes
    /// the same call about its `before:` inset shadow.
    ///
    /// It is *not* a `ring`, so it does not interact with `border.w` — which is
    /// a real 1 here, and the one field this omission must not be confused with.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Option<AnyElement> {
        let label = self.label()?;
        let id = AnchorId::from(ID_KEYBINDING).content_sized();
        Some(anchors.boxed_text(id, Self::shell(theme), label))
    }

    /// [`Keybinding::render`] as a snapshot **root**, which is what the surface
    /// needs: this cap is the whole surface, so its anchor is the frame
    /// boundary rather than a box inside one.
    #[must_use]
    pub fn render_root(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Option<AnyElement> {
        let label = self.label()?;
        let id = AnchorId::from(ID_KEYBINDING).content_sized();
        let run = anchors.text_half(&id, label);
        Some(anchors.root(id.clone(), Self::shell(theme).child(run)))
    }
}

#[cfg(test)]
mod tests {
    use super::{
        BORDER_WIDTH, CONTENT_SIZED, ID_KEYBINDING, Keybinding, LINE_SIZED, MIN_HEIGHT, PADDING_X,
        Platform, Source, TYPE_STEP, WEIGHT, keybinding_to_display,
    };
    use crate::theme::Theme;
    use gpui::{FontWeight, Rems, SharedString, px};

    fn keys(binding: &str, platform: Platform) -> Vec<String> {
        keybinding_to_display(binding, platform)
            .into_iter()
            .map(|key| key.to_string())
            .collect()
    }

    /// **The captured cell**: `mod+w` paints `⌘W` on macOS and `Ctrl+W`
    /// everywhere else, and the two differ in *both* the glyph and the
    /// separator.
    #[test]
    fn the_live_binding_paints_the_captured_legend_on_both_platforms() {
        let mac = Keybinding::fixture();
        assert_eq!(mac.label().map(|l| l.to_string()), Some("\u{2318}W".into()));

        let other = Keybinding {
            platform: Platform::Other,
            ..Keybinding::fixture()
        };
        assert_eq!(other.label().map(|l| l.to_string()), Some("Ctrl+W".into()));

        // And the difference is the platform branch rather than a different
        // parse: the caps are the same count either way.
        assert_eq!(mac.display_keys().len(), other.display_keys().len());
    }

    /// **The separator is the behaviour, and it is not hardcoded to the mac
    /// result.** A control: the two arms disagree on a two-cap legend and agree
    /// on a one-cap one, which is exactly what a join can do.
    #[test]
    fn the_separator_is_the_platforms_and_only_shows_between_caps() {
        assert_eq!(Platform::Mac.separator(), "");
        assert_eq!(Platform::Other.separator(), "+");
        assert_eq!(Platform::default(), Platform::Mac);

        let one = |platform| {
            Keybinding {
                source: Source::Keys(vec![SharedString::new_static("W")]),
                platform,
            }
            .label()
            .map(|l| l.to_string())
        };
        assert_eq!(one(Platform::Mac), one(Platform::Other));
        assert_eq!(one(Platform::Mac), Some("W".into()));

        let three = |platform| {
            Keybinding {
                source: Source::Binding(SharedString::new_static("mod+shift+k")),
                platform,
            }
            .label()
            .map(|l| l.to_string())
        };
        // `cmd` sorts before `shift`, so the glyphs come out ⌘ then ⇧ whatever
        // order the binding string spells them in — see the sort test below.
        assert_eq!(three(Platform::Mac), Some("\u{2318}\u{21e7}K".into()));
        assert_eq!(three(Platform::Other), Some("Ctrl+Shift+K".into()));
    }

    /// **An empty legend has no anchor at all** — `ANCHORS.md` v1.11. Both
    /// spellings of "nothing" reach it, and neither produces a box.
    #[test]
    fn an_empty_legend_renders_nothing_rather_than_an_empty_box() {
        for source in [
            Source::Keys(vec![]),
            Source::Binding(SharedString::default()),
        ] {
            let binding = Keybinding {
                source,
                platform: Platform::Mac,
            };
            assert!(binding.display_keys().is_empty(), "{binding:?}");
            assert_eq!(binding.label(), None, "{binding:?}");
            assert!(
                binding
                    .render(&Theme::DARK, &crate::anchor::Unanchored)
                    .is_none(),
                "{binding:?}",
            );
            assert!(
                binding
                    .render_root(&Theme::DARK, &crate::anchor::Unanchored)
                    .is_none(),
                "{binding:?}",
            );
        }
    }

    /// The modifier sort is what makes `shift+mod+k` and `mod+shift+k` the same
    /// picture — `parseKeyCombination` sorts before formatting, so the legend is
    /// a function of the *set*.
    #[test]
    fn the_modifiers_are_sorted_so_spelling_order_does_not_matter() {
        assert_eq!(
            keys("mod+shift+k", Platform::Mac),
            keys("shift+mod+k", Platform::Mac),
        );
        // `alt` < `cmd` < `ctrl` < `meta` < `shift` by byte order, which is what
        // both engines' default sort gives for these five ASCII words.
        assert_eq!(
            keys("shift+alt+cmd+a", Platform::Mac),
            vec!["\u{2325}", "\u{2318}", "\u{21e7}", "A"],
        );
        // A duplicate modifier is held once.
        assert_eq!(keys("cmd+cmd+a", Platform::Mac), vec!["\u{2318}", "A"]);
    }

    /// `mod` is rewritten to `cmd` **as a whole word**, which is the half of the
    /// parse a naive substring replace gets wrong: `model` is a key name, not a
    /// modifier.
    #[test]
    fn the_mod_rewrite_is_word_bounded_in_both_directions() {
        assert_eq!(keys("mod+a", Platform::Mac), vec!["\u{2318}", "A"]);
        assert_eq!(keys("MOD+a", Platform::Mac), vec!["\u{2318}", "A"]);
        // Not a word: left alone, and then it is an ordinary key.
        assert_eq!(keys("model", Platform::Mac), vec!["model"]);
        // And `normalizeKey`'s own `\bcmd\b` is the same rewrite off macOS.
        assert_eq!(keys("cmd+a", Platform::Other), vec!["Ctrl", "A"]);
        assert_eq!(keys("cmdx", Platform::Other), vec!["cmdx"]);
    }

    /// The special-key table and `formatKey`, on the cases that move: an arrow
    /// loses its prefix, a space becomes the word, a single character is
    /// upcased, and a function key is upcased whole.
    #[test]
    fn the_special_keys_format_the_way_the_table_says() {
        assert_eq!(keys("up", Platform::Mac), vec!["Up"]);
        assert_eq!(keys("pagedown", Platform::Mac), vec!["PageDown"]);
        assert_eq!(keys("space", Platform::Mac), vec!["Space"]);
        assert_eq!(keys("esc", Platform::Mac), vec!["Escape"]);
        assert_eq!(keys("return", Platform::Mac), vec!["Enter"]);
        assert_eq!(keys("f12", Platform::Mac), vec!["F12"]);
        // Three digits is not `/^f\d{1,2}$/`, so it is left as it is.
        assert_eq!(keys("f123", Platform::Mac), vec!["f123"]);
        // A chord is two combos separated by whitespace, flattened into one run.
        assert_eq!(
            keys("mod+k mod+s", Platform::Mac),
            vec!["\u{2318}", "K", "\u{2318}", "S"],
        );
    }

    /// **The cap is content-sized and never line-sized** — the `badge`
    /// precedent, and the single most consequential declaration here.
    #[test]
    fn the_cap_is_content_sized_and_never_line_sized() {
        assert_eq!(CONTENT_SIZED, [ID_KEYBINDING]);
        assert!(
            LINE_SIZED.is_empty(),
            "min-h-4 floors the height at 16; declaring it would compare 16 against 12",
        );
    }

    /// The floor and the line box are **different numbers**, which is what makes
    /// the declaration above load-bearing rather than cosmetic. A control: were
    /// they equal, the test above would pass either way and stop meaning
    /// anything.
    #[test]
    fn the_min_height_really_does_differ_from_the_line_box() {
        let line_box = TYPE_STEP.size.0 * 16.0 * TYPE_STEP.line_height;
        assert!(
            (line_box - 12.0).abs() < 1e-4,
            "line box is 12, got {line_box}"
        );
        assert_eq!(MIN_HEIGHT, px(16.0));
        assert!(
            (f32::from(MIN_HEIGHT) - line_box).abs() > 0.5,
            "a 4px gap is exactly the delta v1.6 warns a wrong declaration invents",
        );
    }

    /// **The border is one real pixel**, which is the inverse of `kbd`'s 0 — and
    /// `border.w` is compared *exactly*, so the direction matters.
    ///
    /// The arithmetic is the reference's own: a `23.843` advance in a `37.844`
    /// box leaves `14.001` for two paddings and two borders, which is `6+6+1+1`.
    #[test]
    fn the_border_is_one_real_pixel_and_kbds_is_zero() {
        assert_eq!(BORDER_WIDTH, px(1.0));
        assert_eq!(super::super::kbd::HEIGHT, px(20.0));

        let advance = 23.843_f32;
        let box_width = advance + 2.0 * f32::from(PADDING_X) + 2.0 * f32::from(BORDER_WIDTH);
        assert!(
            (box_width - 37.844).abs() < 0.01,
            "23.843 + 2×6 + 2×1 should be the reference's 37.844, got {box_width}",
        );

        // A control: dropping the border — the mistake a port reusing `kbd`
        // would make — misses the reference by a whole 2px, which is four times
        // the ±0.5 the bounds tolerance allows.
        let without_border = advance + 2.0 * f32::from(PADDING_X);
        assert!((37.844 - without_border).abs() > 1.5, "{without_border}");
    }

    /// The type step and weight the reference reports: 12px on a 12px line at
    /// weight **400**, where `kbd`'s is 500.
    #[test]
    fn the_type_step_is_the_references_and_not_kbds() {
        assert_eq!(TYPE_STEP.size, Rems(0.75));
        assert!((TYPE_STEP.size.0 * 16.0 - 12.0).abs() < f32::EPSILON);
        assert_eq!(WEIGHT, FontWeight::NORMAL);
        assert_ne!(WEIGHT, super::super::kbd::WEIGHT);
        // Same font size, different line box — the pair that makes neither
        // component line-sized for the same reason.
        assert_eq!(TYPE_STEP.size, super::super::kbd::TYPE_STEP.size);
        assert!(
            (TYPE_STEP.line_height - super::super::kbd::TYPE_STEP.line_height).abs() > 0.1,
            "12/12 against 12/16 — the difference is the whole reason neither is line-sized",
        );
    }

    /// The radius is the theme's `--radius-md`, which this project redefines
    /// away from Tailwind's stock 6 — and it is **not** `kbd`'s literal 4.
    #[test]
    fn the_radius_is_the_theme_step_and_not_kbds_literal() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(theme.radius_md.value(), px(8.0));
            assert_ne!(theme.radius_md.value(), super::super::kbd::RADIUS);
        }
    }

    /// The `keys` arm takes its strings literally — no parse, no normalisation —
    /// which is what makes the dead `tab-context-menu` call site a different
    /// picture from the live one.
    #[test]
    fn the_keys_arm_is_literal_where_the_binding_arm_parses() {
        let literal = Keybinding {
            source: Source::Keys(vec![
                SharedString::new_static("Cmd"),
                SharedString::new_static("W"),
            ]),
            platform: Platform::Mac,
        };
        // The very string `tab-context-menu.tsx` builds, and it is *not* `⌘W`.
        assert_eq!(literal.label().map(|l| l.to_string()), Some("CmdW".into()));
        assert_ne!(literal.label(), Keybinding::fixture().label());
    }

    /// The fixture is the captured `⌘W` cap.
    #[test]
    fn the_fixture_is_the_captured_close_tab_cap() {
        let fixture = Keybinding::fixture();
        assert_eq!(
            fixture.source,
            Source::Binding(SharedString::new_static("mod+w")),
        );
        assert_eq!(fixture.platform, Platform::Mac);
        assert!(
            fixture
                .render(&Theme::DARK, &crate::anchor::Unanchored)
                .is_some(),
        );
    }
}
