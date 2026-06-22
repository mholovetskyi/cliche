package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/mholovetskyi/cliche/internal/style"
)

// spinnerFrames is a braille rotation — smooth and compact.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinnerDebounce delays the first frame so a fast operation (an instant read or
// a quick model reply) finishes without ever flashing a spinner, while a genuine
// wait still shows one promptly.
const spinnerDebounce = 120 * time.Millisecond

// spinner animates a contextual indicator on one line while the agent is busy —
// during a model wait OR a tool's execution — so neither is dead silence. The
// label narrates the current phase ("running go test…", "thinking…"). Each frame
// is tinted a step along the brand gradient, so it shimmers. It is a no-op when
// styling is disabled (piped / NO_COLOR), keeping captured output clean.
type spinner struct {
	out   io.Writer
	label string
	stop  chan struct{}
	done  chan struct{}
}

func newSpinner(out io.Writer, label string) *spinner {
	return &spinner{out: out, label: label}
}

// Start begins animating in a background goroutine (after a short debounce).
func (s *spinner) Start() {
	if !style.Enabled {
		return
	}
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		start := time.Now()
		select { // debounce: a sub-threshold op never paints a frame at all
		case <-s.stop:
			return
		case <-time.After(spinnerDebounce):
		}
		style.HideCursor(s.out)
		defer style.ShowCursor(s.out)
		tk := time.NewTicker(90 * time.Millisecond)
		defer tk.Stop()
		paint := func(i int) {
			frame := style.Color(spinnerFrames[i%len(spinnerFrames)], style.Sample(float64(i%len(spinnerFrames))/float64(len(spinnerFrames)-1)))
			// \x1b[K clears any residue (e.g. when "10s" replaces a wider label).
			fmt.Fprintf(s.out, "\r  %s %s %s\x1b[K", frame, style.Gray(s.label), style.Dim(fmt.Sprintf("%.0fs", time.Since(start).Seconds())))
		}
		paint(0) // paint immediately once past the debounce — no extra blank tick
		for i := 1; ; i++ {
			select {
			case <-s.stop:
				return
			case <-tk.C:
				paint(i)
			}
		}
	}()
}

// Stop halts the animation and clears the line. It waits for the goroutine to
// exit (which restores the cursor) so no further frame races later output.
func (s *spinner) Stop() {
	if s.stop == nil {
		return
	}
	close(s.stop)
	<-s.done
	fmt.Fprint(s.out, "\r\x1b[2K") // clear the spinner line (cursor restored by the goroutine)
	s.stop = nil
}
