//! The icon of a file in the file tree.

use ui::IconName;

const README_STEM: &str = "readme";
const DOCUMENT_EXTENSIONS: [&str; 5] = ["md", "mdx", "txt", "rst", "pdf"];

pub(crate) fn file_icon(name: &str) -> IconName {
    let (stem, extension) = name.rsplit_once('.').unwrap_or((name, ""));
    if stem.eq_ignore_ascii_case(README_STEM) {
        return IconName::Info;
    }
    if DOCUMENT_EXTENSIONS.contains(&extension.to_ascii_lowercase().as_str()) {
        return IconName::FileText;
    }
    IconName::Code
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_name_picks_its_icon() {
        const CASES: [(&str, IconName); 8] = [
            ("README.md", IconName::Info),
            ("readme", IconName::Info),
            ("ReadMe.txt", IconName::Info),
            ("NOTES.md", IconName::FileText),
            ("guide.PDF", IconName::FileText),
            ("main.rs", IconName::Code),
            (".gitignore", IconName::Code),
            ("README.rs.bak", IconName::Code),
        ];
        for (name, icon) in CASES {
            assert_eq!(file_icon(name), icon, "{name}");
        }
    }
}
