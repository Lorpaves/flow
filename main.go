package main

import (
	"flow/internal/core"
	"log"
	"os"
	"strings"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"
)

func main() {
	mygo.App.SetName("Flow")
	mygo.App.SetLanguages("zh-Hans", "en")
	mygo.Theme.SetSource(mygo.ThemeDark)
	missing := ""
	if dir := os.Getenv("FLOW_DATA_DIR"); dir != "" {
		mygo.App.SetPath(mygo.PathUserData, dir)
	} else if def, err := core.DefaultDataDir(); err == nil {
		// The workspace may have been moved in Settings; the default folder
		// says where.
		dir, gone, err := core.ResolveDataDir(def)
		if err != nil {
			log.Print(err)
		}
		if dir != def {
			mygo.App.SetPath(mygo.PathUserData, dir)
		}
		missing = gone
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	d := &desktop{missingData: missing}
	d.createWindow = d.createNativeWindow
	var pendingLinks []string
	open := func(raw string) {
		if d.active == nil {
			pendingLinks = append(pendingLinks, raw)
			return
		}
		if err := d.active.openLink(raw); err != nil {
			d.active.err = err.Error()
		}
		d.show(d.active)
	}
	mygo.App.OnActivate(func(visible bool) {
		if !visible {
			d.show(d.active)
		}
	})
	mygo.App.OnOpenURL(open)
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		for _, arg := range args {
			if strings.HasPrefix(arg, "flow://") {
				open(arg)
			}
		}
		d.show(d.active)
	})
	mygo.App.OnNotificationClick(func(id string) {
		if a := d.active; a != nil {
			if reminder, ok := a.data.Reminders[strings.TrimPrefix(id, "flow-reminder:")]; ok {
				a.openReminder(reminder)
				d.show(a)
			}
		}
	})
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if !d.requestQuit() {
			e.PreventDefault()
		}
	})
	mygo.App.WhenReady(func() {
		dir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			log.Print(err)
			mygo.App.Quit()
			return
		}
		d.dataDir = dir
		d.createNativeWindow("main", false, nil)
		for _, raw := range pendingLinks {
			open(raw)
		}
		pendingLinks = nil
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
