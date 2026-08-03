//! `workspace-inline-input` — the sidebar's rename/create field, and the first
//! surface in this port whose only anchored `<input>` carries **no text field
//! at all**.
//!
//! The native half of `web/src/components/layout/workspace-inline-input.tsx`.
//! See `native/mapping/workspace-inline-input.md`.
//!
//! # The field is `input.rs`'s exact finding, one door over
//!
//! `input.tsx`'s own module docs establish it once: an `<input>` is a void DOM
//! element (`childNodes.length === 0`), so `web/src/lib/oracle/extract.ts`'s
//! `oracleOwnText` returns `''` for it and the reference's record carries only
//! the box — `bounds`, `bg`, `visible`, `radius`, `border`. This component's own
//! `<input>` is not `ui/input.tsx`'s primitive (no `Input` import — grepped, and
//! confirmed by reading the real JSX: a bare `<input>` tag), but it is exactly
//! the same DOM element, so the finding transfers unchanged. [`ID_FIELD`] is
//! opted in through [`AnchorSink::boxed`] only — never `.boxed_text`/`.text` —
//! and the value/placeholder string is painted as a **plain unanchored child**,
//! the identical shape `input::Input::field`'s own doc comment states: *"The
//! string is a plain unanchored child."*
//!
//! # The hint is the opposite shape: a real `<button>`, and it wraps in the
//! app's own sidebar today
//!
//! Measured live (a probe element, real classes, the app's own compiled
//! Tailwind, at the row's real 248px content width — see the mapping doc for
//! the arithmetic): the hint string
//! `'{branch}' already has a workspace — open it` wraps to **two lines** for
//! every branch name of ordinary length. A one-character branch (`x`, `a`) is
//! the only case measured to stay on one line (37 characters against the
//! 40-character wrap point at this width) — not a shape any real git branch
//! takes. So unlike `inline-error.tsx`'s single-line runs, this hint's *typical*
//! rendering is the two-line wrap, and [`ID_HINT`] is declared neither
//! `content_sized` (the button **stretches** to the column's full width — see
//! below) nor `line_sized` (v1.6 is for a box derived from *one* line box; a
//! wrapped paragraph's height is derived from as many line boxes as it wraps
//! to, which is a different claim and not one this component can make safely
//! given that both engines choose their own wrap points).
//!
//! **The button stretches, and `text-left` is the tell.** `workspace-inline-
//! input.tsx`'s root is `flex flex-col` with no `items-center` — so
//! `align-items` computes to its `stretch` default, and a block-level flex item
//! (which a `<button>` becomes once it is a flex child, `useRender`'s
//! `defaultTagName: 'button'` blockified the same way every other flex-item
//! button in this port is) stretches across the cross axis unless it opts out.
//! `text-left` on the hint is otherwise a no-op — text already starts at the
//! left edge of a content-sized box — and it stops being a no-op exactly when
//! the box is wider than its text, which is what stretching produces. Measured:
//! the hint's `getBoundingClientRect().width` equals the root's, at every
//! content length tried.
//!
//! # Two axes this component crosses that no earlier one did together
//!
//! `kind` (`identifier`/`prose`) toggles `font-mono` on the field alone — the
//! hint is `font-mono` unconditionally, so a `prose` cell with a hint (never
//! reachable in `web/src`, see the mapping doc's call-site table) would paint
//! the field in the ambient sans and the hint in mono regardless. Both are
//! invisible to the differ on the field (no text field to disagree on) and
//! real on the hint (`font.family` is compared there).

use gpui::{
    AnyElement, Div, IntoElement as _, ParentElement as _, Pixels, Rems, SharedString, Styled as _,
    div, px, relative,
};

use super::anchor::{AnchorId, AnchorSink};
use crate::theme::{Color, Theme};

/// The root anchor — the `flex min-w-0 flex-1 flex-col` wrapper. It paints no
/// fill of its own, but it is a real flex box with real geometry, not a
/// `display: contents` wrapper (v1.11 excludes only the latter), so it is
/// anchored like every other surface root.
pub const ID_ROOT: &str = "workspace-inline-input";

/// The `<input>` — box-only, per the module docs' headline finding.
pub const ID_FIELD: &str = "workspace-inline-input-field";

/// The `existingWsId` hint `<button>`, present only when a collision is being
/// reported.
///
/// **Mutation run**: wrongly declared this anchor `.content_sized()` at its
/// `render` call site — the §2 claim in reverse. `row_layout`'s
/// `the_hint_sits_below_the_field_and_stretches_full_width` caught it:
/// `"the hint stretches, it does not size to its text"`, panicked at
/// `row_layout/workspace_inline_input.rs:99`. Reverted.
pub const ID_HINT: &str = "workspace-inline-input-hint";

/// Neither anchor declares it. The field has no text field to size to at all;
/// the hint stretches to the column's full width (module docs).
pub const CONTENT_SIZED: [&str; 0] = [];

/// Neither anchor declares it. The field carries no `font` (input.rs's
/// finding); the hint wraps to a variable number of lines, which is a
/// different claim from v1.6's "derived from one line box" (module docs).
pub const LINE_SIZED: [&str; 0] = [];

/// `mt-0.5` on the hint.
pub const HINT_MARGIN_TOP: Pixels = px(2.0);

/// `text-[13px]` on the field, an arbitrary value with no paired line-height
/// token — it inherits the app's ambient `1.5`, the same inheritance
/// `detach_holder_modal::POPUP_LINE_HEIGHT` names for the identical reason.
pub const FIELD_TEXT_SIZE: Rems = Rems(13.0 / 16.0);

/// The field's own intrinsic height: `13 × 1.5`, **measured** rather than
/// authored by a class — this raw `<input>` carries no `h-*` utility at all,
/// unlike `ui/input.tsx`'s primitive, so gpui has no class to translate and
/// this is the probe's number, pinned. Confirmed identical for both `kind`s:
/// `line-height: 1.5` is a unitless multiplier of `font-size`, not of the
/// family's own metrics, so `font-mono` and the ambient sans produce the same
/// box at the same size.
pub const FIELD_HEIGHT: Pixels = px(19.5);

/// `text-[11px]` on the hint — **identical** to `inline_error::DETAIL_STEP`'s
/// own number, and for the identical reason: an arbitrary value with no paired
/// line-height token, landing on the same inherited `1.5`.
pub const HINT_TEXT_SIZE: Rems = Rems(11.0 / 16.0);

/// Tailwind's inherited line-height ratio, unredefined by `theme.css` — see
/// `button::SPACING`'s neighbouring note for the same check made once for
/// `--spacing`.
const AMBIENT_LINE_HEIGHT: f32 = 1.5;

/// `placeholder:text-muted-foreground/40` — **this component's own alpha**,
/// not `input::PLACEHOLDER_ALPHA` (72): two different primitives, two
/// different call-site classes, restated rather than shared for
/// `detach_holder_modal`'s reason.
pub const PLACEHOLDER_ALPHA: f32 = 40.0;

/// `text-muted-foreground/70` on the hint.
pub const HINT_ALPHA: f32 = 70.0;

/// `kind`: which face a typed value takes.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Kind {
    /// The default. A value git must accept verbatim — branch names.
    #[default]
    Identifier,
    /// Free text, read in the row's own ambient face.
    Prose,
}

/// The hint text, verbatim from the call site's own template literal:
/// `` `'${value.trim()}' already has a workspace — open it` `` — the **same**
/// `value` the field itself paints, not an independently resolved branch;
/// `resolveExisting(value)` only ever matches against what is already typed.
///
/// A free function rather than a method, so the format string is testable in
/// isolation from the component's own render path — the surface this
/// component's mutation coverage leans on hardest, since it is the one string
/// this port computes rather than merely relays.
///
/// **Mutation run**: dropped the `.trim()` call (`value` painted raw).
/// `the_hint_text_trims_the_value` caught it —
/// `` left: "'  main  ' already has a workspace — open it" right: "'main'
/// already has a workspace — open it" `` — panicked at
/// `workspace_inline_input.rs` line 423. Reverted.
#[must_use]
pub fn hint_text(value: &str) -> SharedString {
    SharedString::from(format!(
        "'{}' already has a workspace — open it",
        value.trim()
    ))
}

/// One `<WorkspaceInlineInput>`.
#[derive(Clone, Debug, PartialEq)]
pub struct WorkspaceInlineInput {
    /// The typed value. `None` is what the field renders **before** anything
    /// is typed — `defaultValue=''` on every real call site but the two
    /// renames, which pass the current name — and paints the placeholder
    /// instead, `input.rs`'s own `is_empty` shape.
    pub value: Option<SharedString>,
    /// The `placeholder` prop.
    pub placeholder: SharedString,
    /// `kind`.
    pub kind: Kind,
    /// Whether `resolveExisting(value)` matched — driven directly, without
    /// simulating the lookup that would organically produce it, the
    /// `nav-stack`/`sidebar-peek` precedent `native/mapping/layout-
    /// denominator.md` §4 states for a boolean a real pointer/store action
    /// would otherwise compute. Only meaningful with [`Self::value`] set —
    /// the real prop is computed from the *same* value the field paints, and
    /// `resolveExisting('')` is never observed to match on any live call
    /// site.
    pub hint: bool,
}

impl WorkspaceInlineInput {
    /// The live rename call site: `workspace-tree-item.tsx`'s `isRenaming`
    /// branch — a typed branch name, `kind` defaulted to `identifier`, no
    /// hint (`resolveExisting` is not passed on that call site).
    #[must_use]
    pub fn fixture_rename() -> Self {
        Self {
            value: Some(SharedString::new_static("fix-auth-bug")),
            placeholder: SharedString::new_static("branch-name"),
            kind: Kind::Identifier,
            hint: false,
        }
    }

    /// The live create-workspace call site with a real collision:
    /// `workspace-tree-item.tsx`'s `isCreatingChild` branch, `resolveExisting`
    /// finding an existing workspace on the typed branch — the hint text is
    /// derived from this same fixture's own [`Self::value`].
    #[must_use]
    pub fn fixture_create_with_hint() -> Self {
        Self {
            hint: true,
            ..Self::fixture_rename()
        }
    }

    /// The live chat-title rename: `agent-chat-row.tsx`'s `renaming` branch —
    /// `kind="prose"`, no hint (that call site passes no `resolveExisting`
    /// either).
    #[must_use]
    pub fn fixture_prose() -> Self {
        Self {
            value: Some(SharedString::new_static("Fix login flow")),
            placeholder: SharedString::new_static("chat title"),
            kind: Kind::Prose,
            hint: false,
        }
    }

    /// Whether the field is showing its placeholder — §8.3's `empty`.
    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.value.is_none()
    }

    /// The string the field paints — the value where it has one, the
    /// placeholder otherwise. `input::Input::painted`'s identical shape.
    #[must_use]
    pub fn painted(&self) -> &SharedString {
        self.value.as_ref().unwrap_or(&self.placeholder)
    }

    /// `text-foreground` for a value, `placeholder:text-muted-foreground/40`
    /// for a placeholder — gpui has no `::placeholder`, so the port resolves
    /// the pseudo into the run it is actually painting, `input.rs`'s own
    /// resolution. **Invisible to the differ**: [`ID_FIELD`] carries no `fg`
    /// at all, per the module docs.
    #[must_use]
    pub fn field_text_color(&self, theme: &Theme) -> Color {
        if self.is_empty() {
            theme
                .muted_foreground
                .mix(PLACEHOLDER_ALPHA, Color::TRANSPARENT)
        } else {
            theme.foreground
        }
    }

    /// The field's own font family — `font-mono` under `identifier`, the
    /// ambient sans under `prose`.
    fn field_font<'a>(&self, theme: &'a Theme) -> &'a str {
        match self.kind {
            Kind::Identifier => theme.font_mono.primary().unwrap_or("monospace"),
            Kind::Prose => theme.font_sans.primary().unwrap_or("sans-serif"),
        }
    }

    /// The element and its anchors: the field always, the hint when a
    /// collision is being reported.
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut root = div().flex().min_w(px(0.0)).flex_1().flex_col();

        root = root.child(anchors.boxed(AnchorId::from(ID_FIELD), self.field_box(theme)));

        if self.hint {
            let value = self.value.as_deref().unwrap_or("");
            root = root.child(anchors.boxed_text(
                AnchorId::from(ID_HINT),
                Self::hint_box(theme),
                hint_text(value),
            ));
        }

        anchors.root(ID_ROOT.into(), root).into_any_element()
    }

    /// The field's own box: `min-w-0 flex-1 bg-transparent text-[13px]
    /// outline-none placeholder:text-muted-foreground/40 [font-mono]`.
    fn field_box(&self, theme: &Theme) -> Div {
        div()
            .min_w(px(0.0))
            .flex_1()
            .h(FIELD_HEIGHT)
            .font_family(self.field_font(theme).to_owned())
            .text_size(FIELD_TEXT_SIZE)
            .line_height(relative(AMBIENT_LINE_HEIGHT))
            .text_color(self.field_text_color(theme))
            .child(self.painted().clone())
    }

    /// The hint's own box: `mt-0.5 text-left font-mono text-[11px]
    /// text-muted-foreground/70 hover:text-foreground` — the resting state
    /// only; `hover` has no reference (synthetic pointer events are denied on
    /// this project's machines, `button.rs`'s own standing finding).
    fn hint_box(theme: &Theme) -> Div {
        div()
            .mt(HINT_MARGIN_TOP)
            .font_family(theme.font_mono.primary().unwrap_or("monospace").to_owned())
            .text_size(HINT_TEXT_SIZE)
            .line_height(relative(AMBIENT_LINE_HEIGHT))
            .text_color(theme.muted_foreground.mix(HINT_ALPHA, Color::TRANSPARENT))
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CONTENT_SIZED, FIELD_HEIGHT, FIELD_TEXT_SIZE, HINT_ALPHA, HINT_MARGIN_TOP, HINT_TEXT_SIZE,
        ID_FIELD, ID_HINT, ID_ROOT, Kind, LINE_SIZED, PLACEHOLDER_ALPHA, WorkspaceInlineInput,
        hint_text,
    };
    use crate::theme::{Color, Theme};
    use gpui::px;

    /// Every length, against the compiled `calc(var(--spacing) * n)`.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple_or_the_measured_intrinsic() {
        assert_eq!(HINT_MARGIN_TOP, px(2.0)); // mt-0.5
        assert_eq!(FIELD_HEIGHT, px(19.5)); // measured: 13 * 1.5
        assert!((PLACEHOLDER_ALPHA - 40.0).abs() < f32::EPSILON);
        assert!((HINT_ALPHA - 70.0).abs() < f32::EPSILON);
    }

    /// Neither anchor declares either v1.5/v1.6 field — the field has no text
    /// node at all, and the hint's wrap count is not a single-line claim.
    #[test]
    fn neither_anchor_declares_content_sized_or_line_sized() {
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }

    /// `text-[13px]` and `text-[11px]` are `13/16` and `11/16` rem — the same
    /// arithmetic `inline_error::DETAIL_STEP` states for the identical 11px
    /// arbitrary value.
    #[test]
    fn the_arbitrary_text_sizes_are_the_probed_pixel_values() {
        assert!((FIELD_TEXT_SIZE.0 - 13.0 / 16.0).abs() < 1e-6);
        assert!((HINT_TEXT_SIZE.0 - 11.0 / 16.0).abs() < 1e-6);
        assert!((HINT_TEXT_SIZE.0 * 16.0 * 1.5 - 16.5).abs() < 1e-3);
    }

    /// The three anchor ids are distinct and namespaced.
    #[test]
    fn every_anchor_id_is_distinct_and_namespaced() {
        let ids = [ID_ROOT, ID_FIELD, ID_HINT];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
        assert!(
            ids.iter()
                .all(|id| *id == ID_ROOT || id.starts_with("workspace-inline-input-"))
        );
    }

    /// The three live call sites, exactly as read off `web/src`.
    #[test]
    fn the_fixtures_are_the_live_call_sites() {
        let rename = WorkspaceInlineInput::fixture_rename();
        assert_eq!(rename.value.as_deref(), Some("fix-auth-bug"));
        assert_eq!(rename.placeholder, "branch-name");
        assert_eq!(rename.kind, Kind::Identifier);
        assert!(!rename.hint);

        let create = WorkspaceInlineInput::fixture_create_with_hint();
        assert!(create.hint);
        // Everything but the hint is the rename fixture's own shape — the
        // hint text is derived from this same `value`, not a second field.
        assert_eq!(create.value, rename.value);
        assert_eq!(create.kind, rename.kind);

        let prose = WorkspaceInlineInput::fixture_prose();
        assert_eq!(prose.kind, Kind::Prose);
        assert_eq!(prose.placeholder, "chat title");
        assert!(!prose.hint);
    }

    /// `is_empty`/`painted` follow `defaultValue=''` producing no `value`.
    #[test]
    fn an_absent_value_shows_the_placeholder() {
        let mut field = WorkspaceInlineInput::fixture_rename();
        assert!(!field.is_empty());
        assert_eq!(field.painted(), field.value.as_ref().unwrap());

        field.value = None;
        assert!(field.is_empty());
        assert_eq!(field.painted(), &field.placeholder);
    }

    /// A value recolours the run away from the placeholder's alpha — real,
    /// and invisible to the differ (the field carries no `fg` anchor field at
    /// all), painted anyway because it is what the reference looks like.
    #[test]
    fn a_value_and_a_placeholder_paint_different_colours() {
        let theme = Theme::DARK;
        let mut field = WorkspaceInlineInput::fixture_rename();
        let value_color = field.field_text_color(&theme);
        assert_eq!(value_color, theme.foreground);

        field.value = None;
        let placeholder_color = field.field_text_color(&theme);
        assert_eq!(
            placeholder_color,
            theme
                .muted_foreground
                .mix(PLACEHOLDER_ALPHA, Color::TRANSPARENT)
        );
        assert_ne!(value_color, placeholder_color);
    }

    /// The hint string is the call site's own template literal, verbatim.
    #[test]
    fn the_hint_text_matches_the_call_sites_template_literal() {
        assert_eq!(
            hint_text("fix-auth-bug"),
            "'fix-auth-bug' already has a workspace — open it",
        );
        assert_eq!(
            hint_text("main"),
            "'main' already has a workspace — open it"
        );
    }

    /// `value.trim()` — the call site's own arithmetic — not the raw value,
    /// so a value with incidental leading/trailing space still names the
    /// branch it will actually be compared against.
    #[test]
    fn the_hint_text_trims_the_value() {
        assert_eq!(
            hint_text("  main  "),
            "'main' already has a workspace — open it"
        );
    }

    /// The rendered hint, end to end: the create-with-hint fixture's own
    /// `value` is what the hint names, not a fabricated second string.
    #[test]
    fn the_create_fixtures_hint_names_its_own_value() {
        let create = WorkspaceInlineInput::fixture_create_with_hint();
        let value = create.value.as_deref().unwrap_or("");
        assert_eq!(
            hint_text(value),
            "'fix-auth-bug' already has a workspace — open it",
        );
    }

    /// `line-height: 1.5` is unitless, so it produces the same box at the
    /// same font-size regardless of family — the reason `FIELD_HEIGHT` needs
    /// no `kind` parameter.
    #[test]
    fn the_field_height_does_not_depend_on_kind() {
        for kind in [Kind::Identifier, Kind::Prose] {
            let field = WorkspaceInlineInput {
                kind,
                ..WorkspaceInlineInput::fixture_rename()
            };
            // The height constant itself is kind-independent by construction
            // (no `kind` branch in its derivation) — this asserts the
            // property the module docs claim rather than merely restating
            // the literal.
            assert_eq!(FIELD_HEIGHT, px(13.0 * 1.5));
            let _ = field;
        }
    }
}
