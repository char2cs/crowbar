//! `fps_overlay` — a fixed-position performance badge, and the second surface
//! in P3.52's cluster with **no `data-oracle-id` anywhere in its own file**.
//!
//! The native half of `web/src/components/layout/fps-overlay.tsx`. Every value
//! below came out of the app's own `tailwindcss` 4.3.0 with the utility as a
//! candidate — the method `native/MAPPING.md` fixes. See
//! `native/mapping/fps-overlay.md`.
//!
//! # Not dev-only, and not already covered either
//!
//! `native/mapping/layout-denominator.md` §4 already settled the first
//! question this component raises: it is gated by
//! `useSettingsStore((s) => s.settings.showFpsOverlay)`, a real Developer-tab
//! settings toggle with no `NODE_ENV`/`import.meta.env` check anywhere in the
//! path, so it ships — and is reachable — in every build. It is a genuine
//! strict-parity target, not a build artefact.
//!
//! # Why there is no reference: **the file carries no `data-oracle-id` at all**
//!
//! Unlike `skeleton.tsx` and `toast.tsx` — each of which *does* write
//! `data-oracle-id` on its own primitive, and is unreachable for a narrower,
//! call-graph reason — `fps-overlay.tsx`'s own `<div>` and every `<span>`
//! inside it carry no oracle attribute whatsoever
//! (`grep -n data-oracle-id web/src/components/layout/fps-overlay.tsx` finds
//! nothing). There is therefore no anchor id the React extractor could ever
//! collect here, in any state, on any build — a stronger and simpler
//! unreachability than either of this cluster's other two "no reference"
//! members. Every value below is instead read off the app's own compiled CSS,
//! the same discipline `skeleton.rs` and `toast.rs` already establish for a
//! component with nothing to capture.
//!
//! # The one raw colour in this whole cluster
//!
//! `style={{ background: 'rgba(0,0,0,0.72)', backdropFilter: 'blur(10px)' }}`
//! is an inline style, not a Tailwind utility — the only paint anywhere in
//! P3.52's five files that bypasses `theme.css` entirely. `rgba(0,0,0,0.72)`
//! is `color-mix(in srgb, black 72%, transparent)` by another spelling, so it
//! is minted the same way `slider.tsx`'s `bg-white` was: [`Color::BLACK`],
//! added to `theme/token.rs` by this item, mixed with
//! [`Color::TRANSPARENT`] at [`BACKGROUND_ALPHA`] percent. `backdrop-filter`
//! has no gpui equivalent at all (§6.3) and is simply not painted — the same
//! class of omission §6.3 already names for vibrancy generally.
//!
//! # `--flex` on the root is a deviation, and the same one `tooltip::Tooltip::shell` makes
//!
//! `fps-overlay.tsx`'s own `<div>` carries no `flex` class: its seven `<span>`
//! children are inline elements that flow on one line by CSS default, with no
//! wrap because nothing here is wider than the badge itself ever needs to be.
//! gpui has no inline layout model — every box is a flex container or
//! nothing — so the shell is built `.flex().flex_row().items_center()`
//! instead. A single line of non-wrapping inline content and a single-row flex
//! container paint the same picture; `tabs.rs`'s own module docs record the
//! same substitution for `TabsList`'s CSS defaults, and `toast.rs` §5 records
//! it for `Toast.Positioner`'s shrink-to-fit box.
//!
//! # The state axis, and why this is a `no_state_axis` surface
//!
//! `fps`, `max_dt_ms` and `drops` are `FrameStats`' own fields rather than the
//! §8.3 vocabulary: this component has no `hover`/`focus`/`selected` original
//! at all (`aria-hidden`, `pointer-events-none`, nothing interactive), and its
//! **content itself is real-time instrumentation the oracle cannot reproduce
//! identically** — `native/mapping/layout-denominator.md` §4 makes the same
//! call about this surface that `layout-denominator.md` makes generally: the
//! *anchors* (badge position, the three colour thresholds, the drop counter)
//! are exactly as drivable as any other state-driven primitive; only the raw
//! frame-timing number itself is not, and nothing here tries to reproduce it.
//!
//! Checked exhaustively, the same way `workspace_branch_icon`'s module docs
//! do: `export function FpsOverlay()` takes **no props at all** — not
//! `status`/`working`-shaped real props left unforwarded the way that
//! component's are, but literally zero arguments, on both `FpsOverlay` and
//! its own `FpsOverlayInner`. There is no `className` prop, no prop spread,
//! and every `className` on every element below it is a fixed string (the
//! one `cn()` call only switches `fpsColor`, itself computed from `fps`, not
//! from anything a caller passes in). `Empty` does not apply either: its
//! §8.3 meaning is a *row's* trailing edge having no badge or count,
//! modelled on `git-status-row` alone, and this is not a row. So all four
//! non-mandatory flags are unmodelled with no edge value left to reach
//! through any hypothetical caller, `crowbar-app`'s own
//! `SurfaceParams::no_state_axis` declares it, and
//! `crowbar-app/src/surfaces/fps_overlay.rs`'s module docs carry the
//! registry side of the same account.

use gpui::{AnyElement, Div, FontWeight, ParentElement as _, Pixels, Styled as _, div, px};

use super::anchor::{AnchorId, AnchorSink};
use crate::theme::{Color, Theme};

/// The single anchor this surface carries.
pub const ID_FPS_OVERLAY: &str = "fps-overlay";

/// **This one.** No `w-*`/`h-*` class anywhere on the root — its box is
/// `px-2.5 py-1.5` around whatever the seven runs measure, the same "padding
/// plus content, no authored length" shape `keybinding::CONTENT_SIZED`
/// declares.
pub const CONTENT_SIZED: [&str; 1] = [ID_FPS_OVERLAY];

/// **Nothing.** The box's height is padding plus a *multi-run* line, not one
/// text node's own line box — `ANCHORS.md` v1.6 is written for a single
/// anchored run, and declaring it here would be the `tabs` tab trap in a new
/// shape: comparing an authored padded height against a bare line box.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `bottom-8` — the badge's distance from the window's own bottom edge.
pub const BOTTOM: Pixels = px(SPACING * 8.0);

/// `right-3` — the badge's distance from the window's own right edge.
pub const RIGHT: Pixels = px(SPACING * 3.0);

/// `px-2.5`.
pub const PADDING_X: Pixels = px(SPACING * 2.5);

/// `py-1.5`.
pub const PADDING_Y: Pixels = px(SPACING * 1.5);

/// `mx-1.5` on each `·` separator — 6px of clearance on both sides of it.
pub const SEPARATOR_MARGIN_X: Pixels = px(SPACING * 1.5);

/// `text-[11px]` — an arbitrary Tailwind value, not a `ui-text-*` step; there
/// is no token for it and none is minted, the same call `tabs.rs` and
/// `command.rs` both make about a scale point the design system does not
/// carry.
pub const FONT_SIZE: Pixels = px(11.0);

/// `rgba(0,0,0,0.72)`'s alpha, as the percentage [`Color::mix`] takes.
pub const BACKGROUND_ALPHA: f32 = 72.0;

/// `text-muted-foreground/30` on the two `·` separators.
pub const SEPARATOR_ALPHA: f32 = 30.0;

/// The three-way colour threshold `fpsColor` computes.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum FpsTier {
    /// `fps === 0` — nothing has been sampled yet (the first 500ms window, or
    /// a tab that has just regained focus).
    Idle,
    /// `fps > 0 && fps < 80`.
    Bad,
    /// `fps >= 80 && fps < 110`.
    Warning,
    /// `fps >= 110`.
    Good,
}

impl FpsTier {
    /// `fps >= 110 ? 'success' : fps >= 80 ? 'warning' : fps > 0 ? 'destructive' : 'muted'`,
    /// read as the four-way ladder it actually is rather than the three
    /// `? :`s that spell it.
    #[must_use]
    pub const fn of(fps: u32) -> Self {
        if fps == 0 {
            Self::Idle
        } else if fps >= 110 {
            Self::Good
        } else if fps >= 80 {
            Self::Warning
        } else {
            Self::Bad
        }
    }

    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Idle => "idle",
            Self::Bad => "bad",
            Self::Warning => "warning",
            Self::Good => "good",
        }
    }

    /// The number's own colour.
    #[must_use]
    pub fn color(self, theme: &Theme) -> Color {
        match self {
            Self::Good => theme.success,
            Self::Warning => theme.warning,
            Self::Bad => theme.destructive,
            Self::Idle => theme.muted_foreground,
        }
    }
}

/// The four numbers `FpsOverlayInner` derives from one `requestAnimationFrame`
/// window.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct FrameStats {
    /// `stats.fps`. Zero means "not yet sampled", displayed as `—`.
    pub fps: u32,
    /// `stats.maxDt`, in whole ms. Zero means "not yet sampled", displayed as
    /// `—` — the same `> 0` guard `fps` has, and a genuinely different
    /// condition from it: a tab that just regained focus can report a first
    /// `dt` of exactly 0 while `fps` is still mid-window.
    pub max_dt_ms: u32,
    /// `stats.drops`. Always numeric — `drops === 0` prints `0 drops`, not a
    /// dash; the component has no guard on this field at all.
    pub drops: u32,
}

impl FrameStats {
    /// `useState<FrameStats>({ fps: 0, maxDt: 0, drops: 0 })` — the component's
    /// own initial render, before the first `requestAnimationFrame` window has
    /// closed. The truest "resting" cell there is: every live mount passes
    /// through this exact state for its first half-second.
    #[must_use]
    pub const fn fixture() -> Self {
        Self {
            fps: 0,
            max_dt_ms: 0,
            drops: 0,
        }
    }

    /// The number's colour tier.
    #[must_use]
    pub const fn tier(self) -> FpsTier {
        FpsTier::of(self.fps)
    }

    /// `{fps > 0 ? fps : '—'}`.
    #[must_use]
    pub fn fps_text(self) -> String {
        if self.fps > 0 {
            self.fps.to_string()
        } else {
            "—".to_owned()
        }
    }

    /// `` {maxDt > 0 ? `${maxDt}ms` : '—'} ``.
    #[must_use]
    pub fn max_dt_text(self) -> String {
        if self.max_dt_ms > 0 {
            format!("{}ms", self.max_dt_ms)
        } else {
            "—".to_owned()
        }
    }

    /// `` {drops} {drops === 1 ? 'drop' : 'drops'} ``.
    #[must_use]
    pub fn drops_text(self) -> String {
        format!(
            "{} {}",
            self.drops,
            if self.drops == 1 { "drop" } else { "drops" }
        )
    }

    /// `drops > 0` — the one boolean that picks both the drop count's colour
    /// and its weight together, `text-destructive font-medium` against
    /// `text-muted-foreground`.
    #[must_use]
    pub const fn drops_are_a_problem(self) -> bool {
        self.drops > 0
    }
}

/// The badge.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct FpsOverlay {
    /// The four numbers on screen.
    pub stats: FrameStats,
}

impl FpsOverlay {
    /// The component's own first paint.
    #[must_use]
    pub const fn fixture() -> Self {
        Self {
            stats: FrameStats::fixture(),
        }
    }

    /// A `·` separator, `mx-1.5 text-muted-foreground/30`.
    fn separator(theme: &Theme) -> Div {
        div()
            .mx(SEPARATOR_MARGIN_X)
            .text_color(
                theme
                    .muted_foreground
                    .mix(SEPARATOR_ALPHA, Color::TRANSPARENT),
            )
            .child("·")
    }

    /// The badge's own box, with its seven runs as plain, unanchored
    /// children — the same choice `tabs.rs` makes for a tab's call-site label:
    /// none of these spans carries a `data-oracle-id` in the source, so
    /// routing any of them through [`AnchorSink::text_half`] would fabricate a
    /// text group the reference has no field for.
    fn shell(&self, theme: &Theme) -> Div {
        let stats = self.stats;

        // `drops` picks colour and weight together off one boolean —
        // resolved here rather than with a builder combinator so the branch
        // reads the same way `Skeleton::shell`'s does.
        let drops = if stats.drops_are_a_problem() {
            div()
                .text_color(theme.destructive)
                .font_weight(FontWeight::MEDIUM)
        } else {
            div().text_color(theme.muted_foreground)
        };

        div()
            // `fixed bottom-8 right-3`. Nothing in `RowSurface`'s own harness
            // marks any ancestor `.relative()`, so this resolves against the
            // window itself — the same containing-block fallback CSS `fixed`
            // takes when nothing establishes one, and `row_layout::fps_overlay`
            // is what proves it rather than assumes it.
            .absolute()
            .bottom(BOTTOM)
            .right(RIGHT)
            .flex()
            .flex_row()
            .items_center()
            .rounded(theme.radius_md.value())
            .px(PADDING_X)
            .py(PADDING_Y)
            .bg(Color::BLACK.mix(BACKGROUND_ALPHA, Color::TRANSPARENT))
            // The one honest limit `file_tree_row.rs` and `inline_error.rs`
            // both already record for `theme.font_mono`: it is the family's
            // *first* stack entry, not the fallback chain `--editor-font-family`
            // resolves through.
            .font_family(theme.font_mono.primary().unwrap_or("monospace"))
            .text_size(FONT_SIZE)
            .shadow_xl()
            .child(
                div()
                    .font_weight(FontWeight::BOLD)
                    .text_color(stats.tier().color(theme))
                    .child(stats.fps_text()),
            )
            .child(div().text_color(theme.muted_foreground).child(" fps"))
            .child(Self::separator(theme))
            .child(div().text_color(theme.muted_foreground).child("max "))
            .child(
                div()
                    .text_color(theme.foreground)
                    .child(stats.max_dt_text()),
            )
            .child(Self::separator(theme))
            .child(drops.child(stats.drops_text()))
    }

    /// The element, with its one anchor.
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        anchors.boxed(
            AnchorId::new(ID_FPS_OVERLAY).content_sized(),
            self.shell(theme),
        )
    }
}

#[cfg(test)]
mod tests {
    use super::{
        BACKGROUND_ALPHA, BOTTOM, CONTENT_SIZED, FONT_SIZE, FpsOverlay, FpsTier, FrameStats,
        LINE_SIZED, PADDING_X, PADDING_Y, RIGHT, SEPARATOR_ALPHA, SEPARATOR_MARGIN_X,
    };
    use crate::theme::{Color, Theme};
    use gpui::px;

    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;
        assert_eq!(BOTTOM, px(STEP * 8.0)); // bottom-8
        assert_eq!(RIGHT, px(STEP * 3.0)); // right-3
        assert_eq!(PADDING_X, px(STEP * 2.5)); // px-2.5
        assert_eq!(PADDING_Y, px(STEP * 1.5)); // py-1.5
        assert_eq!(SEPARATOR_MARGIN_X, px(STEP * 1.5)); // mx-1.5
        assert_eq!(FONT_SIZE, px(11.0)); // text-[11px], not a spacing multiple
    }

    /// The four-way ladder, at the exact boundaries `fpsColor` tests.
    #[test]
    fn the_tier_boundaries_match_the_source_ladder() {
        assert_eq!(FpsTier::of(0), FpsTier::Idle);
        assert_eq!(FpsTier::of(1), FpsTier::Bad);
        assert_eq!(FpsTier::of(79), FpsTier::Bad);
        assert_eq!(FpsTier::of(80), FpsTier::Warning);
        assert_eq!(FpsTier::of(109), FpsTier::Warning);
        assert_eq!(FpsTier::of(110), FpsTier::Good);
        assert_eq!(FpsTier::of(500), FpsTier::Good);

        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(FpsTier::Good.color(&theme), theme.success);
            assert_eq!(FpsTier::Warning.color(&theme), theme.warning);
            assert_eq!(FpsTier::Bad.color(&theme), theme.destructive);
            assert_eq!(FpsTier::Idle.color(&theme), theme.muted_foreground);
            // Four genuinely different colours, or the ladder would be
            // decoration.
            let colors = [
                theme.success,
                theme.warning,
                theme.destructive,
                theme.muted_foreground,
            ];
            for i in 0..colors.len() {
                for j in (i + 1)..colors.len() {
                    assert_ne!(colors[i], colors[j]);
                }
            }
        }
    }

    /// The three guarded fields, at the exact `> 0` boundary each checks.
    #[test]
    fn the_dash_fallbacks_fire_at_zero_and_nowhere_else() {
        assert_eq!(FrameStats::fixture().fps_text(), "—");
        assert_eq!(FrameStats::fixture().max_dt_text(), "—");
        // `drops` has no dash: zero prints as a number.
        assert_eq!(FrameStats::fixture().drops_text(), "0 drops");

        let sampled = FrameStats {
            fps: 120,
            max_dt_ms: 8,
            drops: 1,
        };
        assert_eq!(sampled.fps_text(), "120");
        assert_eq!(sampled.max_dt_text(), "8ms");
        assert_eq!(sampled.drops_text(), "1 drop"); // singular
        assert_eq!(
            FrameStats {
                drops: 2,
                ..sampled
            }
            .drops_text(),
            "2 drops",
        );
    }

    /// **`drops` picks colour and weight together**, off one boolean.
    #[test]
    fn the_drop_count_is_only_a_problem_above_zero() {
        assert!(!FrameStats::fixture().drops_are_a_problem());
        assert!(
            FrameStats {
                drops: 1,
                ..FrameStats::fixture()
            }
            .drops_are_a_problem()
        );
    }

    /// The one raw colour in this cluster: `rgba(0,0,0,0.72)` is opaque black
    /// mixed 72% against transparent, and the mix is doing real work rather
    /// than being indistinguishable from either endpoint.
    #[test]
    fn the_background_is_black_mixed_seventy_two_percent() {
        let painted = Color::BLACK.mix(BACKGROUND_ALPHA, Color::TRANSPARENT);
        assert_ne!(painted, Color::BLACK);
        assert_ne!(painted, Color::TRANSPARENT);
        // Alpha interpolates linearly regardless of colour space, so the
        // result is exactly 0.72 opaque-black, not an approximation.
        assert!((painted.value().a - 0.72).abs() < 0.001, "{painted:?}");
        // Unlike alpha, `l` is not approximately zero — it is exactly zero,
        // with no rounding step that could perturb it: `color_mix_remainder`
        // weights each channel by `c * a`, and `BLACK`/`TRANSPARENT` both
        // have r=g=b=0, so every product and the final division are `0.0`
        // exactly in IEEE 754. `to_bits` spells that equality so
        // `clippy::float_cmp` sees it as deliberate, not an accidental `==`
        // on a float — the same reasoning `crowbar-driver`'s `assert_px`
        // helpers document for their own exact comparisons.
        assert_eq!(painted.value().l.to_bits(), 0.0f32.to_bits());
        // `SEPARATOR_ALPHA` is a literal `f32` constant, not a computed
        // value, so this is a same-value comparison of two compile-time
        // constants rather than anything that could drift by a rounding
        // error — `to_bits` for the same reason as above.
        assert_eq!(SEPARATOR_ALPHA.to_bits(), 30.0f32.to_bits());
    }

    #[test]
    fn neither_declaration_list_claims_a_line_box() {
        assert_eq!(CONTENT_SIZED, [super::ID_FPS_OVERLAY]);
        assert!(LINE_SIZED.is_empty());
    }

    #[test]
    fn the_fixture_is_the_components_own_first_paint() {
        let overlay = FpsOverlay::fixture();
        assert_eq!(overlay.stats, FrameStats::fixture());
        assert_eq!(overlay.stats.tier(), FpsTier::Idle);
    }
}
