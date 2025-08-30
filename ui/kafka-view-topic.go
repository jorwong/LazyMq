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
	Page        *tview.Pages
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

func NewKafkaTopicViewPage(app *tview.Application, client *kafka2.Client, svc *kafka.Kafka, topic, address string, pages *tview.Pages) *KafkaTopicViewPage {
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
		Page:        pages,
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

func (p *KafkaTopicViewPage) PageName() string { return "topic" } // fixed

func (p *KafkaTopicViewPage) Mount(pages *tview.Pages, visible bool) {
	pages.AddPage(p.PageName(), p.Layout, true, visible)
}

func (p *KafkaTopicViewPage) StartReader() {
	p.StopReader()
	time.Sleep(120 * time.Millisecond) // let old goroutines unwind

	// New, per-run state kept local to avoid field races
	msgCh := make(chan string, 1024)
	readCtx, readCancel := context.WithCancel(context.Background())
	pumpCtx, pumpCancel := context.WithCancel(context.Background())
	reader := p.Svc.HeadReader([]string{p.Address}, p.Topic, 0)

	// Publish handles for StopReader()
	p.msgCh = msgCh
	p.readCtx, p.readCancel = readCtx, readCancel
	p.msgPumpCtx, p.msgPumpCancel = pumpCtx, pumpCancel
	p.reader = reader

	var count int32

	// UI pump (reads from the *local* msgCh)
	go func(ch <-chan string) {
		for {
			select {
			case <-pumpCtx.Done():
				return
			case m, ok := <-ch:
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
	}(msgCh)

	// Reader loop (writes to the *local* msgCh)
	go func(ch chan<- string, r *kafka2.Reader, ctx context.Context) {
		defer close(ch)
		defer r.Close()

		for {
			msg, err := r.ReadMessage(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				// Don’t panic if errCh is closed during shutdown.
				select {
				case p.errCh <- fmt.Errorf("read failed: %w", err):
				default:
				}
				return
			}
			select {
			case <-ctx.Done():
				return
			case ch <- string(msg.Value):
			}
		}
	}(msgCh, reader, readCtx)

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
		case tcell.KeyEsc:
			p.Page.SwitchToPage("List Topics")
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

func (p *KafkaTopicViewPage) Restart() {
	p.MessageView.Clear()
	helper.UpdateStatus(p.Status, "[blue]Reader: RESTARTING...[-]", false)
	go p.StartReader()
}

func (p *KafkaTopicViewPage) SetTopic(topic string) {
	p.Topic = topic
}
