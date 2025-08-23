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

func (c *Controller) handle(ev *tcell.EventKey) *tcell.EventKey {
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

	switch ev.Rune() {
	case 'q', 'Q':
		// shutdown in order
		if c.StopReader != nil {
			c.StopReader()
		}
		if c.StopProducer != nil {
			c.StopProducer()
		}
		c.App.Stop()
		return nil

	case 'r', 'R':
		// restart reader and clear view
		if c.StopReader != nil {
			c.StopReader()
		}
		c.App.QueueUpdateDraw(func() {
			c.MsgView.Clear()
			c.MsgView.ScrollToEnd()
		})
		if c.StartReader != nil {
			c.StartReader()
		}
		helper.UpdateStatus(c.Status, "[blue]Reader: RESTARTED[-]", false)
		return nil

	case 'c', 'C':
		c.App.QueueUpdateDraw(func() {
			c.MsgView.Clear()
			c.MsgView.ScrollToEnd()
		})
		helper.UpdateStatus(c.Status, "[blue]Messages: CLEARED[-]", false)
		return nil

	case 'p', 'P':
		// toggle producer
		if c.ProducerPaused != nil && *c.ProducerPaused {
			if c.StartProducer != nil {
				c.StartProducer()
			}
			*c.ProducerPaused = false
			helper.UpdateStatus(c.Status, "[magenta]Producer: STARTED[-]", false)
		} else {
			if c.StopProducer != nil {
				c.StopProducer()
			}
			if c.ProducerPaused != nil {
				*c.ProducerPaused = true
			}
			helper.UpdateStatus(c.Status, "[magenta]Producer: PAUSED[-]", false)
		}
		return nil

	case 'd', 'D':
		if c.DeleteTopic != nil {
			if err := c.DeleteTopic(); err != nil {
				helper.UpdateStatus(c.Status, fmt.Sprintf("[red]Delete failed: %v[-]", err), false)
			} else {
				helper.UpdateStatus(c.Status, "[red]Topic DELETED[-]", false)
			}
		}
		return nil
	}

	return ev
}
