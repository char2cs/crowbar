//! The embedded icon files; generated from the Phosphor assets in `assets/icons`.

use crate::foundation::icon::file::File;
use crate::foundation::icon::icon_name::IconName;
use crate::foundation::icon::icon_weight::IconWeight;

macro_rules! weight_dir {
    (Regular) => {
        "regular"
    };
    (Bold) => {
        "bold"
    };
    (Fill) => {
        "fill"
    };
    (Duotone) => {
        "duotone"
    };
}

macro_rules! files {
    ($(($name:ident, $stem:literal, [$($weight:ident),+])),+ $(,)?) => {
        &[$($(
            File {
                name: IconName::$name,
                weight: IconWeight::$weight,
                path: concat!("icons/", weight_dir!($weight), "/", $stem, ".svg"),
                bytes: include_bytes!(concat!(
                    "../../../assets/icons/",
                    weight_dir!($weight),
                    "/",
                    $stem,
                    ".svg"
                )),
            }
        ),+),+]
    };
}

pub(crate) const FILES: &[File] = files![
    (ChatsCircle, "chats-circle", [Regular, Fill]),
    (FolderOpen, "folder-open", [Regular, Fill, Duotone]),
    (Folder, "folder", [Regular, Duotone, Fill]),
    (GitBranch, "git-branch", [Regular, Fill, Bold, Duotone]),
    (Gear, "gear", [Regular]),
    (Plus, "plus", [Regular, Bold]),
    (Books, "books", [Regular]),
    (Lock, "lock", [Regular, Fill]),
    (LockOpen, "lock-open", [Regular]),
    (GitFork, "git-fork", [Fill]),
    (GitMerge, "git-merge", [Regular, Fill]),
    (GitPullRequest, "git-pull-request", [Regular, Fill, Duotone]),
    (Warning, "warning", [Regular, Fill]),
    (ArrowClockwise, "arrow-clockwise", [Regular, Bold]),
    (
        ArrowElbowDownRight,
        "arrow-elbow-down-right",
        [Regular, Bold]
    ),
    (DotsThree, "dots-three", [Regular, Bold]),
    (LinkBreak, "link-break", [Regular, Bold]),
    (X, "x", [Regular, Bold]),
    (CaretRight, "caret-right", [Regular, Bold]),
    (CaretDown, "caret-down", [Regular, Bold]),
    (CaretUp, "caret-up", [Regular, Bold]),
    (CaretUpDown, "caret-up-down", [Regular, Bold]),
    (Check, "check", [Regular, Bold]),
    (PushPin, "push-pin", [Regular, Fill]),
    (PushPinSlash, "push-pin-slash", [Regular]),
    (WarningCircle, "warning-circle", [Regular, Fill]),
    (CheckCircle, "check-circle", [Regular, Fill]),
    (Info, "info", [Regular, Fill]),
    (CircleNotch, "circle-notch", [Regular, Bold]),
    (Code, "code", [Regular]),
    (SquareSplitHorizontal, "square-split-horizontal", [Regular]),
    (Copy, "copy", [Regular]),
    (PencilSimpleLine, "pencil-simple-line", [Regular]),
    (ArrowCounterClockwise, "arrow-counter-clockwise", [Regular]),
    (ArrowUUpLeft, "arrow-u-up-left", [Bold]),
    (TerminalWindow, "terminal-window", [Regular, Duotone]),
    (Terminal, "terminal", [Regular]),
    (Globe, "globe", [Regular, Duotone]),
    (DownloadSimple, "download-simple", [Regular]),
    (Trash, "trash", [Regular]),
    (MagnifyingGlass, "magnifying-glass", [Regular]),
    (Skull, "skull", [Regular]),
    (Robot, "robot", [Duotone]),
    (Pulse, "pulse", [Duotone]),
    (Keyboard, "keyboard", [Duotone]),
    (HardDrives, "hard-drives", [Regular, Duotone]),
    (PaintBrush, "paint-brush", [Duotone]),
    (CodeBlock, "code-block", [Duotone]),
    (TreeStructure, "tree-structure", [Duotone]),
    (DotsSixVertical, "dots-six-vertical", [Regular, Bold]),
    (ArrowsOutSimple, "arrows-out-simple", [Regular, Bold]),
    (FilePlus, "file-plus", [Regular]),
    (FolderPlus, "folder-plus", [Regular]),
    (CaretDoubleUp, "caret-double-up", [Regular]),
    (Image, "image", [Regular]),
    (Link, "link", [Regular]),
    (FileText, "file-text", [Regular]),
    (File, "file", [Regular]),
    (FolderSimplePlus, "folder-simple-plus", [Regular]),
    (FolderMinus, "folder-minus", [Regular]),
    (Scissors, "scissors", [Regular]),
    (CopySimple, "copy-simple", [Regular]),
    (Cloud, "cloud", [Regular, Fill]),
    (Desktop, "desktop", [Regular, Fill]),
    (Pencil, "pencil", [Regular]),
    (Upload, "upload", [Regular]),
    (Smiley, "smiley", [Regular]),
    (Star, "star", [Regular]),
    (Minus, "minus", [Regular]),
    (ArrowsOutCardinal, "arrows-out-cardinal", [Regular]),
    (XCircle, "x-circle", [Regular]),
    (Crosshair, "crosshair", [Regular]),
    (ArrowRight, "arrow-right", [Regular]),
    (Funnel, "funnel", [Regular]),
    (Eye, "eye", [Regular]),
    (UploadSimple, "upload-simple", [Regular]),
    (PencilSimple, "pencil-simple", [Regular]),
    (SidebarToggle, "sidebar-toggle", [Regular]),
    (SidebarToggleRight, "sidebar-toggle-right", [Regular]),
];
