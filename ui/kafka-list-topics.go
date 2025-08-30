package ui

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"lazyMq.com/util/helper"
	"lazyMq.com/util/kafka"
	"sort"
	"strconv"
)

type KafkaListTopicsPage struct {
	Pages *tview.Pages
	App   *tview.Application

	// UI
	Header     *tview.TextView
	Status     *tview.TextView
	TopicsView *tview.Table
	Layout     *tview.Flex

	// Kafka deps
	svc *kafka.Kafka

	onOpenTopic func(string)
}

func NewKafkaListTopics(app *tview.Application, svc *kafka.Kafka, pages *tview.Pages, onOpenTopic func(string)) *KafkaListTopicsPage {
	header := tview.NewTextView()
	header.SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText(fmt.Sprintf("View Kafka Topics - Kafka Address: [green]%s[white] ", svc.Address)).
		SetBorder(true).
		SetTitle(" LazyMQ - Kafka Visualizer")

	topicView := tview.NewTable()
	topicView.
		SetTitle(" Topics ").SetBorder(true)
	topicView.
		SetSelectable(true, false) // select by row

	topicView.SetFixed(1, 0) // freeze header row

	// Create a proper status/menu bar
	statusText := "[yellow]Commands:[-] [blue]↑/↓[-] scroll | [blue]c[-] create topic | [blue]d[-] delete topic | [red]q[-] quit"
	status := tview.NewTextView()
	status.SetDynamicColors(true).
		SetText(statusText).
		SetTextAlign(tview.AlignLeft).
		SetBorder(true).
		SetTitle(" Controls ")

	// ─── Layout with proper sizing ──────────────────────────────────────────
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 3, 0, false).   // Fixed height of 3 lines for header
		AddItem(topicView, 0, 1, true). // Flexible height for messages (main area)
		AddItem(status, 3, 0, false)    // Fixed height of 3 lines for status

	return &KafkaListTopicsPage{
		App:         app,
		Pages:       pages,
		Header:      header,
		Status:      status,
		TopicsView:  topicView,
		Layout:      layout,
		svc:         svc,
		onOpenTopic: onOpenTopic,
	}
}

func (p *KafkaListTopicsPage) renderTopicsTable(names []string) {
	p.App.QueueUpdateDraw(func() {
		tbl := p.TopicsView
		tbl.Clear()

		bold := tcell.AttrBold
		tbl.SetCell(0, 0, tview.NewTableCell("#").SetAttributes(bold))
		tbl.SetCell(0, 1, tview.NewTableCell("TOPIC").SetAttributes(bold))

		for i, name := range names {
			r := i + 1
			tbl.SetCell(r, 0, tview.NewTableCell(strconv.Itoa(r)).SetAlign(tview.AlignRight))
			tbl.SetCell(r, 1, tview.NewTableCell(name).SetExpansion(1))
		}

		// Ensure there is a selection and focus on the table
		if tbl.GetRowCount() > 1 {
			tbl.Select(1, 0) // first data row (row 0 is header)
		}
		p.App.SetFocus(tbl)
	})
}

func (p *KafkaListTopicsPage) PageName() string { return "List Topics" }

func (p *KafkaListTopicsPage) Mount(pages *tview.Pages, visible bool) {
	pages.AddPage(p.PageName(), p.Layout, true, visible)
}

func (p *KafkaListTopicsPage) Start() {
	p.ListTopics()
	p.AttachKeys(p.App)
}

func (p *KafkaListTopicsPage) ListTopics() {
	go func() {
		topics, err := p.svc.ListTopics()
		if err != nil {
			p.App.QueueUpdateDraw(func() {
				fmt.Fprintf(p.Status, "[red]ListTopics error: %v[-]\n", err)
			})
			return
		}

		// Filter out the internal offsets topic (safer than topics[1:])
		filtered := make([]string, 0, len(topics))
		for _, t := range topics {
			if t == "__consumer_offsets" {
				continue
			}
			filtered = append(filtered, t)
		}
		sort.Strings(filtered) // optional

		p.renderTopicsTable(filtered)
	}()
}

func (p *KafkaListTopicsPage) AttachKeys(app *tview.Application) {
	p.TopicsView.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Rune() {
		case 'q', 'Q':
			app.Stop()
			return nil

		case 'c', 'C':
			ShowCreateTopicModal(p.App, p.Pages, p.Status, p.svc.CreateTopic, p.ListTopics, p.TopicsView)
			return nil

		case 'd', 'D':
			r, _ := p.TopicsView.GetSelection()
			if r <= 0 || r >= p.TopicsView.GetRowCount() {
				return nil
			}
			topicName := p.TopicsView.GetCell(r, 1).Text
			if topicName == "" {
				return nil
			}

			go func(name string) {
				if err := p.svc.DeleteTopic(name); err != nil {
					p.App.QueueUpdateDraw(func() {
						helper.UpdateStatus(p.Status, "[red]Delete Topic error: "+err.Error()+"\n", false)
					})
					return
				}
				p.App.QueueUpdateDraw(func() {
					helper.UpdateStatus(p.Status, "[red]Topic Deleted: "+name+"[-]\n", false)
				})
				p.ListTopics()
			}(topicName)
			return nil

		}

		switch ev.Key() {
		case tcell.KeyUp:
			r, c := p.TopicsView.GetSelection()
			if r <= 1 {
				r = 1
			} else {
				r--
			}
			p.TopicsView.Select(r, c)
			return nil

		case tcell.KeyDown:
			r, c := p.TopicsView.GetSelection()
			max := p.TopicsView.GetRowCount() - 1
			if r < 1 {
				r = 1
			} else if r < max {
				r++
			}
			p.TopicsView.Select(r, c)
			return nil

		case tcell.KeyHome:
			if p.TopicsView.GetRowCount() > 1 {
				p.TopicsView.Select(1, 0)
			}
			return nil

		case tcell.KeyEnd:
			max := p.TopicsView.GetRowCount() - 1
			if max >= 1 {
				p.TopicsView.Select(max, 0)
			}
			return nil

		case tcell.KeyEnter:
			r, _ := p.TopicsView.GetSelection()
			if r <= 0 || r >= p.TopicsView.GetRowCount() {
				return nil
			}
			topicName := p.TopicsView.GetCell(r, 1).Text
			if topicName == "" {
				return nil
			}
			if p.onOpenTopic != nil {
				p.onOpenTopic(topicName)
			}
			return nil
		}
		return ev
	})
}
