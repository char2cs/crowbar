use crate::alert::AlertRequest;
use crate::error::Result;
use crate::view::ParentWindow;

pub type ChosenButton = Box<dyn FnOnce(Option<usize>) + Send + 'static>;

pub trait AlertAsking {
    fn ask_alert(
        &self,
        parent: ParentWindow,
        request: AlertRequest,
        answer: ChosenButton,
    ) -> Result<()>;
}
