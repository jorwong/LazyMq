package helper

import (
	"github.com/rivo/tview"
	"time"
)

// helper: update status line with temporary messages and restore commands
func UpdateStatus(tv *tview.TextView, text string, permanent bool) {
	tv.SetText(text)

	if !permanent {
		// Show temporary status for 3 seconds, then restore command help
		go func() {
			time.Sleep(3 * time.Second)
			commandsText := "[yellow]Commands:[-] [blue]↑/↓[-] scroll | [blue]r[-] reload | [blue]c[-] clear | [blue]p[-] toggle producer | [blue]d[-] delete topic | [red]q[-] quit"
			tv.SetText(commandsText)
		}()
	}
}
