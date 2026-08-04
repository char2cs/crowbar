//! Command registry.
//!
//! Ported from `web/src/features/keymaps/registry.ts`. A finite, knowable set
//! of commands extracted from the keyboard hooks that already exist in the
//! codebase. Each command records its default chord and whether the live
//! binding is resolved from the registry ([`Command::live_editable`]) or
//! still hardcoded in a hook (display-only, shown read-only in settings).
//!
//! Source hooks (Phase 4 / GPUI action dispatch, not ported here — see
//! `native/mapping/core-keymap.md`):
//!  - `features/panes/hooks/use-pane-keyboard.ts`   (Panes — live-editable)
//!  - `features/keymaps/hooks/use-save-keyboard.ts` (Editor — display-only)
//!
//! # A shape difference from the TS source
//!
//! `getCommand` in TS is backed by a module-level `Map` built once from
//! `COMMANDS.map(...)`. [`COMMANDS`] here is a plain `const` array of 20
//! entries; [`get_command`] does a linear scan rather than building an index.
//! A `HashMap`/`LazyLock` index would just relocate the TS `Map`'s
//! precomputation into a different kind of hidden global for a 20-entry
//! table where a scan is not a real cost — and this crate's `workspace`
//! module already documents its own reluctance to introduce a global for a
//! problem this small (see `crate::workspace`'s module doc).
//!
//! **A correction to `tier-a-denominator.md` §3**, which calls this "the
//! static 19-command table": counting `id:` occurrences directly in the
//! source (`awk '/^export const COMMANDS/,/^\]/' registry.ts | grep -c
//! '^\s*id:'`) gives 20, not 19. [`CATEGORY_ORDER`] and every test below use
//! the real count.

use super::types::{Command, CommandCategory};

pub const PANE_NAVIGATE_LEFT: &str = "panes.navigateLeft";
pub const PANE_NAVIGATE_RIGHT: &str = "panes.navigateRight";
pub const PANE_NAVIGATE_UP: &str = "panes.navigateUp";
pub const PANE_NAVIGATE_DOWN: &str = "panes.navigateDown";
pub const PANE_SPLIT_RIGHT: &str = "panes.splitRight";
pub const PANE_SPLIT_DOWN: &str = "panes.splitDown";
pub const TAB_NEW: &str = "tabs.new";
pub const TAB_NEW_TERMINAL: &str = "tabs.newTerminal";
pub const TAB_NEW_FILE: &str = "tabs.newFile";
pub const AGENT_NEW_CHAT: &str = "agent.newChat";
pub const AGENT_CYCLE_PROVIDER: &str = "agent.cycleProvider";
pub const TAB_REOPEN_CLOSED: &str = "tabs.reopenClosed";
pub const TAB_CLOSE: &str = "tabs.closeActive";

pub const EDITOR_SAVE: &str = "editor.save";
pub const EDITOR_SAVE_ALL: &str = "editor.saveAll";

pub const OPEN_WORKSPACE_SWITCHER: &str = "navigation.openWorkspaceSwitcher";

pub const SIDEBAR_TAB_WORKSPACES: &str = "navigation.sidebarWorkspaces";
pub const SIDEBAR_TAB_FILES: &str = "navigation.sidebarFiles";
pub const SIDEBAR_TAB_GIT: &str = "navigation.sidebarGit";
pub const SIDEBAR_TAB_CHATS: &str = "navigation.sidebarChats";

/// The order the Keybindings settings tab renders categories in — and,
/// because it renders ONLY what is listed here, the set of categories that
/// are visible at all. It lives beside [`COMMANDS`] rather than in the
/// settings component so that adding a category to a command and forgetting
/// to render it cannot silently hide the command: a test in this module
/// asserts every category in use appears here.
pub const CATEGORY_ORDER: [CommandCategory; 5] = [
    CommandCategory::Navigation,
    CommandCategory::Panes,
    CommandCategory::Tabs,
    CommandCategory::Chats,
    CommandCategory::Editor,
];

pub const COMMANDS: [Command; 20] = [
    // --- Panes (live-editable, resolved from registry) ---
    Command {
        id: PANE_SPLIT_RIGHT,
        label: "Split editor right",
        category: CommandCategory::Panes,
        default_chord: "mod+\\",
        live_editable: true,
    },
    Command {
        id: PANE_SPLIT_DOWN,
        label: "Split editor down",
        category: CommandCategory::Panes,
        default_chord: "mod+shift+\\",
        live_editable: true,
    },
    Command {
        id: PANE_NAVIGATE_LEFT,
        label: "Focus pane left",
        category: CommandCategory::Panes,
        default_chord: "mod+alt+arrowleft",
        live_editable: true,
    },
    Command {
        id: PANE_NAVIGATE_RIGHT,
        label: "Focus pane right",
        category: CommandCategory::Panes,
        default_chord: "mod+alt+arrowright",
        live_editable: true,
    },
    Command {
        id: PANE_NAVIGATE_UP,
        label: "Focus pane up",
        category: CommandCategory::Panes,
        default_chord: "mod+alt+arrowup",
        live_editable: true,
    },
    Command {
        id: PANE_NAVIGATE_DOWN,
        label: "Focus pane down",
        category: CommandCategory::Panes,
        default_chord: "mod+alt+arrowdown",
        live_editable: true,
    },
    // --- Tabs (live-editable, resolved from registry) ---
    Command {
        id: TAB_NEW,
        label: "New tab",
        category: CommandCategory::Tabs,
        default_chord: "mod+t",
        live_editable: true,
    },
    Command {
        // Moved off mod+t, which is now New Tab.
        id: TAB_NEW_TERMINAL,
        label: "New terminal tab",
        category: CommandCategory::Tabs,
        default_chord: "mod+j",
        live_editable: true,
    },
    Command {
        // Cmd+N is the chat, not the file: starting a conversation is the
        // thing this app is opened to do, so it takes the unshifted chord
        // and New File takes Cmd+Shift+N. (They were the other way round
        // through the New Tab work.)
        id: AGENT_NEW_CHAT,
        label: "New chat",
        category: CommandCategory::Tabs,
        default_chord: "mod+n",
        live_editable: true,
    },
    Command {
        id: TAB_NEW_FILE,
        label: "New file",
        category: CommandCategory::Tabs,
        default_chord: "mod+shift+n",
        live_editable: true,
    },
    Command {
        // Cycles the OPEN chat to the next enabled provider, the way
        // Cmd-tab cycles apps. It only does anything while a chat pane is
        // the focused, visible one (agent-chat-pane gates it) — elsewhere
        // the chord is free for other uses.
        //
        // Deliberately NOT mod+`, the obvious chord for this. macOS
        // reserves it for "move focus to next window in application" and
        // consumes it in AppKit before WKWebView is reached, so it never
        // arrives as a keydown at all — invisible to this command AND to
        // the settings rebind capture, which is how it presented: a
        // shortcut that did nothing and could not even be reassigned.
        //
        // mod+/ collides with "toggle comment" in editors, which is fine:
        // this fires only while a CHAT pane is focused, never over the
        // editor.
        id: AGENT_CYCLE_PROVIDER,
        label: "Cycle chat provider",
        category: CommandCategory::Chats,
        default_chord: "mod+/",
        live_editable: true,
    },
    Command {
        id: TAB_REOPEN_CLOSED,
        label: "Reopen last closed tab",
        category: CommandCategory::Tabs,
        default_chord: "mod+shift+t",
        live_editable: true,
    },
    Command {
        id: TAB_CLOSE,
        label: "Close active tab",
        category: CommandCategory::Tabs,
        default_chord: "mod+w",
        live_editable: true,
    },
    // --- Editor (display-only: still hardcoded in use-save-keyboard.ts) ---
    Command {
        id: EDITOR_SAVE,
        label: "Save",
        category: CommandCategory::Editor,
        default_chord: "mod+s",
        live_editable: false,
    },
    Command {
        id: EDITOR_SAVE_ALL,
        label: "Save all",
        category: CommandCategory::Editor,
        default_chord: "mod+shift+s",
        live_editable: false,
    },
    // --- Navigation (live-editable) ---
    Command {
        id: OPEN_WORKSPACE_SWITCHER,
        label: "Open workspace switcher",
        category: CommandCategory::Navigation,
        default_chord: "mod+k",
        live_editable: true,
    },
    // The digit IS the tab's position in the sidebar strip — mod+2 is
    // whatever sits second. Keep these four in strip order, and renumber
    // them when the strip is reordered: a number that doesn't match what
    // the user is looking at is a shortcut they will never find.
    Command {
        id: SIDEBAR_TAB_WORKSPACES,
        label: "Sidebar: Workspaces",
        category: CommandCategory::Navigation,
        default_chord: "mod+1",
        live_editable: true,
    },
    Command {
        id: SIDEBAR_TAB_CHATS,
        label: "Sidebar: Chats",
        category: CommandCategory::Navigation,
        default_chord: "mod+2",
        live_editable: true,
    },
    Command {
        id: SIDEBAR_TAB_FILES,
        label: "Sidebar: Files",
        category: CommandCategory::Navigation,
        default_chord: "mod+3",
        live_editable: true,
    },
    Command {
        id: SIDEBAR_TAB_GIT,
        label: "Sidebar: Git",
        category: CommandCategory::Navigation,
        default_chord: "mod+4",
        live_editable: true,
    },
];

/// Mirrors `getCommand`.
#[must_use]
pub fn get_command(id: &str) -> Option<&'static Command> {
    COMMANDS.iter().find(|c| c.id == id)
}

#[cfg(test)]
mod tests {
    use super::{
        AGENT_CYCLE_PROVIDER, AGENT_NEW_CHAT, CATEGORY_ORDER, COMMANDS, TAB_NEW, TAB_NEW_FILE,
        TAB_NEW_TERMINAL, get_command,
    };
    use crate::keymap::types::CommandCategory;
    use std::collections::HashSet;

    // --- ported from web/src/__tests__/features/keymaps/registry.test.ts ---

    #[test]
    fn binds_mod_t_to_new_tab_not_new_terminal() {
        assert_eq!(get_command(TAB_NEW).unwrap().default_chord, "mod+t");
    }

    #[test]
    fn moves_new_terminal_to_mod_j() {
        assert_eq!(
            get_command(TAB_NEW_TERMINAL).unwrap().default_chord,
            "mod+j"
        );
    }

    // The unshifted chord belongs to the chat — starting a conversation is
    // the thing this app is opened to do — and New File takes the shifted
    // one. They shipped the other way round; this pins the swap.
    #[test]
    fn binds_new_chat_to_mod_n_and_new_file_to_mod_shift_n() {
        assert_eq!(get_command(AGENT_NEW_CHAT).unwrap().default_chord, "mod+n");
        assert_eq!(
            get_command(TAB_NEW_FILE).unwrap().default_chord,
            "mod+shift+n"
        );
    }

    // Cycles the open chat to the next enabled provider, the way Cmd-tab
    // cycles apps. Registered like every other chord so it is rebindable
    // rather than a hardcoded literal buried in the chat pane.
    //
    // NOT mod+` — macOS reserves that for "move focus to next window in
    // application" and consumes it in AppKit before WKWebView ever sees it,
    // so the chord was invisible to the handler AND to the rebind capture.
    // Pinned here so nobody restores it.
    #[test]
    fn binds_cycle_chat_provider_to_mod_slash_a_chord_macos_actually_delivers() {
        assert_eq!(
            get_command(AGENT_CYCLE_PROVIDER).unwrap().default_chord,
            "mod+/"
        );
    }

    #[test]
    fn binds_no_command_to_mod_backtick_macos_swallows_it() {
        assert!(COMMANDS.iter().all(|c| c.default_chord != "mod+`"));
    }

    #[test]
    fn files_cycle_chat_provider_under_its_own_chats_category() {
        assert_eq!(
            get_command(AGENT_CYCLE_PROVIDER).unwrap().category,
            CommandCategory::Chats
        );
    }

    // The Keybindings tab renders ONLY the categories listed in
    // CATEGORY_ORDER, so a command in a category missing from it is
    // invisible in settings — bound and working, but unfindable and
    // unrebindable. Every category in use must appear.
    #[test]
    fn renders_every_category_in_use_category_order_covers_them_all() {
        for command in &COMMANDS {
            assert!(CATEGORY_ORDER.contains(&command.category));
        }
    }

    // Every chord the New Tab surface draws must be rebindable, or the
    // badge it renders can never go out of sync with reality by design.
    #[test]
    fn makes_all_five_live_editable() {
        for id in [
            TAB_NEW,
            TAB_NEW_TERMINAL,
            TAB_NEW_FILE,
            AGENT_NEW_CHAT,
            AGENT_CYCLE_PROVIDER,
        ] {
            assert!(get_command(id).unwrap().live_editable);
        }
    }

    #[test]
    fn has_no_duplicate_default_chords() {
        let chords: HashSet<&str> = COMMANDS.iter().map(|c| c.default_chord).collect();
        assert_eq!(chords.len(), COMMANDS.len());
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn has_no_duplicate_command_ids() {
        // The TS suite pins "no duplicate default chords" but never checks
        // ids themselves, even though get_command's linear scan returns the
        // FIRST match — a duplicate id would silently hide every entry
        // after the first, with no test able to tell the difference.
        let ids: HashSet<&str> = COMMANDS.iter().map(|c| c.id).collect();
        assert_eq!(ids.len(), COMMANDS.len());
    }

    #[test]
    fn get_command_returns_none_for_an_unknown_id() {
        assert!(get_command("not.a.real.command").is_none());
    }
}
