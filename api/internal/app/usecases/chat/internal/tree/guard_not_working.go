package tree

import (
	"errors"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
)

var ErrSubtreeWorking = errors.New("tree: a working chat refuses this verb")

func guardNotWorking(
	subtreeIDs []string,
	work *inflight.Work,
) error {
	for _, id := range subtreeIDs {
		if working, _, _ := work.Observe(id); working {
			return ErrSubtreeWorking
		}
	}
	return nil
}
