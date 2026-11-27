use std::fmt::Display;

use journal::{Event, Level};

pub(crate) fn assert_events<M: Display>(
    cases: impl IntoIterator<Item = (impl Event, &'static str, Level, M)>
) {
    for (event, name, level, shown) in cases {
        assert_eq!(event.name(), name);
        assert_eq!(event.level(), level, "{name}");
        assert_eq!(event.to_string(), shown.to_string(), "{name}");
    }
}
