//! The sidebar's own chrome: the shell, its carousel, its header, its tabs,
//! its toggles and its navigation stack. `sidebar_tree` — the row-guide
//! primitives the sidebar's *rows* render through — is grouped with
//! [`super::rows`] instead; see that module's own note for why (it has no
//! consumer here, only in the row family).

// `sidebar.rs` is renamed `shell.rs` on the filesystem — the one mechanical
// exception this item's otherwise-pure `git mv` needed. `pub mod sidebar;`
// nested inside `surfaces/sidebar/` is `clippy::module_inception`
// (`surfaces::sidebar::sidebar`), denied by this workspace's `-D warnings`;
// the file itself is untouched, only its path and its `pub mod` name. It is
// unflattened for the reason `sidebar_carousel` is, and one sharper:
// `shell::Header` is `sidebar.tsx`'s `SidebarHeader` and
// `crate::primitives::card::Header` is `card.tsx`'s, two padded containers
// with different numbers; `shell::Tone` names three text colours where
// `crate::primitives::inline_error` has its own notion of an error tone; and
// `shell::CONTENT_SIZED` is non-empty where the flattened
// `super::rows::git_status_row::CONTENT_SIZED` already occupies the bare
// name in that module — a declaration list that silently meant another
// surface's is exactly the mistake `ANCHORS.md` v1.6 warns about.
//
// **`shell` carries two surfaces and covers two of its file's six visual
// exports.** `Sidebar` and `SidebarFooter` are reported-and-stopped rather than
// rebuilt: the vendor puts geometry between its own border box and the child
// this crate can hold, so neither can reach strict parity. The module docs and
// `native/mapping/sidebar.md` carry the account and the quoted vendor code.
pub mod shell;
// The three P3.52 leaves (`sidebar_project_header`, `sidebar_tab_bar`,
// `sidebar_toggle_icon`) are unflattened for the same reason as the rest.
// `sidebar_project_header::ID_SIDEBAR_PROJECT_HEADER` and its siblings'
// `ID_*` constants would sit next to every other surface's own with the same
// shape of collision, and `sidebar_tab_bar::Tabs` (built from
// `crate::primitives::tabs::Tabs`) would read as a second, unrelated `Tabs`
// type without the module in front of it. None of the three carries a
// reference — see each module's own docs, and
// `native/mapping/layout-denominator.md` §8 Cluster 3 for the survey that
// grouped them.
pub mod sidebar_project_header;
pub mod sidebar_skeleton;
pub mod sidebar_tab_bar;
pub mod sidebar_toggle_icon;
// `sidebar_carousel` is flattened not at all for the same reason: its
// `CONTENT_SIZED`, `LINE_SIZED` and `ID_*` are its own, and a declaration list
// that silently meant another surface's is the mistake `ANCHORS.md` v1.6 warns
// about.
pub mod sidebar_carousel;
// `sidebar_toast_overlay` (P3.62) is unflattened for the same reason as the
// rest, and one further one: it backs **two** registered surfaces
// (`sidebar-toast-overlay`/`sidebar-toast-overlay-fallback`, the registry's
// unique-root constraint applied to one component's two DOM shapes — see the
// module docs §1), and both call sites need `Kind`/`Side`/`ToastFixture`
// read as this module's own.
pub mod sidebar_toast_overlay;
// `sidebar_peek` (P3.59) is unflattened for the same reason as the rest.
// Carries one anchor, on its *inner* div — see the module docs for why the
// outer `data-sidebar-peek` wrapper gets none, the same call
// `super::workspace::workspace_switcher`'s own wrapper made, extended to a
// conditionally `display: contents` element.
pub mod sidebar_peek;

// `nav_stack` (P3.59) is unflattened for the same reason as the rest — its
// `Screen` would collide with a hypothetical future primitive under the same
// short name — and reuses `sidebar_project_header::HEIGHT_MAC`/
// `HEIGHT_OTHER`/`TRAFFIC_LIGHTS_WIDTH` directly rather than re-deriving
// them: `nav-stack.tsx`'s own comment ties its header's height and padding
// to `SidebarProjectHeader`'s. See the module docs.
//
// **Borderline call (item's brief, §"genuinely arguable"), decided here.**
// `nav_stack` wraps `sidebar_carousel`'s own scroll track as its `children`
// and sizes its pushed-screen header off `sidebar_project_header`'s own
// constants — both siblings in this module, not primitives — so unlike a
// generic "push/pop screen stack" widget a foreign UI kit might ship, this
// one is built specifically to sit where `SidebarProjectHeader` normally
// does. Filed as a surface, grouped here rather than flat at `super::`
// because both of its real dependencies live in this module.
pub mod nav_stack;
