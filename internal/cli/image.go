package cli

import (
	"fmt"
	"image"
	_ "image/gif"  // register decoders for image.Decode
	_ "image/jpeg" // (stdlib only)
	_ "image/png"
	"io"
	"os"

	"github.com/mholovetskyi/cliche/internal/cli/rawmode"
	"github.com/mholovetskyi/cliche/internal/style"
)

// cmdImage renders an image file as colored half-block text right in the
// terminal — `cliche image cat.png`. No graphics protocol, no dependencies:
// truecolor "▀" cells on any color terminal (Windows Terminal included).
func cmdImage(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: cliche image <file.png|.jpg|.gif>")
		return 2
	}
	return renderImageFile(args[0], out, errOut)
}

func renderImageFile(path string, out, errOut io.Writer) int {
	if !style.Enabled {
		fmt.Fprintln(errOut, "image: terminal color is off (use a color terminal, or set CLICHE_FORCE_COLOR=1)")
		return 1
	}
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(errOut, "image: "+err.Error())
		return 1
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		fmt.Fprintln(errOut, "image: "+err.Error()+" (supported: png, jpeg, gif)")
		return 1
	}
	s := style.RenderImage(img, imageCols())
	if s == "" {
		fmt.Fprintln(errOut, "image: nothing to render")
		return 1
	}
	fmt.Fprintln(out, s)
	return 0
}

// imageCols renders at the terminal width (less a small gutter), capped so a wide
// window doesn't paint an enormous picture.
func imageCols() int {
	cols, _ := rawmode.Size(os.Stdout)
	if cols < 8 {
		cols = 64
	}
	if cols -= 2; cols > 110 {
		cols = 110
	}
	return cols
}
