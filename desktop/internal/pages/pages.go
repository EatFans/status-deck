// Package pages defines the device-owned page catalog shared by the desktop
// control surface and its selective data synchronizer.
package pages

const (
	Unknown = 0
	System  = 1
	Codex   = 2
)

// Status is the page state reported by the device in a page.status notify.
// New pages can be added by assigning their next stable index here and in the
// firmware catalog; the protocol remains unchanged.
type Status struct {
	Index int
	ID    string
	Count int
}

func (s Status) Known() bool {
	return s.Index == System || s.Index == Codex
}

func (s Status) Label() string {
	switch s.Index {
	case System:
		return "System"
	case Codex:
		return "Codex"
	default:
		return "未知页面"
	}
}
