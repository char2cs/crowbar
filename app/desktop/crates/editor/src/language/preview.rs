const SEPARATOR: char = '/';
const EXTENSION: char = '.';
const MARKDOWN: [&str; 2] = ["md", "markdown"];
const HTML: [&str; 2] = ["html", "htm"];

/// How a file's preview tab renders it, as develop's breadcrumb offers one.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Preview {
    /// Rendered Markdown. A Markdown file also has a rich view beside its source.
    Markdown,
    /// The page in a sandboxed frame.
    Html,
}

impl Preview {
    /// The preview `path` has, by its extension; `None` for a file with none (`.mdx` included).
    #[must_use]
    pub fn of_path(path: &str) -> Option<Self> {
        let name = path.rsplit(SEPARATOR).next().unwrap_or(path);
        let (_, extension) = name.rsplit_once(EXTENSION)?;
        let extension = extension.to_lowercase();
        if MARKDOWN.contains(&extension.as_str()) {
            return Some(Self::Markdown);
        }
        HTML.contains(&extension.as_str()).then_some(Self::Html)
    }

    /// Whether a file of `path` opens in its rich view, as develop opens Markdown.
    #[must_use]
    pub fn has_rich_view(path: &str) -> bool {
        Self::of_path(path) == Some(Self::Markdown)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn markdown_and_html_files_have_a_preview_and_only_markdown_a_rich_view() {
        let cases = [
            ("README.md", Some(Preview::Markdown), true),
            ("docs/Guide.MARKDOWN", Some(Preview::Markdown), true),
            ("site/index.html", Some(Preview::Html), false),
            ("old.htm", Some(Preview::Html), false),
            ("page.mdx", None, false),
            ("src/a.ts", None, false),
            ("Makefile", None, false),
            ("dir.md/notes", None, false),
        ];
        for (path, preview, rich) in cases {
            assert_eq!(Preview::of_path(path), preview, "{path}");
            assert_eq!(Preview::has_rich_view(path), rich, "{path}");
        }
    }
}
