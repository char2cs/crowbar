//! GPUI design system of the Crowbar shell: theme tokens, fonts, icons, motion and base components.
//!
//! Entry point: [`init`]. See `README.md` for the public API status per component.

mod control;
mod display;
mod error;
mod floating;
mod foundation;
mod test_support;
mod ui;

pub use control::{
    Axis, Button, ButtonSize, ButtonVariant, Checkbox, DragGhost, DragHandle, DraggedRow, Sash,
    SashDrag, Separator, Switch, TabItem, Tabs, TabsStyle, TextArea, TextAreaCondition,
    TextAreaSize, TextInput, TextInputEvent, TextInputStyle,
};
pub use display::{
    Badge, BadgeSize, BadgeVariant, EmptyState, Kbd, ProgressBar, ScrollArea, Skeleton, Spinner,
    Toast, ToastContent, ToastId, ToastKind, ToastLifetime, ToastPlacement, ToastQueue,
    ToastQueueEvent, ToastSide,
};
pub use error::Error;
pub use floating::popup;
pub use floating::{
    ContextMenu, Dialog, DialogExtent, DialogSize, DialogStyle, DropdownMenu, Menu, MenuAction,
    MenuItem, OverlayGuard, OverlayHost, OverlayId, Overlays, Placement, Popover, Select, Tooltip,
    TooltipSide,
};
pub use foundation::{
    Appearance, CubicBezier, Glide, HoverFill, Icon, IconName, IconWeight, Motion, Palette, Pose,
    Surface, Theme, ThemeId, ThemeMode, UiText, Vibrancy, Wordmark, assets, focus_ring,
};
pub use foundation::{font, layer, radius, shadow, spacing, text_size, timing};
pub use ui::init;
