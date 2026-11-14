//! Names of the embedded Phosphor icons.

macro_rules! icon_names {
    ($($variant:ident => $stem:literal),* $(,)?) => {
        /// A Phosphor icon the shell surfaces use.
        #[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
        #[non_exhaustive]
        pub enum IconName {
            $(
                #[doc = concat!("Phosphor `", $stem, "`.")]
                $variant,
            )*
        }

        impl IconName {
            /// Every embedded icon.
            pub const ALL: &'static [IconName] = &[$(IconName::$variant),*];

            /// File stem of the icon inside its weight folder.
            pub fn stem(self) -> &'static str {
                match self {
                    $(IconName::$variant => $stem,)*
                }
            }
        }
    };
}

icon_names! {
    ChatsCircle => "chats-circle",
    FolderOpen => "folder-open",
    Folder => "folder",
    GitBranch => "git-branch",
    Gear => "gear",
    Plus => "plus",
    Books => "books",
    Lock => "lock",
    LockOpen => "lock-open",
    GitFork => "git-fork",
    GitMerge => "git-merge",
    GitPullRequest => "git-pull-request",
    Warning => "warning",
    ArrowClockwise => "arrow-clockwise",
    ArrowElbowDownRight => "arrow-elbow-down-right",
    DotsThree => "dots-three",
    LinkBreak => "link-break",
    X => "x",
    CaretRight => "caret-right",
    CaretDown => "caret-down",
    CaretUp => "caret-up",
    CaretUpDown => "caret-up-down",
    Check => "check",
    PushPin => "push-pin",
    PushPinSlash => "push-pin-slash",
    WarningCircle => "warning-circle",
    CheckCircle => "check-circle",
    Info => "info",
    CircleNotch => "circle-notch",
    Code => "code",
    SquareSplitHorizontal => "square-split-horizontal",
    Copy => "copy",
    PencilSimpleLine => "pencil-simple-line",
    ArrowCounterClockwise => "arrow-counter-clockwise",
    TerminalWindow => "terminal-window",
    Terminal => "terminal",
    Globe => "globe",
    DownloadSimple => "download-simple",
    Trash => "trash",
    MagnifyingGlass => "magnifying-glass",
    Skull => "skull",
    Robot => "robot",
    Pulse => "pulse",
    Keyboard => "keyboard",
    HardDrives => "hard-drives",
    PaintBrush => "paint-brush",
    CodeBlock => "code-block",
    TreeStructure => "tree-structure",
    DotsSixVertical => "dots-six-vertical",
    ArrowsOutSimple => "arrows-out-simple",
    FilePlus => "file-plus",
    Cloud => "cloud",
    Desktop => "desktop",
    Pencil => "pencil",
    Upload => "upload",
    Smiley => "smiley",
    Star => "star",
}
