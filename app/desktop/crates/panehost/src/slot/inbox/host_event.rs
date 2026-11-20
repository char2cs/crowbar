use browser::PageUpdate;
use protocol::{
    CommandId, ExternalUrl, FsPath, PaneId, PaneState, PaneTitle, PaneTypeId, Params, SettingKey,
    ViewedFile,
};
use serde_json::Value;

/// Something a pane asked for or the host decided, for the shell to act on. The host never
/// changes the layout itself: it reports, and the shell applies commands to the model.
#[derive(Debug, Clone, PartialEq)]
pub enum HostEvent {
    /// A pane set its tab title.
    TitleChanged {
        /// The pane.
        pane: PaneId,
        /// The new title.
        title: PaneTitle,
    },
    /// A pane replaced its persisted state.
    StateChanged {
        /// The pane.
        pane: PaneId,
        /// The new state, at most 64 KB.
        state: PaneState,
    },
    /// A pane reported whether it holds unsaved work.
    DirtyChanged {
        /// The pane.
        pane: PaneId,
        /// Whether closing it would lose work.
        dirty: bool,
    },
    /// A pane's state must be dropped because its type is destroy-on-hide.
    StateDiscarded {
        /// The pane.
        pane: PaneId,
    },
    /// A pane asked the shell to open another pane.
    OpenRequested {
        /// The asking pane.
        from: PaneId,
        /// The type to open.
        type_id: PaneTypeId,
        /// The ids the new pane is identified by.
        params: Params,
    },
    /// A pane or one of the shell's keybindings triggered a command.
    CommandRequested {
        /// The pane the keypress or call came from.
        pane: PaneId,
        /// The command id.
        command: CommandId,
    },
    /// A pane asked the shell to open a web, mail or phone link in the system's application.
    OpenExternal {
        /// The link.
        url: ExternalUrl,
    },
    /// A pane asked the shell to show a path in the system file manager.
    Reveal {
        /// The absolute path.
        path: FsPath,
    },
    /// A page registered and was sent its `mount`; the shell can now send it frames of its own.
    Mounted {
        /// The pane.
        pane: PaneId,
    },
    /// A review pane asked the shell to mark or unmark files of its workspace viewed.
    ReviewViewedSet {
        /// The pane; its chat names the workspace.
        pane: PaneId,
        /// The files and their current signatures.
        files: Vec<ViewedFile>,
        /// Mark them viewed, or clear their marks.
        viewed: bool,
    },
    /// A pane asked the shell to store a setting; the shell writes it and pushes the set back.
    SettingRequested {
        /// The pane.
        pane: PaneId,
        /// The setting, one the `settings` frame carries.
        key: SettingKey,
        /// Its new value.
        value: Value,
    },
    /// The pointer entered or left a pane's page, which GPUI cannot see over a web view.
    HoverChanged {
        /// The pane.
        pane: PaneId,
        /// Whether the pointer is over it.
        hovered: bool,
    },
    /// The pointer moved over a pane's page, which GPUI cannot see over a web view.
    PointerMoved {
        /// The pane.
        pane: PaneId,
        /// Distance from the page's left edge, in logical pixels.
        x: f64,
        /// Distance from the page's top edge, in logical pixels.
        y: f64,
    },
    /// A pane's page took native keyboard focus: the user clicked or tabbed into it.
    PageFocused {
        /// The pane.
        pane: PaneId,
    },
    /// Files from the operating system were dropped on a pane's page.
    FilesDropped {
        /// The pane.
        pane: PaneId,
        /// The dropped files that are absolute paths.
        paths: Vec<FsPath>,
    },
    /// A browser pane's page reported its address, load state or title.
    BrowserPage {
        /// The pane.
        pane: PaneId,
        /// What the engine reported.
        update: PageUpdate,
    },
}
