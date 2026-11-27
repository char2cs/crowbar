use crate::error::Result;
use crate::menu::MenuRequest;
use crate::view::ParentWindow;

pub type ChosenEntry = Box<dyn FnOnce(Option<Vec<usize>>) + Send + 'static>;

pub trait MenuPopping {
    fn popup_menu(
        &self,
        parent: ParentWindow,
        request: MenuRequest,
        answer: ChosenEntry,
    ) -> Result<()>;
}
