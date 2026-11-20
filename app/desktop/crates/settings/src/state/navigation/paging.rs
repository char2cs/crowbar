//! The selected page, the search query and moving through the pages.

use crate::schema::Page;
use crate::schema::PageId;
use crate::schema::Schema;
use crate::state::State;
use crate::state::navigation::rail_move::RailMove;

impl State {
    /// The schema being shown.
    pub fn schema(&self) -> &Schema {
        &self.schema
    }

    /// The pages the rail lists: those whose condition holds and that the search leaves.
    pub fn visible_pages(&self) -> Vec<&Page> {
        let ids = self.visibility.pages();
        self.schema
            .pages()
            .iter()
            .filter(|page| ids.contains(&page.id))
            .collect()
    }

    /// The selected page, if any page is visible.
    pub fn page(&self) -> Option<&Page> {
        self.schema
            .page(self.navigation.active())
            .filter(|page| self.visibility.pages().contains(&page.id))
    }

    /// Selects `id` when it is visible.
    pub fn select_page(
        &mut self,
        id: &PageId,
    ) {
        let visible: Vec<&PageId> = self.visibility.pages().iter().collect();
        self.navigation.select(id, &visible);
    }

    /// Moves the rail selection with the keyboard.
    pub fn move_selection(
        &mut self,
        movement: RailMove,
    ) {
        let visible: Vec<&PageId> = self.visibility.pages().iter().collect();
        self.navigation.step(movement, &visible);
    }

    /// The search text.
    pub fn query(&self) -> &str {
        &self.query
    }

    /// Searches the pages' index and the rows' text; a non-blank query selects the first page it
    /// leaves, as develop's rail does.
    pub fn set_query(
        &mut self,
        query: &str,
    ) {
        query.clone_into(&mut self.query);
        self.refresh();
        if query.trim().is_empty() {
            return;
        }
        if let Some(first) = self.visibility.pages().first().cloned() {
            self.select_page(&first);
        }
    }
}

#[cfg(test)]
mod tests {
    use crate::schema::Page;
    use crate::schema::PageId;
    use crate::state::navigation::rail_move::RailMove;
    use crate::state::test_support::{engine_row, state};

    #[test]
    fn rail_selection_and_keyboard_movement() {
        let mut state = state();
        state.move_selection(RailMove::Next);
        assert_eq!(state.page().map(Page::label), Some("Browser"));
        state.move_selection(RailMove::Last);
        assert_eq!(state.page().map(Page::label), Some("Keybindings"));
        state.select_page(&PageId::from("appearance"));
        assert_eq!(state.page().map(Page::label), Some("Appearance"));
        assert_eq!(state.visible_pages().len(), 3);
    }

    #[test]
    fn search_filters_pages_and_moves_the_selection_off_a_hidden_page() {
        let mut state = state();
        state.set_query("search engine");
        let labels: Vec<&str> = state
            .visible_pages()
            .iter()
            .map(|page| page.label())
            .collect();
        assert_eq!(labels, ["Browser"]);
        assert_eq!(
            state.page().map(Page::label),
            Some("Browser"),
            "selection follows the only match"
        );
        assert!(state.row_visible(&engine_row()));
        state.set_query("zzz");
        assert_eq!(state.visible_pages().len(), 0);
        assert!(state.page().is_none(), "no match shows no page");
        state.set_query("");
        state.select_page(&PageId::from("browser"));
        state.set_query("font");
        assert_eq!(
            state.page().map(Page::label),
            Some("Appearance"),
            "a search lands on the first page it leaves, even when the selected one stays"
        );
        state.set_query("");
        assert_eq!(state.visible_pages().len(), 3);
        assert_eq!(state.query(), "");
    }
}
