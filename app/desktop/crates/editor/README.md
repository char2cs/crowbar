# editor

## Owns

What the editor knows about a document without drawing it: the language a path is written in
(develop's `language-id.ts`, with the toolbar chip's display names) and why a document could not
be opened (develop's `openFailure`).

## Public interface

- `Language`: `of_path`, `id`, `display_name`.
- `OpenFailure`: `of` (from the daemon's status and problem code), `title`, `message`.

## Depends on

Nothing.
