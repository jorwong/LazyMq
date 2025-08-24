package ui

import (
	"context"
	"errors"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	kafka2 "github.com/segmentio/kafka-go"
	"lazyMq.com/util/helper"
	"lazyMq.com/util/kafka"
	"sync/atomic"
	"time"
)

type KafkaTopicViewPage struct {
	App   *tview.Application
	Topic string

	// UI
	Header      *tview.TextView
	Status      *tview.TextView
	MessageView *tview.TextView
	Layout      *tview.Flex

	// Kafka deps
	KP      *kafka.Kafka
	Address string

	// Cons & gorountines
	conn          *kafka2.Conn
	errCh         chan error
	msgCh         chan string
	readCtx       context.Context
	readCancel    context.CancelFunc
	msgPumpCtx    context.Context
	msgPumpCancel context.CancelFunc

	// Producer
	prodCtx     context.Context
	prodCancel  context.CancelFunc
	ProdEnabled bool // flag to auto-start producer in test mode, etc.
}

func NewKafkaTopicViewPage(app *tview.Application, topic string, address string) *KafkaTopicViewPage {
	header := tview.NewTextView()
	header.SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText(fmt.Sprintf("Kafka Topic: [green]%s[white] ", topic)).
		SetBorder(true).
		SetTitle(" LazyMQ - Kafka Visualizer")

	msgView := tview.NewTextView()

	msgView.SetScrollable(true).
		SetDynamicColors(true).
		SetBorder(true).
		SetTitle(" Messages ")

	// Create a proper status/menu bar
	statusText := "[yellow]Commands:[-] [blue]↑/↓[-] scroll | [blue]r[-] reload | [blue]c[-] clear | [blue]p[-] toggle producer | [blue]d[-] delete topic | [red]q[-] quit"
	status := tview.NewTextView()
	status.SetDynamicColors(true).
		SetText(statusText).
		SetTextAlign(tview.AlignLeft).
		SetBorder(true).
		SetTitle(" Controls ")

	// ─── Layout with proper sizing ──────────────────────────────────────────
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 3, 0, false). // Fixed height of 3 lines for header
		AddItem(msgView, 0, 1, true). // Flexible height for messages (main area)
		AddItem(status, 3, 0, false)  // Fixed height of 3 lines for status

	kp := kafka.NewKafkaCustom(topic, 0, address)

	return &KafkaTopicViewPage{
		App:         app,
		Topic:       topic,
		Header:      header,
		Status:      status,
		MessageView: msgView,
		Layout:      layout,
		KP:          kp,
		Address:     address,
		errCh:       make(chan error, 8),
	}
}

func (p *KafkaTopicViewPage) PageName() string {
	return "topic:" + p.Topic
}

func (p *KafkaTopicViewPage) Mount(pages *tview.Pages, visible bool) {
	pages.AddPage(p.PageName(), p.Layout, true, visible)
}

// StartProducer starts a demo/test producer if enabled.
func (p *KafkaTopicViewPage) StartProducer() {
	if !p.ProdEnabled || p.conn == nil {
		return
	}
	if p.prodCancel != nil {
		return // already running
	}
	p.prodCtx, p.prodCancel = context.WithCancel(context.Background())
	go kafka.Producer(p.prodCtx, p.conn, p.KP, p.errCh)
	helper.UpdateStatus(p.Status, "[green]Producer: RUNNING[-]", false)
}

func (p *KafkaTopicViewPage) StopProducer() {
	if p.prodCancel != nil {
		p.prodCancel()
		p.prodCancel = nil
		helper.UpdateStatus(p.Status, "[yellow]Producer: STOPPED[-]", false)
	}
}

// StartReader wires the tail reader + message pump.
func (p *KafkaTopicViewPage) StartReader() {
	// stop previous if any
	p.StopReader()

	time.Sleep(200 * time.Millisecond) // give goroutines time to unwind

	p.msgCh = make(chan string, 1024)
	p.readCtx, p.readCancel = context.WithCancel(context.Background())
	p.msgPumpCtx, p.msgPumpCancel = context.WithCancel(context.Background())

	var msgCount int32
	reader := p.KP.TailReader([]string{p.Address}, p.KP.Topic, 0)

	// Pump to TextView (from background -> QueueUpdate)
	go func() {
		for {
			select {
			case <-p.msgPumpCtx.Done():
				return
			case m, ok := <-p.msgCh:
				if !ok {
					return
				}
				n := atomic.AddInt32(&msgCount, 1)
				text := fmt.Sprintf("[%d] %s", n, m)
				p.App.QueueUpdateDraw(func() {
					fmt.Fprintf(p.MessageView, "%s\n", text)
					p.MessageView.ScrollToEnd()
				})
			}
		}
	}()

	// Reader loop
	go func() {
		defer close(p.msgCh)
		defer reader.Close()
		for {
			msg, err := reader.ReadMessage(p.readCtx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				p.errCh <- fmt.Errorf("read failed: %w", err)
				return
			}
			select {
			case <-p.readCtx.Done():
				return
			case p.msgCh <- string(msg.Value):
			}
		}
	}()

	helper.UpdateStatus(p.Status, "[green]Reader: RUNNING[-]", false)
}

func (p *KafkaTopicViewPage) StopReader() {
	if p.readCancel != nil {
		p.readCancel()
		helper.UpdateStatus(p.Status, "[yellow]Reader: STOPPING...[-]", false)
	}
	if p.msgPumpCancel != nil {
		p.msgPumpCancel()
	}
}

// DeleteTopic via conn.
func (p *KafkaTopicViewPage) DeleteTopic() error {
	if p.conn == nil {
		return fmt.Errorf("not connected")
	}
	return p.KP.DeleteTopic(p.conn, p.KP.Topic)
}

// PumpErrors prints errors to the message view.
func (p *KafkaTopicViewPage) PumpErrors() {
	go func() {
		for e := range p.errCh {
			if e == nil {
				continue
			}
			p.App.QueueUpdate(func() {
				fmt.Fprintf(p.MessageView, "[red]ERROR:[-] %v\n", e)
				p.MessageView.ScrollToEnd()
			})
		}
	}()
}

// AttachKeys binds keys to this page safely (no nested QueueUpdateDraw).
func (p *KafkaTopicViewPage) AttachKeys(app *tview.Application) {
	app.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Key() {
		case tcell.KeyUp:
			or, _ := p.MessageView.GetScrollOffset()
			if or > 0 {
				p.MessageView.ScrollTo(or-1, 0)
			}
			return nil
		case tcell.KeyDown:
			or, _ := p.MessageView.GetScrollOffset()
			p.MessageView.ScrollTo(or+1, 0)
			return nil
		case tcell.KeyHome:
			p.MessageView.ScrollToBeginning()
			return nil
		case tcell.KeyEnd:
			p.MessageView.ScrollToEnd()
			return nil
		}

		switch ev.Rune() {
		case 'q', 'Q':
			p.StopReader()
			p.StopProducer()
			app.Stop()
			return nil

		case 'r', 'R':
			// direct updates (we’re on UI goroutine)
			p.MessageView.Clear()
			p.MessageView.ScrollToEnd()
			helper.UpdateStatus(p.Status, "[blue]Reader: RESTARTING...[-]", false)

			go func() {
				p.StartReader()
				p.App.QueueUpdate(func() {
					helper.UpdateStatus(p.Status, "[green]Reader: RUNNING[-]", false)
				})
			}()
			return nil

		case 'c', 'C':
			p.MessageView.Clear()
			p.MessageView.ScrollToEnd()
			helper.UpdateStatus(p.Status, "[blue]Messages: CLEARED[-]", false)
			return nil

		case 'p', 'P':
			if p.prodCancel == nil {
				p.StartProducer()
			} else {
				p.StopProducer()
			}
			return nil

		case 'd', 'D':
			if err := p.DeleteTopic(); err != nil {
				p.errCh <- err
			} else {
				helper.UpdateStatus(p.Status, "[red]Topic deleted[-]", false)
			}
			return nil
		}
		return ev
	})
}

// Close cleans up resources.
func (p *KafkaTopicViewPage) Close() {
	p.StopReader()
	p.StopProducer()
	if p.conn != nil {
		p.conn.Close()
		p.conn = nil
	}
	close(p.errCh)
}
