package tmux

// TmuxOps abstracts tmux window/pane operations for testing.
type TmuxOps interface {
	NewWindow(session, name, dir string, command []string) (string, error)
	SplitWindow(paneID, dir string, horizontal bool, sizePercent int, command []string) (string, error)
	KillWindow(target string) error
	KillPane(paneID string) error
	SendKeys(paneID string, keys ...string) error
	SelectWindow(target string) error
	SelectPane(paneID string) error
	PaneExistsInWindow(paneID, windowID string) bool
	WindowIDForPane(paneID string) (string, error)
}

// PaneStatusChecker abstracts pane monitoring for testing.
type PaneStatusChecker interface {
	GetPaneStatus(paneID string) (PaneStatus, error)
	Remove(paneID string)
}

// RealTmux delegates to the package-level functions.
type RealTmux struct{}

func (RealTmux) NewWindow(session, name, dir string, command []string) (string, error) {
	return NewWindow(session, name, dir, command)
}

func (RealTmux) SplitWindow(paneID, dir string, horizontal bool, sizePercent int, command []string) (string, error) {
	return SplitWindow(paneID, dir, horizontal, sizePercent, command)
}

func (RealTmux) KillWindow(target string) error {
	return KillWindow(target)
}

func (RealTmux) KillPane(paneID string) error {
	return KillPane(paneID)
}

func (RealTmux) SendKeys(paneID string, keys ...string) error {
	return SendKeys(paneID, keys...)
}

func (RealTmux) SelectWindow(target string) error {
	return SelectWindow(target)
}

func (RealTmux) SelectPane(paneID string) error {
	return SelectPane(paneID)
}

func (RealTmux) PaneExistsInWindow(paneID, windowID string) bool {
	return PaneExistsInWindow(paneID, windowID)
}

func (RealTmux) WindowIDForPane(paneID string) (string, error) {
	return WindowIDForPane(paneID)
}
