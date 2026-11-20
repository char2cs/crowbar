//! The search index of each page: what a settings search finds a page by. One entry per line:
//! section, label, description and comma-separated keywords, split by `|`.
//! Spec: web/src/features/settings/config/search-index.ts.

use crate::schema::SearchEntry;

pub(super) const EDITOR: &str = "\
Saving|Auto Save|Automatically save files when editing|save, automatic, files\n\
Typography|Editor Font Family|Font family for code editor|font, editor, code, typography, family, monospace\n\
Typography|Font Size|Editor font size in pixels|font, size, editor, typography\n\
Markdown|Markdown Font Size|Base text size for the rich markdown editor, in pixels|markdown, md, font, size, text, typography, plate, document\n\
Typography|Line Height|Editor line height multiplier|line, height, spacing, editor, typography\n\
Typography|Tab Size|Number of spaces per tab|tab, size, indentation, spaces\n\
Display|Word Wrap|Wrap lines that exceed viewport width|word, wrap, lines, display\n\
Display|Line Numbers|Show line numbers in the editor|line, numbers, gutter, display\n\
Display|Render Whitespace|Show visible markers for spaces and tabs|whitespace, spaces, tabs, invisible, render, display\n\
Display|Indent Guides|Show vertical guides for indentation levels|indent, guides, indentation, tabs, spaces, display\n\
Display|Semantic Highlighting|Color functions, types, and properties using language servers and grammars|semantic, highlighting, syntax, colors, tokens, display\n\
Display|Highlight Occurrences|Highlight visible matches for the word under the cursor|highlight, occurrences, matches, word, symbol, cursor\n\
Display|Show Minimap|Show a minimap overview of the code on the right side of the editor|minimap, overview, code, preview, display\n\
Tabs|Max Open Tabs|Maximum number of tabs before oldest closes|tabs, limit, maximum, open\n\
Code Formatting|Format on Save|Format with the language server when saving (manual saves only)|format, save, automatic, formatting, lsp\n\
Code Formatting|Inline Blame|Show who last changed the cursor's line, and when|blame, git, author, annotate\n\
IntelliSense|Auto Completion|Show completion suggestions while typing|autocomplete, completion, intellisense, suggestions\n\
IntelliSense|Parameter Hints|Show function parameter hints|parameter, hints, intellisense, function";

pub(super) const FILES: &str = "\
Display|Indent Size|Pixels per nesting level in the file tree|indent, indentation, nesting, depth, tree, file explorer, spacing\n\
Display|Show Hidden Files|Show dotfiles and hidden directories in the file tree|hidden, dotfiles, dotfile, files, folders, directories, visibility\n\
Display|Compact Folders|Collapse single-child folder chains in the file tree|compact, folders, directories, single child, collapse, nested\n\
Display|Density|Choose file tree row spacing|density, spacing, compact, comfortable, rows, height\n\
Display|Respect .gitignore|Hide files matched by root and nested .gitignore files|gitignore, .gitignore, ignored, git, files, visibility, exclude\n\
Filters|Hidden File Patterns|Files matching these glob patterns will be hidden from the file tree|hidden, files, patterns, glob, filter, exclude\n\
Filters|Hidden Directory Patterns|Directories matching these glob patterns will be hidden from the file tree|hidden, directories, folders, patterns, glob, filter, exclude";

pub(super) const GIT: &str = "\
File Tree|Show Git Status In File Tree|Display Git decorations in Files|git, file tree, files, file explorer, decorations, status\n\
Git View|Compact Git Status Badges|Use denser diff stats and staged indicators in the Git panel|git, compact, badges, status, dense";

pub(super) const APPEARANCE: &str = "\
Theme|Color Theme|Choose your preferred color theme|theme, color, appearance, dark, light\n\
Theme|Icon Theme|Icons displayed in the file tree and tabs|icon, theme, file, tree, tabs\n\
Typography|UI Font Family|Font family for UI elements (file tree, markdown, etc.)|font, ui, typography, family\n\
Typography|UI Font Size|Adjust UI text and icon scale in 0.5px steps|font, size, ui, icon, scale, typography\n\
Layout|Sidebar Position|Choose where to position the sidebar|sidebar, position, left, right, layout\n\
Layout|Tab list|List the tabs of the current project, or every tab open on this client|tab, tabs, list, strip, project, host, all\n\
Layout|Sidebar follows focus|Switch the sidebar between Chats, Files and Git as focus moves|sidebar, focus, follow, files, git, chats, mode";

pub(super) const AGENTS: &str = "\
Agents|Agent Priority|Drag to set which agent a new chat opens first|provider, providers, agent, agents, cli, claude, codex, priority, order, reorder, default\n\
Agents|Enable Agent|Turn an agent off to hide it from every New chat surface|provider, providers, agent, agents, cli, claude, codex, enable, disable, off, hide\n\
Agents|Crowbar Tools|Let an agent use Crowbar itself — your workspaces and review comments|provider, providers, agent, agents, cli, claude, codex, tools, mcp, access, permission\n\
Chat|Chat By Default|Open a chat on Chat rather than Terminal — both stay available either way|chat, terminal, default, presentation, surface, landing, agent, agents, provider";

pub(super) const TERMINAL: &str = "\
Typography|Terminal Font Family|Font family for the integrated terminal|terminal, font, family, monospace, nerd font\n\
Typography|Terminal Font Size|Terminal font size in pixels|terminal, font, size\n\
Typography|Terminal Line Height|Line height multiplier for terminal|terminal, line, height, spacing\n\
Typography|Terminal Letter Spacing|Additional spacing between characters in terminal|terminal, letter, spacing, characters\n\
Typography|Terminal Scrollback|How many terminal history lines to retain|terminal, scrollback, history, buffer, lines\n\
Cursor|Terminal Cursor Style|Shape of the terminal cursor|terminal, cursor, style, block, underline, bar\n\
Cursor|Terminal Cursor Blink|Whether the terminal cursor should blink|terminal, cursor, blink, blinking\n\
Cursor|Terminal Cursor Width|Thickness of the terminal cursor|terminal, cursor, width, thickness, bar";

pub(super) const BROWSER: &str = "\
Navigation|Homepage|Page a new browser tab opens on; leave empty for a blank address bar|browser, homepage, home, start, new tab, url, web\n\
Navigation|Search Engine|Used when the address bar text is not a web address|browser, search, engine, google, duckduckgo, bing, brave, address\n\
Navigation|Custom Search URL|Search address for the custom engine, with %s where the query goes|browser, search, custom, engine, url, template, kagi\n\
Display|Page Zoom|Default zoom for pages, in percent; applies to tabs opened afterwards|browser, zoom, scale, size, magnify, page\n\
Privacy|Clear Browsing Data|Delete cookies, cache and site storage of every browser tab|browser, clear, cookies, cache, history, privacy, storage, delete";

pub(super) fn entries(table: &str) -> Vec<SearchEntry> {
    table.lines().filter_map(entry).collect()
}

fn entry(line: &str) -> Option<SearchEntry> {
    let mut fields = line.split('|');
    let (section, label, description) = (fields.next()?, fields.next()?, fields.next()?);
    let keywords: Vec<&str> = fields.next()?.split(", ").collect();
    Some(SearchEntry::new(section, label, description).keywords(&keywords))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_line_of_every_table_is_one_entry() {
        for table in [APPEARANCE, EDITOR, FILES, GIT, TERMINAL, BROWSER, AGENTS] {
            assert_eq!(entries(table).len(), table.lines().count());
        }
        let first = entries(EDITOR).remove(0);
        assert_eq!(
            first,
            SearchEntry::new(
                "Saving",
                "Auto Save",
                "Automatically save files when editing"
            )
            .keywords(&["save", "automatic", "files"])
        );
    }
}
