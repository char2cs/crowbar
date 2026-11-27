package kit

import "github.com/char2cs/crowbar/api/internal/core/terminal"

func ParseTerminalFrame(msg []byte) (payload []byte, snapshot, ok bool) {
	if len(msg) == 0 || msg[0] > terminal.FrameSnapshot {
		return nil, false, false
	}
	return msg[1:], msg[0] == terminal.FrameSnapshot, true
}
