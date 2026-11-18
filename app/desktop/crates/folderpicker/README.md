# folderpicker

## Owns

The tree of a paired host's folders and the choice of one, over a `store::FolderBrowse`.

## Public interface

- `View`: the picker; it emits `PickerEvent` (`Chosen`, `Cancelled`).

## Depends on

- `store`: `FolderBrowse`, `FolderListing`, `BrowseEntry`, `EntryKind`, `ROOT_PATH`.
- `ui`: the design system.
- `gpui`.
