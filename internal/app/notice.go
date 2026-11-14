package app

// note: warnings belong on the status line, not stderr, because the alternate
// screen is ours and direct writes scribble over the layout

import (
	"fmt"
	"time"

	"github.com/Irsyad-gh/muxplayer/internal/diag"
)

const noticeFor = 5 * time.Second

// noticef shows a short message in the footer and keeps it in the debug log.
func (m *model) noticef(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	m.notice, m.noticeAt = msg, time.Now()
	diag.Debugf("ui: %s", msg)
}

// currentNotice returns the footer message while it is still fresh.
func (m *model) currentNotice() string {
	if m.notice == "" || time.Since(m.noticeAt) > noticeFor {
		return ""
	}
	return m.notice
}
