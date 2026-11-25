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
    Badge, BadgeSize, BadgeVariant, EmptyState, FlickerSpinner, Kbd, ScrollArea, Skeleton, Spinner,
    Toast, ToastContent, ToastId, ToastKind, ToastLifetime, ToastPlacement, ToastQueue,
    ToastQueueEvent, ToastSide,
};
pub use error::Error;
pub use floating::{
    Backdrop, ContextMenu, Dialog, DialogExtent, DialogSize, DialogStyle, DragGhost, DropdownMenu,
    HeaderFlow, Menu, MenuAction, MenuItem, Placement, Popover, Portal, Select, TabPreview,
    Tooltip, TooltipSide, TopLayer, click_away, popup_at,
};
pub use foundation::{
    Appearance, Breakpoint, CaretBlink, ChoiceKind, ChooseFiles, CubicBezier, FileChoice,
    FileChooser, FileFilter, Glide, HoverFill, Icon, IconName, IconWeight, MaterialIcon,
    MaterialTable, Motion, Palette, Pose, ProviderGlyph, ScaledText, Surface, Theme, ThemeId,
    ThemeMode, Vibrancy, Wordmark, app_window_width, assets, focus_ring,
};
pub use foundation::{font, layer, radius, shadow, spacing, text_size, timing, tracking};
pub use ui::init;
