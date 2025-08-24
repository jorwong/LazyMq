// lazyMq.com/ui/topic_page.go
package ui

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	kafka2 "github.com/segmentio/kafka-go"

	"lazyMq.com/util/helper"
	"lazyMq.com/util/kafka"
)

type KafkaTopicViewPage struct {
	App         *tview.Application
	Topic       string
	Address     string
	Header      *tview.TextView
	Status      *tview.TextView
	MessageView *tview.TextView
	Layout      *tview.Flex

	// Kafka deps
	Client *kafka2.Client // optional if you still need it
	Svc    *kafka.Kafka   // <— our utility svc

	// runtime
	reader        *kafka2.Reader
	writer        *kafka2.Writer
	errCh         chan error
	msgCh         chan string
	readCtx       context.Context
	readCancel    context.CancelFunc
	msgPumpCtx    context.Context
	msgPumpCancel context.CancelFunc

	// Producer
	prodCtx     context.Context
	prodCancel  context.CancelFunc
	ProdEnabled bool
}

func NewKafkaTopicViewPage(app *tview.Application, client *kafka2.Client, svc *kafka.Kafka, topic, address string) *KafkaTopicViewPage {
	header := tview.NewTextView()
	header.
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText(fmt.Sprintf("Kafka Topic: [green]%s[white] ", topic)).
		SetBorder(true).
		SetTitle(" LazyMQ - Kafka Visualizer")

	msgView := tview.NewTextView()
	msgView.
		SetScrollable(true).
		SetDynamicColors(true).
		SetBorder(true).
		SetTitle(" Messages ")

	status := tview.NewTextView()
	status.
		SetDynamicColors(true).
		SetText("[yellow]Commands:[-] [blue]↑/↓[-] scroll | [blue]r[-] reload | [blue]c[-] clear | [blue]p[-] prod on/off | [blue]t[-] list topics | [blue]d[-] delete topic | [red]q[-] quit").
		SetTextAlign(tview.AlignLeft).
		SetBorder(true).
		SetTitle(" Controls ")

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 3, 0, false).
		AddItem(msgView, 0, 1, true).
		AddItem(status, 3, 0, false)

	return &KafkaTopicViewPage{
		App:         app,
		Topic:       topic,
		Address:     address,
		Header:      header,
		Status:      status,
		MessageView: msgView,
		Layout:      layout,
		Client:      client,
		Svc:         svc,
		errCh:       make(chan error, 16),
	}
}

func (p *KafkaTopicViewPage) PageName() string { return "topic:" + p.Topic }

func (p *KafkaTopicViewPage) Mount(pages *tview.Pages, visible bool) {
	pages.AddPage(p.PageName(), p.Layout, true, visible)
}

func (p *KafkaTopicViewPage) StartReader() {
	p.StopReader()
	time.Sleep(120 * time.Millisecond) // small gap to unwind

	p.msgCh = make(chan string, 1024)
	p.readCtx, p.readCancel = context.WithCancel(context.Background())
	p.msgPumpCtx, p.msgPumpCancel = context.WithCancel(context.Background())

	var count int32
	p.reader = p.Svc.TailReader([]string{p.Address}, p.Topic, 0)

	// UI pump
	go func() {
		for {
			select {
			case <-p.msgPumpCtx.Done():
				return
			case m, ok := <-p.msgCh:
				if !ok {
					return
				}
				n := atomic.AddInt32(&count, 1)
				txt := fmt.Sprintf("[%d] %s", n, m)
				p.App.QueueUpdateDraw(func() {
					fmt.Fprintf(p.MessageView, "%s\n", txt)
					p.MessageView.ScrollToEnd()
				})
			}
		}
	}()

	// Reader loop
	go func() {
		defer close(p.msgCh)
		defer p.reader.Close()
		for {
			msg, err := p.reader.ReadMessage(p.readCtx)
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
		p.readCancel = nil
		helper.UpdateStatus(p.Status, "[yellow]Reader: STOPPING...[-]", false)
	}
	if p.msgPumpCancel != nil {
		p.msgPumpCancel()
		p.msgPumpCancel = nil
	}
}

func (p *KafkaTopicViewPage) StartProducer() {
	if p.prodCancel != nil {
		return // already running
	}
	if p.writer == nil {
		p.writer = p.Svc.NewWriter(p.Topic)
	}
	p.prodCtx, p.prodCancel = context.WithCancel(context.Background())

	// demo producer: 1 msg / 500ms; replace with your own trigger
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		i := 0
		for {
			select {
			case <-p.prodCtx.Done():
				return
			case <-t.C:
				i++
				if err := p.Svc.Produce(p.prodCtx, p.writer, fmt.Sprintf("demo #%d @ %s", i, time.Now().Format(time.RFC3339))); err != nil {
					p.errCh <- err
					return
				}
			}
		}
	}()

	helper.UpdateStatus(p.Status, "[green]Producer: RUNNING[-]", false)
}

func (p *KafkaTopicViewPage) StopProducer() {
	if p.prodCancel != nil {
		p.prodCancel()
		p.prodCancel = nil
	}
	helper.UpdateStatus(p.Status, "[yellow]Producer: STOPPED[-]", false)
}

func (p *KafkaTopicViewPage) DeleteTopic() error {
	return p.Svc.DeleteTopic(p.Topic)
}

func (p *KafkaTopicViewPage) PumpErrors() {
	go func() {
		for e := range p.errCh {
			if e == nil {
				continue
			}
			p.App.QueueUpdateDraw(func() {
				fmt.Fprintf(p.MessageView, "[red]ERROR:[-] %v\n", e)
				p.MessageView.ScrollToEnd()
			})
		}
	}()
}

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
			p.MessageView.Clear()
			helper.UpdateStatus(p.Status, "[blue]Reader: RESTARTING...[-]", false)
			go p.StartReader()
			return nil

		case 'c', 'C':
			p.MessageView.Clear()
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

		case 't', 'T':
			// List topics and print to message pane
			go func() {
				topics, err := p.Svc.ListTopics()
				if err != nil {
					p.errCh <- err
					return
				}
				p.App.QueueUpdateDraw(func() {
					fmt.Fprintf(p.MessageView, "[yellow]Topics:[-] %v\n", topics)
					p.MessageView.ScrollToEnd()
				})
			}()
			return nil
		}
		return ev
	})
}

func (p *KafkaTopicViewPage) Start(testFlag bool) {
	p.PumpErrors()
	p.StartReader()
	if testFlag {
		p.StartProducer()
	}

	p.AttachKeys(p.App)
	p.App.SetFocus(p.MessageView)

}

func (p *KafkaTopicViewPage) Close() {
	p.StopReader()
	p.StopProducer()
	if p.writer != nil {
		_ = p.writer.Close()
		p.writer = nil
	}
	close(p.errCh)
}
