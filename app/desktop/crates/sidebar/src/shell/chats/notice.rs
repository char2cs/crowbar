#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Notice {
    Project,
    Repo(String),
}

impl Notice {
    pub fn text(&self) -> String {
        match self {
            Self::Project => "Couldn't load this project's chats".into(),
            Self::Repo(name) => format!("Couldn't load chats in {name}"),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_text_names_the_repository() {
        assert_eq!(
            Notice::Project.text(),
            "Couldn't load this project's chats",
            "project"
        );
        assert_eq!(
            Notice::Repo("demo".into()).text(),
            "Couldn't load chats in demo",
            "repo"
        );
    }
}
