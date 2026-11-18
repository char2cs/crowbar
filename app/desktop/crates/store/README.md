# store

## Owns

The live mirror of every followed daemon host (entities, streams, the Files and Changes panels),
the sidebar's reads and commands, the settings core, and the data behind the oobe, pairing,
branch import and folder picker dialogs. It never depends on gpui.

## Public interface

- `ShellData`: `start(handle, transport, hosts, ShellWiring)`, then `view()` (`SidebarView`),
  `commands()` (`SidebarCommands`), `settings()` (`SettingsAdapter`), `pairing()`,
  `branch_import()`, `folder_browse()`, `system_check()`.
- `Error`, `Result`, `ChangeSignal`, `Outcome` and `OutcomeSink`.
- Hosts: `HostSpec`, `HostDesk`, `HostBook`, `BookFuture`, `BookError`, `HostNotes`,
  `PairingRequest`; chats' terminals: `ChatTerminal`, `ChatPhase`, `ChatSurface`.
- Files panel commands: `FileDone`, `FileError`, `Overwrite`, `TemplateOutcome`.
- Settings: `SettingsCore`, `SettingsAdapter`, `SystemFonts`, `LegacyImport`.
- Dialog data: `PairingSession`, `PairingState`, `PairingStage`, `PairingPending`;
  `BranchImport`, `BranchListing`, `ImportBranch`, `ImportPlan`, `Availability`; `FolderBrowse`,
  `ROOT_PATH`, `FolderChoice`, `FolderListing`, `BrowseEntry` (and `EntryKind`); `SystemCheck`,
  `Tool`, `ToolReport`.

## Depends on

- `protocol`: the daemon transport port.
- `daemonapi`: every daemon request and the event stream.
- `sidebar` and `settings`: the source and sink traits it implements.
- `atomicfile`: the host notes and settings files.
