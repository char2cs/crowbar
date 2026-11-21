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
    Axis, Button, ButtonSize, ButtonVariant, Checkbox, DragHandle, DraggedRow, Sash, SashDrag,
    Separator, Switch, TabItem, Tabs, TabsStyle, TextArea, TextAreaCondition, TextAreaSize,
    TextInput, TextInputEvent, TextInputStyle,
};
pub use display::{
    Badge, BadgeSize, BadgeVariant, EmptyState, FlickerSpinner, Kbd, ProgressBar, ScrollArea,
    Skeleton, Spinner, Toast, ToastContent, ToastId, ToastKind, ToastLifetime, ToastPlacement,
    ToastQueue, ToastQueueEvent, ToastSide,
};
pub use error::Error;
pub use floating::{
    Backdrop, ContextMenu, Dialog, DialogExtent, DialogSize, DialogStyle, DragGhost, DropdownMenu,
    Menu, MenuAction, MenuItem, Placement, Popover, Portal, Select, Tooltip, TooltipSide, TopLayer,
    click_away, popup_at,
};
pub use foundation::{
    Appearance, CaretBlink, CubicBezier, Glide, HoverFill, Icon, IconName, IconWeight,
    MaterialIcon, Motion, Palette, Pose, ProviderGlyph, ScaledText, Surface, Theme, ThemeId,
    ThemeMode, Vibrancy, Wordmark, assets, focus_ring,
};
pub use foundation::{font, layer, radius, shadow, spacing, text_size, timing};
pub use ui::init;
