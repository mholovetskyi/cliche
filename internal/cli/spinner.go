package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/mholovetskyi/cliche/internal/style"
)

// spinnerFrames is a braille rotation — smooth and compact.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinner animates a "thinking…" indicator on a single line while the model is
// working, so a network wait isn't dead silence. Each frame is tinted a step
// further along the brand gradient, so it shimmers. It is a no-op when styling
// is disabled (non-TTY / NO_COLOR), keeping piped output and tests clean.
type spinner struct {
	out   io.Writer
	label string
	stop  chan struct{}
	done  chan struct{}
}

func newSpinner(out io.Writer, label string) *spinner {
	return &spinner{out: out, label: label}
}

// Start begins animating in a background goroutine.
func (s *spinner) Start() {
	if !style.Enabled {
		return
	}
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		fmt.Fprint(s.out, "\x1b[?25l") // hide cursor
		start := time.Now()
		tk := time.NewTicker(90 * time.Millisecond)
		defer tk.Stop()
		for i := 0; ; i++ {
			select {
			case <-s.stop:
				return
			case <-tk.C:
				frame := style.Color(spinnerFrames[i%len(spinnerFrames)], style.Sample(float64(i%len(spinnerFrames))/float64(len(spinnerFrames)-1)))
				fmt.Fprintf(s.out, "\r  %s %s %s ", frame, style.Gray(s.label), style.Dim(fmt.Sprintf("%.0fs", time.Since(start).Seconds())))
			}
		}
	}()
}

// Stop halts the animation, clears the line, and restores the cursor. It waits
// for the goroutine to exit so no further frame races with subsequent output.
func (s *spinner) Stop() {
	if s.stop == nil {
		return
	}
	close(s.stop)
	<-s.done
	fmt.Fprint(s.out, "\r\x1b[2K\x1b[?25h") // clear the line + show cursor
	s.stop = nil
}
