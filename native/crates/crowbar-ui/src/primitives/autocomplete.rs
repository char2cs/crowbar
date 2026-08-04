//! `autocomplete` — an ordinary component port, and the primitive `command`
//! composes.
//!
//! The native half of `web/src/components/ui/autocomplete.tsx`, a set of
//! Tailwind class lists over `@base-ui/react`'s `Autocomplete`. P3.32's own
//! brief flagged this pair as needing Zed's `fuzzy_nucleo` — measured and
//! found wrong: a grep for `.filter(`/`score`/`fuzzy`/`match(`/`includes(`/
//! `indexOf(` across both files returns **0** lines. The real matchers live in
//! `web/src/utils/fuzzy-matcher.tsx` and `utils/search-match.ts`, outside
//! `components/ui/`, and are Phase 4 work. Nothing here ports a matcher.
//!
//! # Wrap or build: **build**
//!
//! `autocomplete.tsx` reaches no `gpui-component` widget at all — it
//! delegates to `@base-ui/react/autocomplete` (headless behaviour) plus this
//! app's own already-ported [`super::input::Input`]/[`super::scroll_area`].
//! §10.1's "do not rebuild a primitive that exists there" therefore does not
//! fire: the closest vendor concept — `gpui_component::combobox::Combobox` —
//! is a *different* shape (a trigger that opens a floating popup), and its
//! item box is `SearchableListItemElement`, which `native/mapping/select.md`
//! already found unmeasurable (`h_flex().id(self.id)` is built inside the
//! vendor's own `RenderOnce::render`, real chrome and all — confirmed again
//! here by reading `searchable_list/item.rs` directly). `autocomplete.tsx`'s
//! own shape — an always-mounted input with an inline, non-floating list
//! below it — is `Command`'s shape (see `command.rs`), not `Combobox`'s.
//!
//! Every box below is therefore this crate's own `div()`, exactly as
//! `dialog`'s header/title/footer are despite `gpui_component::dialog`
//! existing: the vendor widget worth wrapping here is the *unrelated*
//! `Combobox`/`Select`, and wrapping the wrong shape to satisfy §10.1's
//! letter would be the fake convergence `ANCHORS.md` exists to refuse.
//!
//! # Reachability, and the one thing `data-slot` hides
//!
//! `autocomplete.tsx` has **one importer**: `command.tsx`. Every one of its
//! exports is nonetheless live — `command.tsx`'s `Command`/`CommandInput`/
//! `CommandList`/`CommandItem`/`CommandEmpty` each render this file's
//! `Autocomplete`/`AutocompleteInput`/`AutocompleteList`/`AutocompleteItem`/
//! `AutocompleteEmpty` directly, restyled through `className`, not
//! reimplemented — confirmed live (§ below) rather than read off the source
//! alone: `command.tsx`'s own wrappers pass **no `data-slot` override** for
//! `CommandInput`, so the live DOM's actual `<input>` still carries
//! `data-slot="autocomplete-input"`, not a `command-input` that does not
//! exist anywhere in the file. `CommandList`/`CommandItem`/`CommandEmpty` *do*
//! pass their own `data-slot`, and because it lands after `autocomplete.tsx`'s
//! own `{...props}` spread it wins — the live DOM shows `command-list`/
//! `command-item`/`command-empty`, and `autocomplete-list`/`autocomplete-item`/
//! `autocomplete-empty` are masked on every cell `command.rs` renders. Two
//! different id vocabularies naming the *same* elements, which is why
//! `command.rs` reuses this module's structs directly (see its module docs)
//! rather than re-deriving the same boxes under new ids: there is only one
//! set of boxes, and this file is where they are built.
//!
//! `showTrigger`/`showClear` are **never set** by the one reachable call site
//! (`CommandInput` passes neither), so [`Input::show_trigger`] and
//! [`Input::show_clear`] are modelled — the primitive really has them — but
//! unreached, the same call `dialog::Dialog::description` makes about its own
//! unreached field.
//!
//! # What was captured, and how
//!
//! The dev server serves the **shared** worktree (`dialog.md` §6's wall,
//! met again here): this branch's `data-oracle-id` edits are not live there.
//! Following `dialog.md`'s own workaround, every number below was read from
//! `getComputedStyle`/`getBoundingClientRect` on the live, unmodified
//! `[data-slot="autocomplete-*"]` elements inside the running app's command
//! palette (Context Pill → click → workspace switcher), pinned at rest
//! (`command-dialog-popup.style.transition = 'none'`, confirmed
//! `transform: none`, `opacity: 1`, no `data-starting-style` first). See
//! `command.rs`'s module docs for the full capture transcript and
//! `native/mapping/autocomplete.md` for the per-field table.

use gpui::{
    AnyElement, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, SharedString,
    Styled as _, div, px, relative,
};

use crate::anchor::AnchorSink;
use crate::surfaces::rows::git_status_row::Breakpoint;
use super::input::Size as InputSize;
use crate::theme::{Color, Theme};

/// The root anchor when this surface is captured on its own: `AutocompleteInputGroup`.
pub const ID_INPUT_GROUP: &str = "autocomplete-input-group";
/// The `startAddon` slot — an empty box; see [`Input::start_addon`].
pub const ID_START_ADDON: &str = "autocomplete-start-addon";
/// `AutocompletePrimitive.Input`, rendered through this app's own `<Input
/// nativeInput>` — the field the value, the placeholder and the caret paint
/// in.
pub const ID_INPUT: &str = "autocomplete-input";
/// `AutocompleteTrigger`. **Unreached** — see the module docs.
pub const ID_TRIGGER: &str = "autocomplete-trigger";
/// `AutocompleteClear`. **Unreached** — see the module docs.
pub const ID_CLEAR: &str = "autocomplete-clear";
/// `AutocompleteItem`. Exactly one exists on any cell this port can reach —
/// see [`Item`]'s own docs for why a second one would be an `ANCHORS.md`
/// v1.8 refusal rather than a picture this port renders.
pub const ID_ITEM: &str = "autocomplete-item";
/// `AutocompleteEmpty` — a real DOM node on every cell, `:empty` (and so
/// zero-height) whenever [`ListContent::Item`] is what filled the list.
pub const ID_EMPTY: &str = "autocomplete-empty";
/// `AutocompleteList`'s own list container, **inside** `ScrollArea`'s
/// viewport — not the viewport itself, which belongs to the already-ported
/// [`super::scroll_area`] surface.
pub const ID_LIST: &str = "autocomplete-list";

/// The anchors on this surface whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// **None.** The input group and the list are both `w-full`; the field's
/// width is `w-full` too, and an `<input>` has no text node regardless (see
/// `input.rs`'s own module docs); the item's content is the *call site's*,
/// unanchored for the reason [`Item::content_height`] gives.
pub const CONTENT_SIZED: [&str; 0] = [];

/// The anchors whose **box height is their own line box**
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// **None.** Every anchor here is padding-plus-content or a call-site
/// parameter — never a bare text run whose box has no padding of its own.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `border`/`bg`/`shadow` etc. on the control resolve entirely through
/// [`super::input`]'s own vocabulary once `size` and `startAddon` are known —
/// see [`Input::render`] for the one place that is not true (the addon
/// gutter, which is `autocomplete.tsx`'s own class, not `input.tsx`'s).
///
/// This surface's own breakpoint is fixed at [`Breakpoint::Sm`] for the
/// reason `dialog`'s footer states its own: every cell this port can drive
/// is at or above 640px.
const BP: Breakpoint = Breakpoint::Sm;

/// `min-h-8`/`sm:min-h-7` on an item — `Item`'s own content usually exceeds
/// it (see [`Item::content_height`]), so this is carried for provenance
/// rather than read anywhere.
pub const ITEM_MIN_HEIGHT: Pixels = px(SPACING * 7.0);

/// `px-2` on an item.
pub const ITEM_PADDING_X: Pixels = px(SPACING * 2.0);

/// `AutocompleteItem`'s own `py-1` — **not** what `command.rs` renders; see
/// that module for the `py-1.5` override.
pub const ITEM_PADDING_Y: Pixels = px(SPACING * 1.0);

/// `rounded-sm` on an item.
pub const ITEM_RADIUS: Pixels = px(6.0);

/// `AutocompleteList`'s own `not-empty:p-1` — **not** what `command.rs`
/// renders; that module's `CommandList` overrides it to `p-2`, and
/// tailwind-merge takes the later declaration.
pub const LIST_PADDING: Pixels = px(SPACING * 1.0);

/// `startAddon`'s own box, measured live at `23×36` (padding-left plus the
/// `SearchIcon`'s own rendered extent) — see [`Input::addon_box`].
pub const ADDON_WIDTH: Pixels = px(23.0);

/// The addon-clearing gutter `autocomplete.tsx` puts on the **field**
/// through a `*:data-[slot=autocomplete-input]:ps-*` selector on the
/// control — a different class from `input.tsx`'s own `leftIcon` gutter
/// (`Size::icon_gutter`), confirmed live: the reachable field (size `lg`,
/// `startAddon` set) reports `padding-left: 31px`, not `Size::Lg`'s bare
/// `padding_x()` of 11.
///
/// `size="sm"` has no live reference; both arms are carried because the
/// class exists for both, symmetric with [`Input::padding_x`]'s own two
/// arms.
#[must_use]
pub const fn addon_gutter(size: InputSize) -> Pixels {
    match size {
        InputSize::Sm => px(SPACING * 7.0 - 1.0),
        InputSize::Default | InputSize::Lg => px(SPACING * 8.0 - 1.0),
    }
}

/// `has-[+trigger,+clear]:*:...:pe-6.5`/`pe-7` — the trailing-edge twin of
/// [`addon_gutter`], gated on [`Affordances::show_trigger`] or
/// [`Affordances::show_clear`]. **Unreached** — see the module docs.
///
/// Unlike `addon_gutter`, this class is a **plain** `pe-*` utility with no
/// `calc(…-1px)` — read off the source rather than assumed after
/// `addon_gutter`'s own `-1px` cost a first draft of this function the same
/// mistake once.
#[must_use]
pub const fn trailing_gutter(size: InputSize) -> Pixels {
    match size {
        InputSize::Sm => px(SPACING * 6.5),
        InputSize::Default | InputSize::Lg => px(SPACING * 7.0),
    }
}

/// One `AutocompleteInput`: the group, the optional `startAddon`, the
/// control and the field.
///
/// # Why this does not call [`super::input::Input::render`]
///
/// It is the same `<Input>` underneath (`autocomplete.tsx` literally writes
/// `render={<Input nativeInput size={sizeValue} />}`), but base-ui's `render`
/// prop clones that element and merges `AutocompletePrimitive.Input`'s own
/// generated props onto it — confirmed live rather than assumed: the
/// reachable field's `padding-left` (31px) traces to a selector
/// (`*:data-[slot=autocomplete-input]:ps-*`) authored on the **control**'s
/// merged `className`, which lands on `Input`'s outer `<span>` because that
/// is the element base-ui actually clones. `input.rs`'s `Input` has no seam
/// for a caller to inject that selector's *effect* (a child's padding) from
/// outside — its `field()` is a private method with a closed
/// [`super::input::LeadingPad`] vocabulary that the module's own docs say is
/// deliberately not open to a second call site's numbers. Reusing
/// [`InputSize`]'s own arithmetic (this module does, throughout) gets the
/// shared part without reopening that file's closed vocabulary for a class
/// that is not even `input.tsx`'s.
/// Which of `AutocompleteInput`'s optional slots render — a bundle for the
/// reason `input.rs`'s own `State` is one: they are one kind of thing (the
/// presence conditions `AutocompleteInput`'s own JSX reads), and
/// `struct_excessive_bools` is the lint that asks the question. Kept
/// separate from [`Input::transparent`], which is a style override rather
/// than a presence flag.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Affordances {
    /// `startAddon` — an empty box (an icon a call site chooses; no native
    /// equivalent, the `input.rs` `icon` convention). `true` on the one
    /// reachable cell (`CommandInput`'s `SearchIcon`).
    pub start_addon: bool,
    /// `showTrigger`. **Unreached.**
    pub show_trigger: bool,
    /// `showClear`. **Unreached.**
    pub show_clear: bool,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Input {
    /// `size`.
    pub size: InputSize,
    /// Which optional slots render.
    pub affordances: Affordances,
    /// Whether `command.rs`'s override (`border-transparent! bg-transparent!
    /// shadow-none before:hidden has-focus-visible:ring-0`) is in force.
    /// `false` is `autocomplete.tsx`'s own resting control — modelled for
    /// completeness, **unreached**: the only live call site is
    /// `CommandInput`, which always sets it.
    pub transparent: bool,
    /// The placeholder text.
    pub placeholder: SharedString,
    /// The value, where the field holds one. `None` is the live reference.
    pub value: Option<SharedString>,
}

impl Input {
    /// The live `CommandInput` inside the workspace switcher: `size="lg"`,
    /// `startAddon={<SearchIcon/>}`, `border-transparent!` etc., placeholder
    /// "Switch workspace…", empty.
    ///
    /// Measured at a 1714px viewport, dark: group `554×36`, addon `23×36`,
    /// control `554×36` (border `1px` transparent, radius `10`), field
    /// `552×34` (`padding-left: 31px`, `padding-right: 11px`).
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            size: InputSize::Lg,
            affordances: Affordances {
                start_addon: true,
                show_trigger: false,
                show_clear: false,
            },
            transparent: true,
            placeholder: SharedString::new_static("Switch workspace…"),
            value: None,
        }
    }

    #[must_use]
    fn is_empty(&self) -> bool {
        self.value.is_none()
    }

    #[must_use]
    fn painted(&self) -> &SharedString {
        self.value.as_ref().unwrap_or(&self.placeholder)
    }

    /// The control's own height: the field's [`InputSize::extent`] plus two
    /// borders — real space even when [`Input::transparent`] makes them
    /// invisible, since a `border` utility reserves the pixel regardless of
    /// its colour. Measured live at `36` (`34 + 2×1`).
    #[must_use]
    pub fn control_height(&self) -> Pixels {
        px(f32::from(self.size.extent(BP)) + 2.0)
    }

    /// Renders the group, opting [`ID_INPUT_GROUP`] into `anchors`.
    ///
    /// **Always [`AnchorSink::boxed`], never [`AnchorSink::root`].**
    /// `autocomplete.tsx`'s own `Autocomplete` (`AutocompletePrimitive.Root`)
    /// renders no DOM node of its own — it is a headless context provider —
    /// so there is no primitive-level box that contains the input and the
    /// list together; that combination exists only at a call site, and the
    /// one live call site is `command.tsx`, whose own root is
    /// `command-dialog-popup`. A `--surface autocomplete` capturing this
    /// group alone would therefore not be a picture any real screen shows —
    /// see `command.rs`, the surface this module's boxes are actually
    /// reached through.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut group = div().relative().w_full().text_color(theme.foreground);

        if self.affordances.start_addon {
            group = group.child(anchors.boxed(ID_START_ADDON.into(), Self::addon_box()));
        }
        group = group.child(self.control(theme, anchors));
        if self.affordances.show_trigger {
            group = group.child(anchors.boxed(ID_TRIGGER.into(), Self::affordance_box()));
        }
        if self.affordances.show_clear {
            group = group.child(anchors.boxed(ID_CLEAR.into(), Self::affordance_box()));
        }

        anchors.boxed(ID_INPUT_GROUP.into(), group)
    }

    /// `startAddon`'s empty box: `absolute inset-y-0 start-px ... ps-3`,
    /// unanchored content (a caller's icon), for the reason `input.rs`'s
    /// `icon_box` is.
    ///
    /// **Width is a measured constant, not `ps-3` alone.** The box is
    /// absolutely positioned with no explicit width and no rendered icon
    /// (this port's own icon convention), so an unmeasured version would
    /// collapse to its own padding (11px) — the live box, icon included, is
    /// `23×36`.
    fn addon_box() -> Div {
        div()
            .absolute()
            .top(px(0.0))
            .bottom(px(0.0))
            .left(px(1.0))
            .w(ADDON_WIDTH)
            .pl(px(SPACING * 3.0 - 1.0))
            .flex()
            .items_center()
    }

    /// The `AutocompletePrimitive.Trigger`/`.Clear` empty box: `size-8
    /// sm:size-7` — a fixed pair of classes, **not** driven by the input's
    /// own `size` prop (this port is always at `sm:` — see [`BP`]).
    /// Unanchored content. **Unreached.**
    fn affordance_box() -> Div {
        let extent = px(SPACING * 7.0);
        div().w(extent).h(extent)
    }

    /// The control (`<Input>`'s outer `<span>`) — real paint (border, bg,
    /// radius), **no anchor of its own**. `Input`'s own hardcoded
    /// `data-oracle-id="input-control"` (already shipped, `input.rs`'s own
    /// surface) lands on this same live element, confirmed by reading
    /// `input.tsx`'s source: the literal is on the `<span>` itself, never
    /// spread from props, so `AutocompletePrimitive.Input`'s own merged
    /// props (this module's `data-oracle-id="autocomplete-input"` included)
    /// cannot reach it — they land on the *inner* `<input>` instead, exactly
    /// where [`ID_INPUT`] belongs. This port does not re-declare
    /// `input-control` here: it is a different, already-ported surface's
    /// anchor, not this one's, the same call `command.rs`'s own docs make
    /// about not re-deriving boxes under a second name.
    ///
    /// The field is a **normal-flow child**, not absolutely positioned: the
    /// control's own `border(1px)` already insets it by exactly that much on
    /// every side (measured live: field `552×34` at `(1,1)` inside a
    /// `554×36` control), which is what [`Input::control_height`]'s own
    /// `+2.0` term accounts for.
    fn control(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut control = div()
            .relative()
            .flex()
            .w_full()
            .rounded(theme.radius_lg.value())
            .text_size(InputSize::text_size(BP, theme))
            .line_height(relative(InputSize::control_line_height(BP)));

        if self.transparent {
            control = control.border(px(1.0)).border_color(Color::TRANSPARENT);
        } else {
            control = control
                .border(px(1.0))
                .border_color(theme.input)
                .bg(theme.input.mix(32.0, Color::TRANSPARENT));
        }

        let mut left_pad = self.size.padding_x();
        if self.affordances.start_addon {
            left_pad = addon_gutter(self.size);
        }
        let mut right_pad = self.size.padding_x();
        if self.affordances.show_trigger || self.affordances.show_clear {
            right_pad = trailing_gutter(self.size);
        }

        let field = div()
            .w_full()
            .min_w(px(0.0))
            .h(self.size.extent(BP))
            .rounded(theme.radius_lg.value())
            .pl(left_pad)
            .pr(right_pad)
            .line_height(self.size.line_height(BP))
            .text_color(if self.is_empty() {
                theme.muted_foreground.mix(72.0, Color::TRANSPARENT)
            } else {
                theme.foreground
            })
            .overflow_hidden()
            .child(self.painted().clone());

        control
            .child(anchors.boxed(ID_INPUT.into(), field))
            .into_any_element()
    }
}

/// Which of the two mutually-exclusive pictures a cell is in — whether
/// [`empty`] paints real content or [`List`] paints a row. **Not** a field
/// of one primitive nested inside the other: `AutocompleteEmpty` and
/// `AutocompleteList` are siblings in the source (`CommandPanel` nests
/// `<CommandEmpty/><CommandList>…</CommandList>` directly), so this enum is
/// shared state a caller (`command.rs`) reads twice — once to choose
/// [`empty`]'s `has_content`, once to build [`List::content`] — rather than
/// something either primitive owns alone.
///
/// **Never both real at once on a live cell** — base-ui's `Autocomplete.Empty`
/// is `:empty` (and so zero-height) exactly when the list has a row, which is
/// what keeps `ANCHORS.md` v1.8's "each anchor at most once" satisfied on
/// every cell this port can reach: the one workspace fixture this dev
/// environment holds means exactly one `autocomplete-item`/`command-item`
/// ever exists in the document at once, never two.
#[derive(Clone, Debug, PartialEq)]
pub enum ListContent {
    /// The list has no rows: `AutocompleteEmpty` paints `not-empty:p-2
    /// text-center`. **Modelled, unreached** — no fixture this dev
    /// environment holds has zero workspaces.
    Empty,
    /// One row. See [`Item`].
    Item(Item),
}

/// One `AutocompleteItem` — the call site's own content, as the height it
/// comes out at.
///
/// # Why the content is a height, not an element tree
///
/// Exactly `popover`'s `body_height`/`dialog`'s `body_height`: an item's
/// children are the call site's (`workspace-switcher.tsx` renders an icon, a
/// truncated label and a trailing check), and none of that is
/// `autocomplete.tsx`'s. The live reachable row is `558×30` — under
/// `min-h-8`'s 32px floor at rest, so the floor is carried
/// ([`ITEM_MIN_HEIGHT`]) but not read: this cell's content already exceeds
/// it once `command.rs`'s `py-1.5` padding is folded in
/// (`30 = 2×6 + 18`, the call site's own two icons' line box).
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Item {
    /// The row's own content height, between its vertical padding.
    pub content_height: Pixels,
    /// `data-highlighted:bg-accent` — the keyboard cursor's row.
    /// `autoHighlight="always"` plus a one-item list means this is `true` on
    /// every reachable cell; `false` is modelled for the primitive's other
    /// picture.
    pub highlighted: bool,
    /// `data-disabled:opacity-64`. **Unreached** — invisible to the differ
    /// regardless (`ANCHORS.md` v1.7 fires only at zero opacity).
    pub disabled: bool,
}

impl Item {
    /// The live `workspace-switcher.tsx` row through `command.rs`'s
    /// `py-1.5`: content height 18 (a 13px label's own line box, the call
    /// site's own `text-[13px]` override — not this primitive's `text-sm`).
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            content_height: px(18.0),
            highlighted: true,
            disabled: false,
        }
    }

    /// The item's own box, with `padding_y` supplied by the caller —
    /// `autocomplete.tsx`'s own `py-1` or `command.tsx`'s `py-1.5` override.
    /// See [`ITEM_PADDING_Y`] and `command.rs`'s own constant.
    ///
    /// Takes `self` by value: `Item` is `Copy` and small, and clippy's
    /// `trivially_copy_pass_by_ref` is the lint that asks the question.
    fn render(self, theme: &Theme, padding_y: Pixels) -> Div {
        let mut item = div()
            .flex()
            .items_center()
            .w_full()
            .px(ITEM_PADDING_X)
            .py(padding_y)
            .rounded(ITEM_RADIUS)
            .font_weight(ITEM_FONT_WEIGHT)
            .h(padding_y * 2.0 + self.content_height);

        if self.highlighted {
            item = item.bg(theme.accent).text_color(theme.accent_foreground);
        }
        if self.disabled {
            item = item.opacity(0.64);
        }
        item
    }
}

/// `AutocompleteEmpty`, rendered — opts [`ID_EMPTY`] into `anchors`.
///
/// **A sibling of the list, not nested inside it.** `workspace-switcher.tsx`
/// nests `<CommandEmpty/><CommandList>…</CommandList>` directly inside
/// `CommandPanel`, both direct children of the panel — confirmed live,
/// `command-empty`'s own parent is the panel, not the scroll area, and its
/// position (`(1,50)` relative to the popup root) sits at the panel's own
/// content-box origin, one border-width in, **before** the list's own
/// `list_padding`. A first draft nested this inside [`List::render`]
/// instead, on the assumption that `AutocompleteEmpty` was `AutocompleteList`'s
/// own concern because both are usually reached through the same call
/// site — wrong shape, caught by a driver snapshot that put the anchor at
/// the *item's* padded position (`(9,58)`) rather than the live `(1,50)`.
///
/// **Mounted on every cell, real content only when [`ListContent::Empty`].**
/// Confirmed live: the reachable `command-empty` (one workspace, so
/// [`ListContent::Item`]) is a genuine `0×574` DOM node, not an absent one —
/// base-ui always renders `Autocomplete.Empty`, and `not-empty:p-2` is a
/// `:empty` pseudo-class selector that only paints padding (and a string)
/// once the element actually has content. `has_content = false` reproduces
/// that: a bare `div()` with no padding and no text collapses to zero height
/// exactly as the live node does. The `true` arm (this primitive's other
/// picture, `not-empty:p-2 text-center text-base text-muted-foreground
/// sm:text-sm`) has no live reference.
#[must_use]
pub fn empty(theme: &Theme, anchors: &dyn AnchorSink, has_content: bool) -> AnyElement {
    let empty = if has_content {
        div()
            .w_full()
            .p(px(SPACING * 2.0))
            .text_size(InputSize::text_size(BP, theme))
            .text_color(theme.muted_foreground)
    } else {
        div()
    };
    anchors.boxed(ID_EMPTY.into(), empty)
}

/// `AutocompleteList`: the already-ported [`super::scroll_area`]'s root and
/// viewport, wrapping this file's own `AutocompletePrimitive.List` — the one
/// box this module still has to build itself, because `ScrollArea::render`
/// has no seam for a caller-supplied child (its own `body()` is a plain
/// extent, exactly the shape `popover`'s and `dialog`'s bodies take — see
/// `scroll_area.rs`'s own module docs). Reproducing the two-`div()` root and
/// viewport here — rather than threading a body-extent contract meant for an
/// *unanchored* child — is cheaper and more honest than fighting that
/// contract for the one case that needs a **real, anchored** child; both
/// [`super::scroll_area::BORDER_WIDTH`] and
/// [`super::scroll_area::RADIUS`] are `0`, so nothing here diverges from
/// what that surface already establishes.
///
/// `list_padding` is the caller's — `autocomplete.tsx`'s own `not-empty:p-1`
/// or `command.tsx`'s `not-empty:p-2` override, which tailwind-merge resolves
/// in the call site's favour (confirmed live: the reachable list reports
/// `p-2`, `558×30`'s item nested one padding step inside a `574×46`
/// viewport).
#[derive(Clone, Debug, PartialEq)]
pub struct List {
    /// The root's (and so the viewport's) own extent — `size-full` inside
    /// whatever the call site's own layout gives it. Measured live at
    /// `574×46`, the same instance `scroll_area.rs`'s own fixture docs
    /// already record as "the command palette's".
    pub width: Pixels,
    /// Ditto, vertically.
    pub height: Pixels,
    /// `not-empty:p-1`/`not-empty:p-2` — see the module docs.
    pub list_padding: Pixels,
    /// `AutocompleteItem`'s own `py-1` or `command.tsx`'s `py-1.5` override,
    /// threaded down to [`Item::render`].
    pub item_padding_y: Pixels,
    /// What is inside.
    pub content: ListContent,
}

impl List {
    /// The live command palette's list: `574×46`, `p-2` (command's
    /// override), one highlighted item.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            width: px(574.0),
            height: px(46.0),
            list_padding: px(SPACING * 2.0),
            item_padding_y: px(SPACING * 1.5),
            content: ListContent::Item(Item::fixture()),
        }
    }

    /// Renders the root, the viewport and the list container, opting
    /// [`ID_LIST`] and, where present, [`ID_ITEM`] into `anchors`. **Not**
    /// [`ID_EMPTY`] — see [`empty`]'s own docs for why that is a sibling of
    /// this element, rendered by the caller, rather than a child of it.
    /// Also opts the already-ported [`super::scroll_area`]'s own
    /// `ID_ROOT`/`ID_VIEWPORT` in — confirmed live:
    /// `scroll-area-root`/`scroll-area-viewport` genuinely appear nested
    /// under `command-panel`, the same ids that surface's own standalone
    /// call sites (`workspace-tree`, `git-panel`) carry.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut list = div().w_full().h_full().p(self.list_padding);

        if let ListContent::Item(item) = &self.content {
            list = list
                .child(anchors.boxed(ID_ITEM.into(), (*item).render(theme, self.item_padding_y)));
        }

        let viewport = anchors.boxed(
            super::scroll_area::ID_VIEWPORT.into(),
            div()
                .w_full()
                .h_full()
                .overflow_hidden()
                .child(anchors.boxed(ID_LIST.into(), list)),
        );

        let root = div()
            .relative()
            .w(self.width)
            .h(self.height)
            .child(viewport);
        anchors.boxed(super::scroll_area::ID_ROOT.into(), root)
    }
}

/// `AutocompleteItem`'s own weight — `font-normal` (the class list sets
/// none, so this is Tailwind's inherited default) — carried so a reader
/// comparing this file against the source does not have to wonder.
pub const ITEM_FONT_WEIGHT: FontWeight = FontWeight::NORMAL;

#[cfg(test)]
mod tests {
    use super::{
        BP, CONTENT_SIZED, ID_CLEAR, ID_EMPTY, ID_INPUT, ID_INPUT_GROUP, ID_ITEM, ID_LIST,
        ID_START_ADDON, ID_TRIGGER, ITEM_FONT_WEIGHT, ITEM_MIN_HEIGHT, ITEM_PADDING_X,
        ITEM_PADDING_Y, ITEM_RADIUS, Input, Item, LINE_SIZED, LIST_PADDING, List, ListContent,
        addon_gutter, trailing_gutter,
    };
    use crate::surfaces::rows::git_status_row::Breakpoint;
    use crate::primitives::input::Size as InputSize;
    use crate::theme::Theme;
    use gpui::{FontWeight, px};

    /// Every length, against the compiled `calc(var(--spacing) * n)`.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;

        assert_eq!(ITEM_MIN_HEIGHT, px(STEP * 7.0)); // sm:min-h-7
        assert_eq!(ITEM_PADDING_X, px(STEP * 2.0)); // px-2
        assert_eq!(ITEM_PADDING_Y, px(STEP * 1.0)); // py-1
        assert_eq!(LIST_PADDING, px(STEP * 1.0)); // not-empty:p-1
        assert_eq!(ITEM_RADIUS, px(6.0)); // rounded-sm
        assert_eq!(BP, Breakpoint::Sm);
    }

    /// **The reachable field's addon gutter, to the live pixel.**
    /// `sm:*:...:ps-[calc(--spacing(8)-1px)]` for `size="lg"` — measured on
    /// the running `CommandInput` at `padding-left: 31px`.
    #[test]
    fn the_addon_gutter_matches_the_live_field() {
        assert_eq!(addon_gutter(InputSize::Lg), px(31.0));
        assert_eq!(
            addon_gutter(InputSize::Default),
            addon_gutter(InputSize::Lg)
        );
        assert_eq!(addon_gutter(InputSize::Sm), px(27.0));
        // Unreached, but internally consistent with `input.rs`'s own
        // `padding_x` split between `sm` and the rest.
        assert!(addon_gutter(InputSize::Sm) < addon_gutter(InputSize::Default));
    }

    /// The trailing gutter, unreached but carried for the primitive's other
    /// picture — `pe-7`/`pe-6.5`, a plain utility with no `-1px`, unlike
    /// `addon_gutter`'s own arbitrary `calc(…)` values.
    #[test]
    fn the_trailing_gutter_is_carried_though_unreached() {
        assert_eq!(trailing_gutter(InputSize::Lg), px(28.0));
        assert_eq!(trailing_gutter(InputSize::Sm), px(26.0));
        assert_eq!(
            trailing_gutter(InputSize::Default),
            trailing_gutter(InputSize::Lg)
        );
    }

    /// The fixture is the live `CommandInput`, measured off the running app.
    #[test]
    fn the_input_fixture_is_the_live_command_input() {
        let input = Input::fixture();
        assert_eq!(input.size, InputSize::Lg);
        assert!(input.affordances.start_addon);
        assert!(!input.affordances.show_trigger);
        assert!(!input.affordances.show_clear);
        assert!(input.transparent);
        assert_eq!(input.placeholder, "Switch workspace…");
        assert!(input.value.is_none());

        // `h-9.5 sm:h-8.5` at the pinned `sm:` breakpoint — the live field's
        // own 34px.
        assert_eq!(input.size.extent(BP), px(34.0));
    }

    /// The list fixture is the live command palette's scroll area — the
    /// same `574×46` `scroll_area.rs`'s own docs already record for this
    /// instance.
    #[test]
    fn the_list_fixture_is_the_live_command_palette() {
        let list = List::fixture();
        assert_eq!(list.width, px(574.0));
        assert_eq!(list.height, px(46.0));
        assert_eq!(list.list_padding, px(8.0));
        assert_eq!(list.item_padding_y, px(6.0));
        match list.content {
            ListContent::Item(item) => {
                assert!(item.highlighted);
                assert!(!item.disabled);
                // 2 * 6 (py-1.5) + 18 = 30, the live row's own height.
                assert_eq!(item.content_height, px(18.0));
            }
            ListContent::Empty => panic!("the live reference has one workspace"),
        }
    }

    /// The item's own box height folds the caller's padding around the
    /// content height — the arithmetic the live `558×30` row satisfies.
    #[test]
    fn the_item_height_is_padding_around_content() {
        let item = Item::fixture();
        let padding = px(6.0);
        let rendered_height = padding * 2.0 + item.content_height;
        assert_eq!(rendered_height, px(30.0));
    }

    /// Neither declaration list claims anything, for the reasons the module
    /// docs give.
    #[test]
    fn neither_declaration_list_claims_anything() {
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }

    /// Every anchor id is distinct and namespaced.
    #[test]
    fn every_anchor_id_is_distinct_and_namespaced() {
        let ids = [
            ID_INPUT_GROUP,
            ID_START_ADDON,
            ID_INPUT,
            ID_TRIGGER,
            ID_CLEAR,
            ID_ITEM,
            ID_EMPTY,
            ID_LIST,
        ];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
        assert!(ids.iter().all(|id| id.starts_with("autocomplete-")));
    }

    /// `AutocompleteItem` sets no weight of its own, so this stays
    /// Tailwind's (and gpui's) shared default.
    #[test]
    fn the_item_carries_no_font_weight_override() {
        assert_eq!(ITEM_FONT_WEIGHT, FontWeight::NORMAL);
    }

    /// A resting theme still renders on both tables, and does not panic
    /// building either the input or the list fixture.
    #[test]
    fn both_themes_render_the_fixture() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            let _ = Input::fixture().render(&theme, &crate::anchor::Unanchored);
            let _ = List::fixture().render(&theme, &crate::anchor::Unanchored);
        }
    }
}
