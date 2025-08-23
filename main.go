package main

import (
	"context"
	"errors"
	"fmt"
	"lazyMq.com/util/helper"
	"lazyMq.com/util/keys"
	"log"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/joho/godotenv"
	"github.com/rivo/tview"
	"lazyMq.com/util/kafka"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	topic := os.Getenv("TOPIC")
	address := os.Getenv("ADDRESS")
	testFlag, err := strconv.ParseBool(os.Getenv("TEST_FLAG"))       // true : testing
	producerPaused, err := strconv.ParseBool(os.Getenv("TEST_FLAG")) // true : testing
	if err != nil {
		panic(fmt.Errorf("error Parsing Env Variables: %w", err))
	}

	app := tview.NewApplication()

	// ─── Channels & Kafka setup ──────────────────────────────────────────────
	var msgCh chan string
	errCh := make(chan error, 4)
	defer close(errCh)

	kp := kafka.NewKafkaCustom(topic, 0, address)

	conn, err := kp.Connect()
	if err != nil {
		panic(fmt.Errorf("kafka connect failed: %w", err))
	}
	defer conn.Close()

	// ─── Producer (can be toggled off if you only want to *read*) ───────────
	prodCtx, prodCancel := context.WithCancel(context.Background())

	if testFlag {
		go kafka.Producer(prodCtx, conn, kp, errCh)
	}

	// ─── UI widgets ─────────────────────────────────────────────────────────
	header := tview.NewTextView()
	header.SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText(fmt.Sprintf("Kafka Topic: [green]%s[white] ", kp.Topic)).
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
		reader := kp.TailReader([]string{address}, kp.Topic, 0)

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

		helper.UpdateStatus(status, "[green]Reader: RUNNING[-]", true)
	}

	stopReader := func() {
		if readCancel != nil {
			readCancel()
			helper.UpdateStatus(status, "[yellow]Reader: STOPPING...[-]", true)
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
	prodCtx, prodCancel = context.WithCancel(context.Background())
	startProd := func() {
		if testFlag {
			go kafka.Producer(prodCtx, conn, kp, errCh)
		}
	}
	stopProd := func() {
		if prodCancel != nil {
			prodCancel()
		}
	}
	deleteTopic := func() error {
		return kp.DeleteTopic(conn, kp.Topic)
	}

	ctrl := keys.NewController(
		app,
		msgView,
		status,
		startReader,
		stopReader,
		&producerPaused,
		startProd,
		stopProd,
		deleteTopic,
	)
	ctrl.Attach() // Set focus to the message view
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
	stopProd()
}
