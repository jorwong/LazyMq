package ui

import (
	"fmt"
	"github.com/rivo/tview"
	"lazyMq.com/util/kafka"
)

type KafkaListTopicsPage struct {
	App *tview.Application

	// UI
	Header      *tview.TextView
	Status      *tview.TextView
	MessageView *tview.TextView
	Layout      *tview.Flex

	// Kafka deps
	svc *kafka.Kafka
}

func NewKafkaListTopics(app *tview.Application, svc *kafka.Kafka) *KafkaListTopicsPage {
	header := tview.NewTextView()
	header.SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText(fmt.Sprintf("View Kafka Topics - Kafka Address: [green]%s[white] ", svc.Address)).
		SetBorder(true).
		SetTitle(" LazyMQ - Kafka Visualizer")

	msgView := tview.NewTextView()

	msgView.SetScrollable(true).
		SetDynamicColors(true).
		SetBorder(true).
		SetTitle(" Topics ")

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

	return &KafkaListTopicsPage{
		App:         app,
		Header:      header,
		Status:      status,
		MessageView: msgView,
		Layout:      layout,
		svc:         svc,
	}
}

func (p *KafkaListTopicsPage) ListTopics() {
	topics, err := p.svc.ListTopics()
	if err != nil {
		return
	}
	for _, topic := range topics {
		p.App.QueueUpdateDraw(func() {
			fmt.Fprintf(p.MessageView, "[yellow]Topics:[-] %v\n", topic)
			p.MessageView.ScrollToEnd()
		})
	}
}

func (p *KafkaListTopicsPage) PageName() string { return "List Topics Page" }

func (p *KafkaListTopicsPage) Mount(pages *tview.Pages, visible bool) {
	pages.AddPage(p.PageName(), p.Layout, true, visible)
}

func (p *KafkaListTopicsPage) Start() {
	p.ListTopics()
}

func (p *KafkaListTopicsPage) AttachKeys(app *tview.Application) {}
