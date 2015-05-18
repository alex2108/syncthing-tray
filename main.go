package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/alex2108/trayhost"
	"github.com/toqueteos/webbrowser"
	"io/ioutil"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

var since_events = 0
var client = &http.Client{}

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
	id         string
	completion float64
	state      string
	needFiles  int
	sharedWith []string
}

var folder map[string]*Folder

func get_folder_state() error {
	for key, rep := range folder {
		r_json, err := query_syncthing(config.Url + "/rest/db/status?folder=" + rep.id)

		if err == nil {
			type Folderstate struct {
				NeedFiles   int
				GlobalFiles int
				State       string
			}

			var m Folderstate
			json_err := json.Unmarshal([]byte(r_json), &m)

			if json_err != nil {
				return json_err
			} else {

				folder[key].state = m.State
				folder[key].needFiles = m.NeedFiles
				folder[key].completion = 100 - 100*float64(m.NeedFiles)/math.Max(float64(m.GlobalFiles), 1) // max to prevent division by zero

			}
		} else {
			return err
		}

	}

	return nil
}
func get_connections() error {
	input, _ := query_syncthing(config.Url + "/rest/system/connections")

	var res map[string]interface{}
	err := json.Unmarshal([]byte(input), &res)

	for deviceId, _ := range device {
		device[deviceId].connected = false
	}

	for deviceId, _ := range res["connections"].(map[string]interface{}) {
		device[deviceId].connected = true
	}

	return err
}
func update_ul() error {

	type Completion struct {
		Completion float64
	}
	for r, r_info := range folder {
		for _, n := range r_info.sharedWith {
			if device[n].connected { // only query connected devices
				out, err := query_syncthing(config.Url + "/rest/db/completion?device=" + n + "&folder=" + r)
				if err != nil {
					log.Println(err)
					return err
				}
				var m Completion
				err = json.Unmarshal([]byte(out), &m)
				if err != nil {
					log.Println(err)
					return err
				}
				device[n].folderCompletion[r] = m.Completion
			}
		}
	}
	return nil
}
func set_latest_id() error {

	type event struct {
		ID int `json:"id"`
	}

	res, err := query_syncthing(fmt.Sprintf("%s/rest/events?since=%d", config.Url, since_events))

	var events []event
	err = json.Unmarshal([]byte(res), &events)

	if err != nil {
		log.Fatal(err)
		return err
	}

	for _, event := range events {
		since_events = event.ID
	}

	return nil
}

func initialize() {
	since_events = 0 // reset event counter because syncthing could be restarted
	err := get_config()
	if err != nil {
		log.Println(err)
		log.Println("error getting syncthing config -> retry in 5s")

		trayhost.UpdateCh <- trayhost.MenuItemUpdate{0, trayhost.MenuItem{
			fmt.Sprintf("Syncthing: no connection to " + config.Url),
			true,
			nil,
		},
		}
		trayhost.SetIcon(trayhost.ICON_ERROR)
		time.Sleep(5 * time.Second)
		initialize()
	}
	// get all previous events without processing them
	set_latest_id()
	// get current state
	get_folder_state()
	get_connections()
	update_ul()
	

}
func get_config() error {
	log.Println("reading config from syncthing")
	//create empty state
	device = make(map[string]*Device)
	folder = make(map[string]*Folder)

	r_json, err := query_syncthing(config.Url + "/rest/system/config")

	if err == nil {
		type SyncthingConfigDevice struct {
			Deviceid string
			Name     string
		}
		type SyncthingConfigFolderDevice struct {
			Deviceid string
		}

		type SyncthingConfigFolder struct {
			Id      string
			Devices []SyncthingConfigFolderDevice
		}
		type SyncthingConfig struct {
			Devices []SyncthingConfigDevice
			Folders []SyncthingConfigFolder
		}

		var m SyncthingConfig
		json_err := json.Unmarshal([]byte(r_json), &m)

		if json_err != nil {
			return json_err
		} else { // save config in structs

			//save Devices
			for _, v := range m.Devices {
				device[v.Deviceid] = &Device{v.Name, make(map[string]float64), false}
			}
			//save Folders
			for _, v := range m.Folders {
				folder[v.Id] = &Folder{v.Id, 0, "invalid", 0, make([]string, 0)}
				for _, v2 := range v.Devices {
					folder[v.Id].sharedWith = append(folder[v.Id].sharedWith, v2.Deviceid)
					device[v2.Deviceid].folderCompletion[v.Id] = -1
				}
			}

		}
	} else {
		return err
	}

	//Display version
	log.Println("getting version")
	resp, err := query_syncthing(config.Url + "/rest/system/version")
	if err == nil {
		type STVersion struct {
			Version string
		}

		var m STVersion
		err = json.Unmarshal([]byte(resp), &m)
		if err == nil {
			log.Println("displaying version")
			trayhost.UpdateCh <- trayhost.MenuItemUpdate{0, trayhost.MenuItem{
				fmt.Sprintf("Syncthing: %s", m.Version),
				true,
				nil,
			},
			}
		}
	}
	return err
}

func query_syncthing(url string) (string, error) {
	req, _ := http.NewRequest("GET", url, nil)
	//req.Header.Set("X-API-Key", config.ApiKey)

	if config.username != "" || config.password != "" {
		req.SetBasicAuth(config.username, config.password)
	}

	response, err := client.Do(req)

	if err != nil {
		log.Printf("ERROR: %s\n", err)
		return "", err
	} else {
		defer response.Body.Close()
		contents, err := ioutil.ReadAll(response.Body)
		if response.StatusCode == 401 {
			log.Fatal("Invalid username or password")
		}
		if err != nil {
			log.Printf("ERROR: %s\n", err)
			return "", err
		}
		return string(contents), err
	}
	return "", err
}

func main_loop() {
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

	for {
		res, err := query_syncthing(fmt.Sprintf("%s/rest/events?since=%d", config.Url, since_events))

		if err != nil { //usually connection error -> continue
			log.Println(err)
			initialize()
		} else {
			var events []event
			err = json.Unmarshal([]byte(res), &events)
			if err != nil {
				log.Println(err)
				initialize()
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
				}
				since_events = event.ID
			}

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
		trayhost.SetIcon(trayhost.ICON_NOT_CONNECTED)
		log.Println("not connected")
	} else if downloading && uploading {
		//ul+dl
		log.Println("ul+dl")
		trayhost.SetIcon(trayhost.ICON_UL_DL)
	} else if downloading && !uploading {
		//dl
		log.Println("dl")
		trayhost.SetIcon(trayhost.ICON_DL)
	} else if !downloading && uploading {
		//ul
		log.Println("ul")
		trayhost.SetIcon(trayhost.ICON_UL)
	} else if !downloading && !uploading {
		//idle
		log.Println("idle")
		trayhost.SetIcon(trayhost.ICON_IDLE)
	}

}

func main() {

	url := flag.String("target", "http://localhost:8080", "Target Syncthing instance")
	user := flag.String("u", "", "User")
	pw := flag.String("p", "", "Password")
	insecure := flag.Bool("i", false, "skip verification of SSL certificate")
	flag.Parse()

	config.Url = *url
	config.username = *user
	config.password = *pw
	config.insecure = *insecure

	client = &http.Client{
		Transport: &http.Transport{
			Dial: func(netw, addr string) (net.Conn, error) {
				conn, err := net.DialTimeout(netw, addr, time.Second*10)
				if err != nil {
					return nil, err
				}
				//conn.SetDeadline(time.Now().Add(time.Second * 20))
				return conn, nil
			},
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: config.insecure,
			},
			ResponseHeaderTimeout: time.Minute * 6,
		},
	}

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

	trayhost.Initialize("Syncthing-Tray", icon_error, menuItems)
	trayhost.SetClickHandler(onClick)
	trayhost.SetIconImage(trayhost.ICON_ALTERNATIVE, icon_not_connected)
	trayhost.SetIconImage(trayhost.ICON_ATTENTION, icon_error)
	trayhost.SetIconImage(trayhost.ICON_IDLE, icon_idle)
	trayhost.SetIconImage(trayhost.ICON_NOT_CONNECTED, icon_not_connected)
	trayhost.SetIconImage(trayhost.ICON_DL, icon_dl)
	trayhost.SetIconImage(trayhost.ICON_UL, icon_ul)
	trayhost.SetIconImage(trayhost.ICON_ERROR, icon_error)
	trayhost.SetIconImage(trayhost.ICON_UL_DL, icon_ul_dl)

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
		updateStatus()
		main_loop()
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
