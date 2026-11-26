pub(crate) fn of(language: editor::Language) -> &'static str {
    match language.id() {
        "typescriptreact" => "tsx",
        "javascriptreact" => "javascript",
        "embedded_template" => "erb",
        "dotenv" => "bash",
        id => id,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(&str, &str); 4] = [
        ("App.tsx", "tsx"),
        ("a.jsx", "javascript"),
        ("src/a.rs", "rust"),
        ("notes", "text"),
    ];

    #[test]
    fn each_language_names_the_grammar_that_colours_it() {
        for (path, name) in CASES {
            assert_eq!(of(editor::Language::of_path(path)), name, "{path}");
        }
    }
}
