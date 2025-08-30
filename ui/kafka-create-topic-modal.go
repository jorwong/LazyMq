package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"lazyMq.com/util/helper"
	"regexp"
	"strings"
)

var topicNameRE = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,249}$`)

// ShowCreateTopicModal pops a centered input modal to create a Kafka topic.
// - createTopic: function that creates the topic (e.g., p.svc.CreateTopic)
// - afterSuccess: callback after success (e.g., p.ListTopics)
// - returnFocus: primitive to refocus when modal closes (e.g., p.TopicsView)
func ShowCreateTopicModal(
	app *tview.Application,
	pages *tview.Pages,
	status *tview.TextView,
	createTopic func(string) error,
	afterSuccess func(),
	returnFocus tview.Primitive,
) {
	const modalName = "createTopicModal"

	input := tview.NewInputField().
		SetLabel("Topic name: ").
		SetFieldWidth(40)

	form := tview.NewForm().
		AddFormItem(input).
		AddButton("Create", nil). // callback wired below
		AddButton("Cancel", func() {
			pages.RemovePage(modalName)
			if returnFocus != nil {
				app.SetFocus(returnFocus)
			}
		})
	form.
		SetBorder(true).
		SetTitle(" Create Topic ").
		SetTitleAlign(tview.AlignLeft)

	create := func() {
		name := strings.TrimSpace(input.GetText())
		switch {
		case name == "":
			helper.UpdateStatus(status, "[red]Please enter a topic name.[-]\n", false)
			return
		case name == "__consumer_offsets":
			helper.UpdateStatus(status, "[red]This name is reserved.[-]\n", false)
			return
		case !topicNameRE.MatchString(name):
			helper.UpdateStatus(status, "[red]Invalid name. Use letters, digits, . _ - (max 249).[-]\n", false)
			return
		}

		go func(topic string) {
			if err := createTopic(topic); err != nil {
				app.QueueUpdateDraw(func() {
					helper.UpdateStatus(status, "[red]Create Topic error: "+err.Error()+"\n", false)
				})
				return
			}
			app.QueueUpdateDraw(func() {
				pages.RemovePage(modalName)
				helper.UpdateStatus(status, "[green]Topic Created: "+topic+"[-]\n", false)
				if returnFocus != nil {
					app.SetFocus(returnFocus)
				}
			})
			if afterSuccess != nil {
				afterSuccess() // e.g., refresh list
			}
		}(name)
	}

	// Wire the button and Enter key
	form.GetButton(form.GetButtonCount() - 2).SetSelectedFunc(create) // "Create" button
	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			create()
		}
	})
	form.SetCancelFunc(func() {
		pages.RemovePage(modalName)
		if returnFocus != nil {
			app.SetFocus(returnFocus)
		}
	})

	// Centered modal layout
	modal := tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(
			tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(tview.NewBox(), 0, 1, false).
				AddItem(form, 9, 0, true).
				AddItem(tview.NewBox(), 0, 1, false),
			60, 0, true,
		).
		AddItem(tview.NewBox(), 0, 1, false)

	pages.AddPage(modalName, modal, true, true)
	app.SetFocus(input)
}
