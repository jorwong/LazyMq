package main

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	kafka2 "github.com/segmentio/kafka-go"

	"lazyMq/util/kafka"
)

func main() {
	app := tview.NewApplication()

	// ─── Channels & Kafka setup ──────────────────────────────────────────────
	var msgCh chan string
	errCh := make(chan error, 4)
	defer close(errCh)

	kp := kafka.NewKafkaCustom("test-topic", 0, "localhost:9092")

	conn, err := kp.Connect()
	if err != nil {
		panic(fmt.Errorf("kafka connect failed: %w", err))
	}
	defer conn.Close()

	// ─── Producer (can be toggled off if you only want to *read*) ───────────
	prodCtx, prodCancel := context.WithCancel(context.Background())
	go producer(prodCtx, conn, kp, errCh)

	// ─── UI widgets ─────────────────────────────────────────────────────────
	header := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText(fmt.Sprintf("[white:blue] Kafka Topic: [green]%s[white] ", kp.Topic)).
		SetBorder(true).
		SetTitle(" LazyMQ - Kafka Visualizer ")

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

	// ─── Reader lifecycle (start/stop with context) ─────────────────────────
	var readCtx context.Context
	var readCancel context.CancelFunc
	var msgPumpCtx context.Context
	var msgPumpCancel context.CancelFunc

	startReader := func() {
		// stop previous if any
		if readCancel != nil {
			readCancel()
		}
		if msgPumpCancel != nil {
			msgPumpCancel()
		}

		// Wait for cleanup
		time.Sleep(200 * time.Millisecond)

		// Create fresh channel for new reader session
		msgCh = make(chan string, 1024)
		readCtx, readCancel = context.WithCancel(context.Background())
		msgPumpCtx, msgPumpCancel = context.WithCancel(context.Background())

		var msgCount int32
		reader := kp.NewTailReader([]string{"localhost:9092"}, kp.Topic, 0)

		go func() {
			for {
				select {
				case <-msgPumpCtx.Done():
					return
				case m, ok := <-msgCh:
					if !ok {
						return
					}
					currentCount := atomic.AddInt32(&msgCount, 1)
					text := fmt.Sprintf("[%d] %s", currentCount, m)
					app.QueueUpdateDraw(func() {
						fmt.Fprintf(msgView, "%s\n", text)
						msgView.ScrollToEnd()
					})
				}
			}
		}()

		// Start reader
		go func() {
			defer close(msgCh)
			defer reader.Close()
			for {
				m, err := reader.ReadMessage(readCtx)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					errCh <- fmt.Errorf("read failed: %w", err)
					return
				}
				select {
				case <-readCtx.Done():
					return
				case msgCh <- string(m.Value):
				}
			}
		}()

		updateStatus(status, "[green]Reader: RUNNING[-]", true)
	}

	stopReader := func() {
		if readCancel != nil {
			readCancel()
			updateStatus(status, "[yellow]Reader: STOPPING...[-]", true)
		}
		if msgPumpCancel != nil {
			msgPumpCancel()
		}
	}

	startReader()

	// ─── Error/status pump (optional) ───────────────────────────────────────
	go func() {
		for e := range errCh {
			if e == nil {
				continue
			}
			app.QueueUpdateDraw(func() {
				fmt.Fprintf(msgView, "[red]ERROR:[-] %v\n", e)
				msgView.ScrollToEnd()
			})
		}
	}()

	// ─── Key-bindings (global) ──────────────────────────────────────────────
	producerPaused := true

	app.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Key() {
		case tcell.KeyUp:
			or, _ := msgView.GetScrollOffset()
			if or > 0 {
				msgView.ScrollTo(or-1, 0)
			}
			return nil
		case tcell.KeyDown:
			or, _ := msgView.GetScrollOffset()
			msgView.ScrollTo(or+1, 0)
			return nil
		case tcell.KeyHome:
			msgView.ScrollToBeginning()
			return nil
		case tcell.KeyEnd:
			msgView.ScrollToEnd()
			return nil
		}

		switch ev.Rune() {
		case 'q', 'Q':
			// shutdown in order
			stopReader()
			prodCancel()
			app.Stop()
			return nil

		case 'r', 'R':
			// restart reader and clear view
			stopReader()
			msgView.Clear()
			startReader()
			updateStatus(status, "[blue]Reader: RESTARTED[-]", false)
			return nil

		case 'c', 'C':
			msgView.Clear()
			updateStatus(status, "[blue]Messages: CLEARED[-]", false)
			return nil

		case 'p', 'P':
			// toggle producer
			if producerPaused {
				prodCtx, prodCancel = context.WithCancel(context.Background())
				go producer(prodCtx, conn, kp, errCh)
				producerPaused = false
				updateStatus(status, "[magenta]Producer: STARTED[-]", false)
			} else {
				prodCancel()
				producerPaused = true
				updateStatus(status, "[magenta]Producer: PAUSED[-]", false)
			}
			return nil

		case 'd', 'D':
			// Dangerous: delete topic (kept since you had it)
			if err := kp.DeleteTopic(conn, kp.Topic); err != nil {
				updateStatus(status, fmt.Sprintf("[red]Delete failed: %v[-]", err), false)
			} else {
				updateStatus(status, "[red]Topic DELETED[-]", false)
			}
			return nil
		}
		return ev
	})

	// Set focus to the message view
	app.SetFocus(msgView)

	if err := app.SetRoot(layout, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}

	// final cleanup (in case)
	if readCancel != nil {
		readCancel()
	}
	if msgPumpCancel != nil {
		msgPumpCancel()
	}
	prodCancel()
}

// producer: sends 3 lines per tick
func producer(ctx context.Context, conn *kafka2.Conn, k *kafka.Kafka, errCh chan<- error) {
	ticker := time.NewTicker(3 * time.Second) // Slowed down to 3 seconds for better readability
	defer ticker.Stop()

	counter := 0
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			messages := []string{
				fmt.Sprintf("Message %d at %s", counter, t.Format("15:04:05")),
				fmt.Sprintf("Data payload %d", counter),
				fmt.Sprintf("Event %d processed", counter),
			}
			if err := k.WriteMessages(conn, messages); err != nil {
				errCh <- fmt.Errorf("producer write failed: %w", err)
				// continue; if the broker is briefly unavailable, keep trying
			}
			counter++
		}
	}
}

// helper: update status line with temporary messages and restore commands
func updateStatus(tv *tview.TextView, text string, permanent bool) {
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
