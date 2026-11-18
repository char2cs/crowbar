//! The open find bar and its subscription.

use gpui::{Entity, Subscription};

use crate::search_bar::SearchBar;

pub(crate) struct Session {
    pub(crate) bar: Entity<SearchBar>,
    pub(crate) _events: Subscription,
}
