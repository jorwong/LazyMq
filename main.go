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

	pages := tview.NewPages()

	viewTopicPage := ui.NewKafkaTopicViewPage(app, client, kafkaConn, topic, addr, pages)
	viewTopicPage.ProdEnabled = testFlag
	viewTopicPage.Mount(pages, false) // initially hidden

	openTopic := func(topic string) {
		viewTopicPage.SetTopic(topic) // your method to change topic
		viewTopicPage.Restart()       // or Start/reload if that's your API
		viewTopicPage.Start(testFlag)
		pages.SwitchToPage(viewTopicPage.PageName())
	}

	listTopicsPage := ui.NewKafkaListTopics(app, kafkaConn, pages, openTopic)
	listTopicsPage.Mount(pages, true) // visible first
	listTopicsPage.Start()

	if err := app.SetRoot(pages, true).EnableMouse(true).Run(); err != nil {
		log.Fatal(err)
	}
}
