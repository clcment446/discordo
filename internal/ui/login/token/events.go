package token

import (
	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
)

type TokenEvent struct {
	tcell.EventTime
	Token string
}

func tokenCommand(token string) tview.Cmd {
	return func() tview.Event {
		return &TokenEvent{Token: token}
	}
}
