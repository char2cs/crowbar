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
    ArrowUUpLeft => "arrow-u-up-left",
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
    FolderPlus => "folder-plus",
    CaretDoubleUp => "caret-double-up",
    Image => "image",
    Link => "link",
    FileText => "file-text",
    File => "file",
    FolderSimplePlus => "folder-simple-plus",
    FolderMinus => "folder-minus",
    Scissors => "scissors",
    CopySimple => "copy-simple",
    Cloud => "cloud",
    Desktop => "desktop",
    Pencil => "pencil",
    Upload => "upload",
    Smiley => "smiley",
    Star => "star",
    Minus => "minus",
    ArrowsOutCardinal => "arrows-out-cardinal",
    XCircle => "x-circle",
    Funnel => "funnel",
    Eye => "eye",
    UploadSimple => "upload-simple",
    PencilSimple => "pencil-simple",
    SidebarToggle => "sidebar-toggle",
    SidebarToggleRight => "sidebar-toggle-right",
    Crosshair => "crosshair",
    ArrowRight => "arrow-right",
}

#[cfg(test)]
mod tests {
    use std::collections::HashSet;

    use super::*;

    #[test]
    fn stems_are_unique_kebab_case() {
        let stems: HashSet<_> = IconName::ALL.iter().map(|name| name.stem()).collect();
        assert_eq!(stems.len(), IconName::ALL.len());
        assert!(stems.iter().all(|stem| {
            stem.chars()
                .all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-')
        }));
    }

    #[test]
    fn all_lists_known_icons() {
        assert!(IconName::ALL.contains(&IconName::Check));
        assert_eq!(IconName::Check.stem(), "check");
    }
}
