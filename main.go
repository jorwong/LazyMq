package main

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/rivo/tview"

	"lazyMq.com/ui"
)

func main() {
	_ = godotenv.Load()

	topic := os.Getenv("TOPIC")
	addr := os.Getenv("ADDRESS")
	testFlag, _ := strconv.ParseBool(os.Getenv("TEST_FLAG"))

	app := tview.NewApplication()

	// Build page
	page := ui.NewKafkaTopicViewPage(app, topic, addr)
	page.ProdEnabled = testFlag

	// Wire into pages
	pages := tview.NewPages()
	page.Mount(pages, true)

	// Connect + start
	page.PumpErrors()
	page.StartReader()
	if testFlag {
		page.StartProducer()
	}

	// Keys
	page.AttachKeys(app)
	app.SetFocus(page.MessageView)

	// Run
	if err := app.SetRoot(pages, true).EnableMouse(true).Run(); err != nil {
		log.Fatal(err)
	}

	// Cleanup
	page.Close()
}
