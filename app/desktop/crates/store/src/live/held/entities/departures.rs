use crate::live::held::entities::{ChatRecord, Entities};

impl Entities {
    pub(crate) fn take_departed(&mut self) -> Vec<String> {
        std::mem::take(&mut self.departed)
    }

    pub(crate) fn take_departed_projects(&mut self) -> Vec<String> {
        std::mem::take(&mut self.departed_projects)
    }

    pub(crate) fn depart_all(&mut self) -> (Vec<String>, Vec<String>) {
        self.prune_chats(|_, _| false);
        self.prune_projects(|_| false);
        (self.take_departed(), self.take_departed_projects())
    }

    pub(super) fn forget_project(
        &mut self,
        id: &str,
    ) {
        if self.projects.get(id).is_some() {
            self.departed_projects.push(id.to_owned());
        }
        self.projects.remove(id);
    }

    pub(super) fn prune_projects(
        &mut self,
        kept: impl Fn(&str) -> bool,
    ) {
        let departed = &mut self.departed_projects;
        self.projects.retain(|id, _| {
            let keep = kept(id);
            departed.extend((!keep).then(|| id.to_owned()));
            keep
        });
    }

    pub(super) fn forget_chat(
        &mut self,
        id: &str,
    ) {
        if self.chats.get(id).is_some() {
            self.departed.push(id.to_owned());
        }
        self.chats.remove(id);
    }

    pub(super) fn prune_chats(
        &mut self,
        kept: impl Fn(&str, &ChatRecord) -> bool,
    ) {
        let departed = &mut self.departed;
        self.chats.retain(|id, row| {
            let keep = kept(id, row);
            departed.extend((!keep).then(|| id.to_owned()));
            keep
        });
    }
}

#[cfg(test)]
mod tests {
    use daemonapi::Frame;

    use super::*;

    const CHAT: &str =
        r#"{"t":"entity","scope":"projects","kind":"chat","id":"c","v":1,"data":{"id":"c"}}"#;
    const NEWER: &str =
        r#"{"t":"entity","scope":"projects","kind":"chat","id":"c","v":2,"data":{"id":"c"}}"#;
    const GONE: &str =
        r#"{"t":"entity","scope":"projects","kind":"chat","id":"c","v":3,"gone":true}"#;
    const UNKNOWN_GONE: &str =
        r#"{"t":"entity","scope":"projects","kind":"chat","id":"x","v":1,"gone":true}"#;
    const SYNCED: &str = r#"{"t":"synced","scope":"projects","unread":[]}"#;
    const PROJECT: &str = r#"{"t":"entity","scope":"projects","kind":"project","id":"p","v":1,"data":{"id":"p","name":"P"}}"#;
    const PROJECT_GONE: &str =
        r#"{"t":"entity","scope":"projects","kind":"project","id":"p","v":2,"gone":true}"#;

    fn heard(texts: &[&str]) -> Entities {
        let mut store = Entities::default();
        texts
            .iter()
            .filter_map(|text| Frame::decode(text).ok())
            .for_each(|frame| store.apply(frame));
        store
    }

    fn resnapshot() -> Entities {
        let mut store = heard(&[CHAT]);
        store.begin_snapshot();
        store.apply(Frame::decode(SYNCED).unwrap());
        store
    }

    #[test]
    fn a_chat_the_daemon_deleted_is_named_once() {
        let cases = [
            ("a removal", heard(&[CHAT, GONE]), "c"),
            ("a snapshot that leaves it out", resnapshot(), "c"),
            ("a newer version", heard(&[CHAT, NEWER]), ""),
            (
                "a removal of a chat never heard of",
                heard(&[UNKNOWN_GONE]),
                "",
            ),
        ];
        for (name, mut store, expected) in cases {
            assert_eq!(store.take_departed().join(","), expected, "{name}");
            assert!(store.take_departed().is_empty(), "{name}: named once");
        }
    }

    #[test]
    fn a_project_the_daemon_deleted_is_named_once() {
        let mut dropped = heard(&[PROJECT]);
        dropped.begin_snapshot();
        dropped.apply(Frame::decode(SYNCED).unwrap());
        let cases = [
            ("a removal", heard(&[PROJECT, PROJECT_GONE]), "p"),
            ("a snapshot that leaves it out", dropped, "p"),
            ("a project still listed", heard(&[PROJECT]), ""),
        ];
        for (name, mut store, expected) in cases {
            assert_eq!(store.take_departed_projects().join(","), expected, "{name}");
            assert!(
                store.take_departed_projects().is_empty(),
                "{name}: named once"
            );
        }
    }
}
