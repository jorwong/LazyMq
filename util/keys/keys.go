// ui/keys/keys.go
package keys

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"lazyMq.com/util/helper"
)

type Controller struct {
	App     *tview.Application
	MsgView *tview.TextView
	Status  *tview.TextView

	// lifecycle hooks provided by main
	StartReader func()
	StopReader  func()

	// producer control (toggle)
	ProducerPaused *bool
	StartProducer  func() // starts producer goroutine
	StopProducer   func() // cancels producer ctx

	// dangerous ops
	DeleteTopic func() error
}

func NewController(
	app *tview.Application,
	msgView, status *tview.TextView,
	startReader, stopReader func(),
	producerPaused *bool,
	startProducer, stopProducer func(),
	deleteTopic func() error,
) *Controller {
	return &Controller{
		App:            app,
		MsgView:        msgView,
		Status:         status,
		StartReader:    startReader,
		StopReader:     stopReader,
		ProducerPaused: producerPaused,
		StartProducer:  startProducer,
		StopProducer:   stopProducer,
		DeleteTopic:    deleteTopic,
	}
}

func (c *Controller) Attach() {
	c.App.SetInputCapture(c.handle)
}

// in ui/keys/keys.go
func (c *Controller) handle(ev *tcell.EventKey) *tcell.EventKey {
	// 1) Printable keys first (portable)
	if r := ev.Rune(); r != 0 {
		switch r {
		case 'q', 'Q':
			// quick & safe on UI thread
			if c.StopReader != nil {
				c.StopReader()
			}
			if c.StopProducer != nil {
				c.StopProducer()
			}
			c.App.Stop()
			return nil

		case 'r', 'R':
			// 1) Stop reader (fast)
			if c.StopReader != nil {
				c.StopReader()
			}

			// 2) Update UI directly (we're on the app goroutine inside InputCapture)
			c.MsgView.Clear()
			c.MsgView.ScrollToEnd()
			helper.UpdateStatus(c.Status, "[blue]Reader: RESTARTING...[-]", false) // must NOT queue/draw

			// 3) Restart reader in background; post back with QueueUpdate (no Draw)
			go func() {
				if c.StartReader != nil {
					c.StartReader() // contains small time.Sleep, ok
				}
				c.App.QueueUpdate(func() {
					helper.UpdateStatus(c.Status, "[green]Reader: RUNNING[-]", false)
				})
				// (optional) if you really want an immediate repaint, do ONE of these from outside any queued fn:
				// c.App.Draw()  // ONLY if you're certain you're on the app goroutine. From goroutines, don't call Draw.
			}()
			return nil

		case 'c', 'C':
			c.MsgView.Clear()
			c.MsgView.ScrollToEnd()
			helper.UpdateStatus(c.Status, "[blue]Messages: CLEARED[-]", false)
			return nil

		case 'p', 'P':
			// start/stop producer can touch nets/ctx -> do in bg
			if c.ProducerPaused != nil && *c.ProducerPaused {
				go func() {
					if c.StartProducer != nil {
						c.StartProducer()
					}
					*c.ProducerPaused = false

					c.App.QueueUpdate(func() {
						helper.UpdateStatus(c.Status, "[magenta]Producer: STARTED[-]", false)
					})
				}()
			} else {
				go func() {
					if c.StopProducer != nil {
						c.StopProducer()
					}
					if c.ProducerPaused != nil {
						*c.ProducerPaused = true
					}
					c.App.QueueUpdate(func() {
						helper.UpdateStatus(c.Status, "[magenta]Producer: PAUSED[-]", false)
					})
				}()
			}
			return nil

		case 'd', 'D':
			// network/delete may block -> background
			go func() {
				if c.DeleteTopic != nil {
					if err := c.DeleteTopic(); err != nil {
						c.App.QueueUpdate(func() {
							helper.UpdateStatus(c.Status, fmt.Sprintf("[red]Delete failed: %v[-]", err), false)
						})
					} else {
						c.App.QueueUpdate(func() {
							helper.UpdateStatus(c.Status, "[red]Topic DELETED[-]", false)
						})
					}
				}
			}()
			return nil
		}
		return ev
	}

	// 2) Navigation keys (cheap; OK on UI thread)
	switch ev.Key() {
	case tcell.KeyUp:
		row, _ := c.MsgView.GetScrollOffset()
		if row > 0 {
			c.MsgView.ScrollTo(row-1, 0)
		}
		return nil
	case tcell.KeyDown:
		row, _ := c.MsgView.GetScrollOffset()
		c.MsgView.ScrollTo(row+1, 0)
		return nil
	case tcell.KeyHome:
		c.MsgView.ScrollToBeginning()
		return nil
	case tcell.KeyEnd:
		c.MsgView.ScrollToEnd()
		return nil
	}
	return ev
}
