#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct PathText<'a> {
    text: &'a str,
}

impl<'a> PathText<'a> {
    pub fn new(text: &'a str) -> Self {
        Self { text }
    }

    pub fn join(
        self,
        name: &str,
    ) -> String {
        if self.text.is_empty() || self.text == "." {
            return name.to_owned();
        }
        format!("{}/{name}", self.text.trim_end_matches('/'))
    }

    pub fn parent(self) -> &'a str {
        self.text.rsplit_once('/').map_or("", |(dir, _)| dir)
    }

    pub fn base(self) -> &'a str {
        self.text.rsplit('/').next().unwrap_or(self.text)
    }

    pub fn absolute(
        self,
        root: Option<&str>,
    ) -> String {
        let Some(root) = root else {
            return self.text.to_owned();
        };
        if self.text.is_empty() || self.text == "." {
            return root.to_owned();
        }
        PathText::new(root).join(self.text)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn paths_join_split_and_anchor_at_the_root() {
        type Case = (&'static str, String, &'static str);
        let cases: [Case; 10] = [
            ("root join", PathText::new("").join("a.rs"), "a.rs"),
            ("nested join", PathText::new("src").join("a.rs"), "src/a.rs"),
            (
                "trailing slash",
                PathText::new("src/").join("a.rs"),
                "src/a.rs",
            ),
            (
                "parent",
                PathText::new("src/a.rs").parent().to_owned(),
                "src",
            ),
            (
                "top level parent",
                PathText::new("a.rs").parent().to_owned(),
                "",
            ),
            ("base", PathText::new("src/a.rs").base().to_owned(), "a.rs"),
            (
                "file",
                PathText::new("src/a.rs").absolute(Some("/w")),
                "/w/src/a.rs",
            ),
            ("root", PathText::new("").absolute(Some("/w")), "/w"),
            (
                "filesystem root",
                PathText::new("a").absolute(Some("/")),
                "/a",
            ),
            ("unknown root", PathText::new("src").absolute(None), "src"),
        ];
        for (name, got, want) in cases {
            assert_eq!(got, want, "{name}");
        }
    }
}
