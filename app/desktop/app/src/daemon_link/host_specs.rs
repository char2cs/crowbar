use store::HostSpec;

use super::DaemonLink;
use super::LOCAL_HOST;

const LOCAL_NAME: &str = "This computer";

pub(super) fn host_specs(
    paired: Vec<(String, String)>,
    name_of: impl Fn(&str) -> String,
) -> Vec<HostSpec> {
    let local = HostSpec {
        id: LOCAL_HOST.to_owned(),
        name: LOCAL_NAME.to_owned(),
        endpoint: None,
    };
    let paired = paired.into_iter().map(|(id, endpoint)| HostSpec {
        name: name_of(&id),
        id,
        endpoint: Some(endpoint),
    });
    std::iter::once(local).chain(paired).collect()
}

impl DaemonLink {
    pub(crate) fn hosts(&self) -> Vec<HostSpec> {
        let paired = self
            .registry
            .list()
            .into_iter()
            .map(|(id, endpoint)| (id.as_str().to_owned(), String::from(endpoint)))
            .collect();
        host_specs(paired, |id| {
            self.desk
                .notes
                .get(id)
                .map_or_else(|| id.to_owned(), |note| note.name)
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn this_computer_comes_first_then_each_paired_host_under_its_note_name() {
        let paired = vec![("h1".to_owned(), "wss://one".to_owned())];
        let specs = host_specs(paired, |id| format!("name of {id}"));
        let rows: Vec<_> = specs
            .iter()
            .map(|spec| {
                (
                    spec.id.as_str(),
                    spec.name.as_str(),
                    spec.endpoint.as_deref(),
                )
            })
            .collect();
        assert_eq!(
            rows,
            [
                ("local", "This computer", None),
                ("h1", "name of h1", Some("wss://one"))
            ]
        );
    }
}
