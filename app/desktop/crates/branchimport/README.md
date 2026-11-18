# branchimport

## Owns

The branch import dialog over a `store::BranchImport`: the remote-branch picker with its filter,
rows and hints, and the import button.

## Public interface

- `BranchImportView`: the dialog; it emits `ImportEvent`.

## Depends on

- `store`: `BranchImport`, `BranchListing`, `ImportBranch`, `ImportPlan`, `Availability`.
- `ui`: the design system.
- `gpui`.
