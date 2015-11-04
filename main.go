package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/AislerHQ/trayhost"
	"github.com/toqueteos/webbrowser"
	"log"
	"math"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
	"errors" 
)

const (
	ICON_IDLE          = 1
	ICON_NOT_CONNECTED = 2
	ICON_DL            = 3
	ICON_UL            = 4
	ICON_ERROR         = 5
	ICON_UL_DL         = 6
)


var since_events = 0
var startTime = "-"

// config for connection to syncthing
type Config struct {
	Url      string
	username string
	password string
	insecure bool
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
	id               string
	completion       float64
	state            string
	needFiles        int
	sharedWith       []string
}

var folder map[string]*Folder



func readEvents() error {
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

	res, err := query_syncthing(fmt.Sprintf("%s/rest/events?since=%d", config.Url, since_events))

	if err != nil { //usually connection error -> continue
		//log.Println(err)
		return err
	} else {
		var events []event
		err = json.Unmarshal([]byte(res), &events)
		if err != nil {
			//log.Println(err)
			return err
		}

		for _, event := range events {
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
				return errors.New("got new config") 

			}
			since_events = event.ID
		}

	}

	return nil
}

func main_loop() {
	for {
		err := readEvents()
		if err != nil {
			defer initialize()
			log.Println("error while reading events:",err)
			return
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

	log.Printf("connected %v", numConnected)

	trayhost.UpdateCh <- trayhost.MenuItemUpdate{2, trayhost.MenuItem{
		fmt.Sprintf("Connected to %d Devices", numConnected),
		true,
		nil,
	},
	}

	if numConnected == 0 {
		//not connected
		log.Println("not connected")
		err := trayhost.SetIcon(ICON_NOT_CONNECTED)
		log.Println(err)
	} else if downloading && uploading {
		//ul+dl
		log.Println("ul+dl")
		err := trayhost.SetIcon(ICON_UL_DL)
		log.Println(err)
	} else if downloading && !uploading {
		//dl
		log.Println("dl")
		err := trayhost.SetIcon(ICON_DL)
		log.Println(err)
	} else if !downloading && uploading {
		//ul
		log.Println("ul")
		err := trayhost.SetIcon(ICON_UL)
		log.Println(err)
	} else if !downloading && !uploading {
		//idle
		log.Println("idle")
		err := trayhost.SetIcon(ICON_IDLE)
		log.Println(err)
	}

}

func main() {
	url := flag.String("target", "http://localhost:8384", "Target Syncthing instance")
	user := flag.String("u", "", "User")
	pw := flag.String("p", "", "Password")
	iconDir := flag.String("icondir", os.TempDir(), "Directory to store temporary icons")
	insecure := flag.Bool("i", false, "skip verification of SSL certificate")
	flag.Parse()

	config.Url = *url
	config.username = *user
	config.password = *pw
	config.insecure = *insecure

	// EnterLoop must be called on the OS's main thread
	runtime.LockOSThread()

	menuItems := trayhost.MenuItems{
		0: trayhost.MenuItem{
			"Syncthing-Tray",
			true,
			nil,
		},
		1: trayhost.MenuItem{
			"",
			true,
			nil,
		},
		2: trayhost.MenuItem{
			"waiting for response of syncthing",
			true,
			nil,
		},
		3: trayhost.MenuItem{
			"Open Syncthing GUI",
			false,
			func() {
				log.Println("Opening Browser")
				webbrowser.Open(config.Url)
			},
		},
		4: trayhost.MenuItem{
			fmt.Sprintf(""),
			true,
			nil,
		},
		5: trayhost.MenuItem{
			"Exit",
			false,
			trayhost.Exit,
		}}

	trayhost.Initialize("Syncthing-Tray", icon_error, menuItems, *iconDir)
	trayhost.SetClickHandler(onClick)
	//trayhost.SetIconImage(ICON_ALTERNATIVE, icon_not_connected)
	//trayhost.SetIconImage(ICON_ATTENTION, icon_error)
	trayhost.SetIconImage(ICON_IDLE, icon_idle)
	trayhost.SetIconImage(ICON_NOT_CONNECTED, icon_not_connected)
	trayhost.SetIconImage(ICON_DL, icon_dl)
	trayhost.SetIconImage(ICON_UL, icon_ul)
	trayhost.SetIconImage(ICON_ERROR, icon_error)
	trayhost.SetIconImage(ICON_UL_DL, icon_ul_dl)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	go func() {
		<-c
		trayhost.Exit()
	}()

	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	log.Println("Starting Syncthing-Tray")
	log.Println("Connecting to syncthing at", config.Url)

	go func() {
		initialize()
	}()

	// Enter the host system's event loop
	trayhost.EnterLoop()

	// This is only reached once the user chooses the Exit menu item
	log.Println("Exiting syncthing-tray")
}

func onClick() { // not usable on ubuntu, left click also displays the menu
	fmt.Println("Opening webinterface in browser")
	webbrowser.Open(config.Url)
}
