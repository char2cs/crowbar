//! The icon of a space as the daemon holds it: an emoji, a fetched image or none.

use sidebar::{Mark, ProjectId, SpaceIcon};

use crate::project_record::ProjectRecord;
use crate::repo_mark;
use crate::state::State;

pub(crate) fn of(
    state: &State,
    project: &ProjectId,
) -> SpaceIcon {
    let Some(record) = state
        .hosts
        .iter()
        .find_map(|slot| slot.store.projects.get(project.as_str()))
    else {
        return SpaceIcon::Default;
    };
    if !record.avatar_emoji.is_empty() {
        return SpaceIcon::Emoji(record.avatar_emoji.clone());
    }
    image(state, record).map_or(SpaceIcon::Default, SpaceIcon::Image)
}

pub(crate) fn mark(
    state: &State,
    record: &ProjectRecord,
) -> Mark {
    match image(state, record) {
        Some(path) if record.avatar_emoji.is_empty() => Mark::Image(path.into()),
        _ => repo_mark::of((&record.name, ""), (&record.avatar_emoji, "")),
    }
}

fn image(
    state: &State,
    record: &ProjectRecord,
) -> Option<String> {
    state
        .icons
        .file_for(&record.id, &record.avatar_url)
        .map(|path| path.to_string_lossy().into_owned())
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;

    use super::*;
    use crate::frame::decode;
    use crate::icon_cache::IconCache;
    use crate::test_support::state;

    const URL: &str = "/v0/projects/p/icon?v=1";

    fn with_project(data: &str) -> State {
        let (mut state, _) = state();
        let frame = format!(
            r#"{{"t":"entity","scope":"projects","kind":"project","id":"p","v":9,"data":{data}}}"#
        );
        state.apply("local", decode(&frame).unwrap());
        state
    }

    fn held(state: &mut State) {
        let mut cache = IconCache::in_dir(PathBuf::from("/c"));
        cache.finish("p", URL, Some(PathBuf::from("/c/p-1")));
        state.icons = cache;
    }

    #[test]
    fn a_space_shows_its_emoji_its_fetched_image_or_the_default() {
        let p = ProjectId::from("p");
        let mut plain = with_project(r#"{"id":"p","name":"A"}"#);
        assert_eq!(of(&plain, &p), SpaceIcon::Default);
        let emoji = with_project(r#"{"id":"p","name":"A","avatarEmoji":"🦊"}"#);
        assert_eq!(of(&emoji, &p), SpaceIcon::Emoji("🦊".into()));
        let mut image = with_project(&format!(r#"{{"id":"p","name":"A","avatarUrl":"{URL}"}}"#));
        assert_eq!(of(&image, &p), SpaceIcon::Default, "not fetched yet");
        held(&mut image);
        assert_eq!(of(&image, &p), SpaceIcon::Image("/c/p-1".into()));
        held(&mut plain);
        assert_eq!(of(&plain, &p), SpaceIcon::Default, "no url, no image");
        let record = image.hosts[0].store.projects.get("p").unwrap().clone();
        assert_eq!(mark(&image, &record), Mark::Image("/c/p-1".into()));
    }
}
