package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/alex2108/systray"
	"github.com/toqueteos/webbrowser"
)

var mutex = &sync.Mutex{}
var eventMutex = &sync.Mutex{}
var dataMutex = &sync.Mutex{}
var trayMutex = &sync.Mutex{}
var since_events = 0
var startTime = "-"
var eventChan = make(chan event, 10000)

var inBytesRate float64
var outBytesRate float64

type folderSummary struct {
	NeedFiles   int    `json:"needFiles"`
	State       string `json:"state"`
	GlobalFiles int    `json:"globalFiles"`
}

type eventData struct {
	Folder     string        `json:"folder"`
	Summary    folderSummary `json:"summary"`
	Completion float64       `json:"completion"`
	Device     string        `json:"device"`
	Id         string        `json:"id"`
}
type event struct {
	ID   int       `json:"id"`
	Type string    `json:"type"`
	Time time.Time `json:"time"`
	Data eventData `json:"data"`
}

// config for connection to syncthing
type Config struct {
	Url      string
	ApiKey   string
	insecure bool
	useRates bool
}

var config Config

// configured devices
type Device struct {
	name             string
	folderCompletion map[string]float64
	connected        bool
}

var device map[string]*Device

// configured folders
type Folder struct {
	id         string
	completion float64
	state      string
	needFiles  int
	sharedWith []string
}

var folder map[string]*Folder

func readEvents() error {
	res, err := query_syncthing(fmt.Sprintf("%s/rest/events?since=%d", config.Url, since_events))

	if err != nil {
		return err
	} else {
		var events []event
		err = json.Unmarshal([]byte(res), &events)
		if err != nil {
			return err
		}

		for _, event := range events {
			eventChan <- event
			since_events = event.ID
		}

	}
	return nil
}

func eventProcessor() {
	for event := range eventChan {
		mutex.Lock() // mutex with initialitze which may still be running
		// handle different events
		if event.Type == "FolderSummary" {
			folder[event.Data.Folder].needFiles = event.Data.Summary.NeedFiles
			folder[event.Data.Folder].state = event.Data.Summary.State
			folder[event.Data.Folder].completion = 100 - 100*float64(event.Data.Summary.NeedFiles)/math.Max(float64(event.Data.Summary.GlobalFiles), 1)

			updateStatus()

		} else if event.Type == "FolderCompletion" {
			device[event.Data.Device].folderCompletion[event.Data.Folder] = event.Data.Completion

			updateStatus()

		} else if event.Type == "DeviceConnected" {
			log.Println(event.Data.Id, "connected")
			device[event.Data.Id].connected = true
			updateStatus()

		} else if event.Type == "DeviceDisconnected" {
			log.Println(event.Data.Id, "disconnected")
			device[event.Data.Id].connected = false
			updateStatus()
		} else if event.Type == "ConfigSaved" {
			log.Println("got new config -> reinitialize")
			since_events = event.ID
			mutex.Unlock()
			initialize()
			continue
		}
		mutex.Unlock()
	}
}

func main_loop() {
	for {
		eventMutex.Lock()
		err := readEvents()
		eventMutex.Unlock()
		time.Sleep(time.Millisecond) // otherwise initialize does not have a chance to get the lock since it is aquired here instantly again
		if err != nil {
			initialize()
		}
	}

}

func updateStatus() {
	log.Println("updating status")

	downloading := false
	uploading := false
	numConnected := 0

	for _, fol_info := range folder {
		//log.Printf("folder %v",fol)
		//log.Printf("folder_info %v",fol_info)
		if fol_info.completion < 100 {
			downloading = true
		}
	}

	for _, dev_info := range device {
		//%log.Printf("device %v",dev)
		//log.Printf("device_info %v",dev_info)

		if dev_info.connected {
			numConnected++

			for _, completion := range dev_info.folderCompletion {
				if completion < 100 {
					uploading = true
				}
			}
		}

	}

	if config.useRates {
		dataMutex.Lock()
		if inBytesRate > 500 {
			downloading = true
		} else {
			downloading = false
		}
		if outBytesRate > 500 {
			uploading = true
		} else {
			uploading = false
		}
		dataMutex.Unlock()
	}

	log.Printf("connected %v", numConnected)

	trayMutex.Lock()
	trayEntries.connectedDevices.SetTitle(fmt.Sprintf("Connected to %d Devices", numConnected))
	setIcon(numConnected, downloading, uploading)
	trayMutex.Unlock()

}

func setIcon(numConnected int, downloading, uploading bool) {
	if numConnected == 0 {
		//not connected
		log.Println("not connected")
		systray.SetIcon(icon_not_connected)

	} else if downloading && uploading {
		//ul+dl
		log.Println("ul+dl")
		systray.SetIcon(icon_ul_dl)
	} else if downloading && !uploading {
		//dl
		log.Println("dl")
		systray.SetIcon(icon_dl)
	} else if !downloading && uploading {
		//ul
		log.Println("ul")
		systray.SetIcon(icon_ul)
	} else if !downloading && !uploading {
		//idle
		log.Println("idle")
		systray.SetIcon(icon_idle)
	}

}

func main() {
	// must be done at the beginning
	systray.Run(setupTray)
}

type TrayEntries struct {
	stVersion        *systray.MenuItem
	connectedDevices *systray.MenuItem
	rateDisplay      *systray.MenuItem
	openBrowser      *systray.MenuItem
	quit             *systray.MenuItem
}

var trayEntries TrayEntries

func setupTray() {
	url := flag.String("target", "http://localhost:8384", "Target Syncthing instance")
	api := flag.String("api", "", "Syncthing Api Key (used for password protected syncthing instance)")
	insecure := flag.Bool("i", false, "skip verification of SSL certificate")
	useRates := flag.Bool("R", false, "use transfer rates to determine upload/download state")
	flag.Parse()

	config.Url = *url
	config.ApiKey = *api
	config.insecure = *insecure
	config.useRates = *useRates

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	go func() {
		<-c
		systray.Quit()
		os.Exit(0)
	}()

	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	log.Println("Starting Syncthing-Tray")
	log.Println("Connecting to syncthing at", config.Url)
	trayMutex.Lock()
	go rate_reader()
	go eventProcessor()
	go func() {
		initialize()
		main_loop()

	}()
	systray.SetIcon(icon_error)
	systray.SetTitle("")
	systray.SetTooltip("Syncthing-Tray")

	trayEntries.stVersion = systray.AddMenuItem("not connected", "Syncthing")
	trayEntries.stVersion.Disable()

	trayEntries.connectedDevices = systray.AddMenuItem("not connected", "Connected devices")
	trayEntries.connectedDevices.Disable()
	trayEntries.rateDisplay = systray.AddMenuItem("↓: 0 B/s ↑: 0 B/s", "Upload and download rate")
	trayEntries.rateDisplay.Disable()
	trayEntries.openBrowser = systray.AddMenuItem("Open Syncthing GUI", "opens syncthing GUI in default browser")

	trayEntries.quit = systray.AddMenuItem("Quit", "Quit Syncthing-Tray")
	go func() {
		for {
			select {
			case <-trayEntries.quit.ClickedCh:
				systray.Quit()
				fmt.Println("Quit now...")
				os.Exit(0)
			case <-trayEntries.openBrowser.ClickedCh:
				webbrowser.Open(config.Url)
			}
		}

	}()
	trayMutex.Unlock()
}

func onClick() { // not usable on ubuntu, left click also displays the menu
	fmt.Println("Opening webinterface in browser")
	webbrowser.Open(config.Url)
}
