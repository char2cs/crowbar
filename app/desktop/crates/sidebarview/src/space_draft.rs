//! The create-space form: what has been typed and chosen, and the command it makes.

use gpui::{Entity, Subscription};
use sidebar::{Command, HostId, SpaceIcon, base};
use ui::TextInput;

use crate::icon_editor::IconEditor;

#[derive(Debug)]
pub(crate) struct SpaceDraft {
    pub(crate) name: Entity<TextInput>,
    pub(crate) folder: String,
    pub(crate) host: usize,
    pub(crate) icon: SpaceIcon,
    pub(crate) editor: Entity<IconEditor>,
    pub(crate) _subscriptions: Vec<Subscription>,
}

pub(crate) fn create_command(
    (name, folder): (&str, &str),
    host: HostId,
    icon: SpaceIcon,
) -> Command {
    let folder = folder.trim().to_owned();
    let typed = name.trim();
    let name = Some(typed).filter(|typed| !typed.is_empty()).map_or_else(
        || base(folder.trim_end_matches('/')).to_owned(),
        str::to_owned,
    );
    if icon == SpaceIcon::Default {
        return Command::CreateSpace { name, folder, host };
    }
    Command::CreateSpaceWithIcon {
        name,
        folder,
        host,
        icon,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_name_falls_back_to_the_folder_and_an_icon_picks_the_richer_command() {
        let host = HostId::from("local");
        let plain = |name: &str, folder: &str| Command::CreateSpace {
            name: name.into(),
            folder: folder.into(),
            host: host.clone(),
        };
        let rocket = SpaceIcon::Emoji("🚀".into());
        let cases = [
            (
                "typed name",
                ("  Work ", "/w/app"),
                SpaceIcon::Default,
                plain("Work", "/w/app"),
            ),
            (
                "folder name",
                ("", "/w/app/"),
                SpaceIcon::Default,
                plain("app", "/w/app/"),
            ),
            (
                "with icon",
                ("Work", "/w/app"),
                rocket.clone(),
                Command::CreateSpaceWithIcon {
                    name: "Work".into(),
                    folder: "/w/app".into(),
                    host: host.clone(),
                    icon: rocket,
                },
            ),
        ];
        for (name, inputs, icon, want) in cases {
            assert_eq!(create_command(inputs, host.clone(), icon), want, "{name}");
        }
    }
}
