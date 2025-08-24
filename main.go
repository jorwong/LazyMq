package main

import (
	"lazyMq.com/util/kafka"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/rivo/tview"
	kafka2 "github.com/segmentio/kafka-go"

	"lazyMq.com/ui"
)

func main() {
	_ = godotenv.Load()

	topic := os.Getenv("TOPIC")
	addr := os.Getenv("ADDRESS")
	testFlag, _ := strconv.ParseBool(os.Getenv("TEST_FLAG"))

	app := tview.NewApplication()
	client := &kafka2.Client{Addr: kafka2.TCP(addr)} // optional
	kafkaConn := kafka.NewKafkaCustom(0, addr)

	viewTopicsPage := ui.NewKafkaListTopics(app, kafkaConn)

	page := ui.NewKafkaTopicViewPage(app, client, kafkaConn, topic, addr)
	page.ProdEnabled = testFlag

	pages := tview.NewPages()
	page.Mount(pages, false)
	page.Start(testFlag)

	viewTopicsPage.Mount(pages, true)
	viewTopicsPage.Start()
	defer page.Close()

	if err := app.SetRoot(pages, true).EnableMouse(true).Run(); err != nil {
		log.Fatal(err)
	}
}
