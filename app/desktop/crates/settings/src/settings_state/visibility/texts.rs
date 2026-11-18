//! The searchable text of rows and blocks.

use crate::schema::Block;
use crate::schema::Row;
use crate::schema::Section;

pub(crate) fn words(query: &str) -> Vec<String> {
    query.split_whitespace().map(str::to_lowercase).collect()
}

pub(crate) fn row_texts(
    row: &Row,
    section: &Section,
) -> String {
    let mut texts = vec![row.label.as_str(), section.title.as_str()];
    texts.extend(row.description.as_deref());
    texts.extend(row.keywords.iter().map(String::as_str));
    texts.join("\n")
}

pub(crate) fn block_text(block: &Block) -> String {
    match block {
        Block::Heading(heading) => format!("{}\n{}", heading.title, heading.description),
        Block::List(list) => format!(
            "{}\n{}",
            list.title.as_deref().unwrap_or_default(),
            list.intro.as_deref().unwrap_or_default()
        ),
        Block::Button(button) => button.label.clone(),
        Block::Section(_) | Block::Shortcuts(_) => String::new(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn words_are_lowercased_and_split_on_whitespace() {
        assert_eq!(words("  Font   SIZE "), ["font", "size"]);
        assert_eq!(words("   "), Vec::<String>::new());
    }

    #[test]
    fn a_section_block_has_no_text_of_its_own() {
        assert_eq!(block_text(&Block::Section(Section::new("Fonts"))), "");
    }
}
