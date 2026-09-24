package kit

import "github.com/char2cs/crowbar/api/internal/core/terminal"

// ParseTerminalFrame is the client side of the terminal wire protocol: it
// splits one binary output message into its payload and whether it is a
// snapshot. ok is false for anything that is not an output frame (the JSON
// exit frame, for one). The daemon only ever writes these frames, so the
// decoder lives with the test clients that read them.
func ParseTerminalFrame(msg []byte) (payload []byte, snapshot bool, ok bool) {
	if len(msg) == 0 || msg[0] > terminal.FrameSnapshot {
		return nil, false, false
	}
	return msg[1:], msg[0] == terminal.FrameSnapshot, true
}
