package main

import (
	"encoding/json"
//	"encoding/xml"
	"flag"
	"fmt"
	"github.com/alex2108/trayhost"
	"github.com/toqueteos/webbrowser"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"
)

var client = &http.Client{}
var since_events = 0

type event struct {
	ID   int                    `json:"id"`
	Type string                 `json:"type"`
	Time time.Time              `json:"time"`
	Data map[string]interface{} `json:"data"`
}
/*
type Config struct {
	XMLName xml.Name `xml:"config"`
	Version int      `xml:"version,attr"`
	ApiKey  string   `xml:"apikey"`
	Url     string   `xml:"url"`
}*/

type Config struct {
	ApiKey  string
	Url     string
}

var config Config

type Device struct {
	name       string
	completion int
	connected  bool
}

var device map[string]*Device

type Folder struct {
	id         string
	completion int
	state      string
	needFiles  int
	sharedWith []string
}

var folder map[string]*Folder

type Device_self struct {
	name            string
	id              string
	dl_completion   int //0 syncing, 1 complete
	ul_completion   int //0 syncing, 1 complete
	devices_connected int
}

var device_self = Device_self{}

func get_connections() error {
	input, _ := query_syncthing(config.Url + "/rest/connections")

	var res map[string]interface{}
	err := json.Unmarshal([]byte(input), &res)

	for deviceId, _ := range device {
		device[deviceId].connected = false
	}

	for deviceId, _ := range res {
		//completion :=v.(map[string]interface{})["Completion"]
		if deviceId != "total" {
			log.Printf("connected: %d",device_self.devices_connected)
			device[deviceId].connected = true
		}
	}

	return err
}

func get_folder_state() error {
	for key, rep := range folder {
		r_json, err := query_syncthing(config.Url + "/rest/model?folder=" + rep.id)

		if err == nil {
			type Folderstate struct {
				Needfiles int
				State     string
			}

			var m Folderstate
			json_err := json.Unmarshal([]byte(r_json), &m)

			if json_err != nil {
				return json_err
			} else {
				if m.Needfiles > 0 {
					folder[key].state = "syncing"
				} else {
					folder[key].state = "idle"
				}

			}
		} else {
			return err
		}

	}

	return nil
}

func query_syncthing(url string) (string, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-API-Key", config.ApiKey)
	response, err := client.Do(req)

	if err != nil {
		log.Printf("ERROR: %s\n", err)
		return "", err
	} else {
		defer response.Body.Close()
		contents, err := ioutil.ReadAll(response.Body)
		if err != nil {
			log.Printf("ERROR: %s\n", err)
			return "", err
		}
		return string(contents), err
	}
	return "", err
}

func get_config() error {
	log.Println("reading config from syncthing")
	//create empty state
	device = make(map[string]*Device)
	folder = make(map[string]*Folder)

	r_json, err := query_syncthing(config.Url + "/rest/config")

	if err == nil {
		type SyncthingConfigDevice struct {
			Deviceid string
			Name   string
		}
		type SyncthingConfigFolderDevice struct {
			Deviceid string
		}

		type SyncthingConfigFolder struct {
			Id    string
			Devices []SyncthingConfigFolderDevice
		}
		type SyncthingConfig struct {
			Devices        []SyncthingConfigDevice
			Folders []SyncthingConfigFolder
		}

		var m SyncthingConfig
		json_err := json.Unmarshal([]byte(r_json), &m)

		if json_err != nil {
			return json_err
		} else { // save config in structs

			//save Devices
			for _, v := range m.Devices {
				device[v.Deviceid] = &Device{v.Name, -1, false}
			}
			//save Folders
			for _, v := range m.Folders {
				folder[v.Id] = &Folder{v.Id, 0, "invald", 0, make([]string, 0)}
				for _, v2 := range v.Devices {
					folder[v.Id].sharedWith = append(folder[v.Id].sharedWith, v2.Deviceid)
				}
			}

		}
	} else {
		return err
	}

	device_self.name = "invalid"
	device_self.id = "invalid"
	device_self.dl_completion = 0
	device_self.ul_completion = 0
	device_self.devices_connected = 0

	//Display version
	log.Println("getting version")
	resp, err := query_syncthing(config.Url + "/rest/version")
	if err == nil {
		type STVersion struct {
			Version     string
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

/*func get_tray_config() error {
	log.Println("reading tray config")
	b, err := ioutil.ReadFile("config.xml")
	if err != nil {
		panic(err)
	}
	err2 := xml.Unmarshal([]byte(b), &config)
	if err2 != nil {
		log.Printf("error: %v \n", err2)
		return err2
	}

	trayhost.UpdateCh <- trayhost.MenuItemUpdate{3, trayhost.MenuItem{
		"Open Syncthing GUI",
		false,
		func() {
			log.Println("Opening Browser")
			webbrowser.Open(config.Url)
		},
	},
	}

	return nil
}*/

func update_self_status() {
	log.Println("updating status")
	device_self.devices_connected = 0
	for _, v := range device {
		if v.connected {
			device_self.devices_connected++
		}
	}
	log.Printf("connected to %d devices", device_self.devices_connected)
	trayhost.UpdateCh <- trayhost.MenuItemUpdate{2, trayhost.MenuItem{
		fmt.Sprintf("Connected to %d Devices", device_self.devices_connected),
		true,
		nil,
	},
	}

	device_self.dl_completion = 1
	for _, v := range folder {
		if v.state == "syncing" {
			log.Printf("%s is syncing, setting state to downloading", v.id)
			device_self.dl_completion = 0
		}
	}
}

func update_icon() {
	log.Println("Updating icon...")
	update_self_status()

	if device_self.devices_connected == 0 {
		//not connected
		trayhost.SetIcon(trayhost.ICON_NOT_CONNECTED)
		log.Println("not connected")
	} else if device_self.ul_completion == 0 && device_self.dl_completion == 0 {
		//ul+dl
		log.Println("ul+dl")
		trayhost.SetIcon(trayhost.ICON_UL_DL)
	} else if device_self.ul_completion == 1 && device_self.dl_completion == 0 {
		//dl
		log.Println("dl")
		trayhost.SetIcon(trayhost.ICON_DL)
	} else if device_self.ul_completion == 0 && device_self.dl_completion == 1 {
		//ul
		log.Println("ul")
		trayhost.SetIcon(trayhost.ICON_UL)
	} else if device_self.ul_completion == 1 && device_self.dl_completion == 1 {
		//idle
		log.Println("idle")
		trayhost.SetIcon(trayhost.ICON_IDLE)
	}
}

func initialize() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	log.Println("Starting Syncthing-Tray")
	log.Println("Connecting to syncthing at",config.Url)
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
			ResponseHeaderTimeout: time.Minute * 6,
		},
	}

	reinitialize()
}

func reinitialize() {
	log.Println("(re)initializing")
	since_events = 0
	/*err := get_tray_config()
	if err != nil {
		log.Println("Config not found or other error with config, exiting")
		panic(err)
	}*/
	log.Println("reading syncthing config")
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
		update_self_status()
		trayhost.SetIcon(trayhost.ICON_ERROR)
		time.Sleep(5 * time.Second)
		reinitialize()
	} else {
		log.Println("reading past events")
		set_latest_id()
		log.Println("reading folder state")
		get_folder_state()
		log.Println("reading connections")
		get_connections()
		log.Println("reading upload status")
		update_ul()
		update_icon()
	}
}

func set_latest_id() error {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/rest/events?since=%d", config.Url, since_events), nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("X-API-Key", config.ApiKey)
	res, err := client.Do(req)
	if err != nil {
		log.Printf("Connection Error:")
		log.Println(err)
		return err
	}

	var events []event
	err = json.NewDecoder(res.Body).Decode(&events)
	if err != nil {
		log.Fatal(err)
		return err
	}
	res.Body.Close()

	for _, event := range events {
		since_events = event.ID
	}

	return nil
}

func update_ul() error {

	type Completion struct {
		Completion float64
	}
	device_self.ul_completion = 1
	for r, r_info := range folder {
		for _, n := range r_info.sharedWith {
			if device[n].connected { // only query connected devices
				//log.Println("device="+ n +"folder=" + r)
				out, err := query_syncthing(config.Url + "/rest/completion?device=" + n + "&folder=" + r)
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
				if m.Completion < 100 { //any folder on any device not updated -> uploading
					device_self.ul_completion = 0
				}
			}
		}
	}
	return nil
}

func main_loop() {
	errors := false

	//target:="localhost:8080"
	//apikey:="asdf"
	//since_events = 0
	for {
		ulOutdated := false
		req, err := http.NewRequest("GET", fmt.Sprintf("%s/rest/events?since=%d", config.Url, since_events), nil)
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Set("X-API-Key", config.ApiKey)
		res, err := client.Do(req)
		if err != nil { //usually connection error -> continue
			log.Println(err)
			errors = true
		} else {

			var events []event
			err = json.NewDecoder(res.Body).Decode(&events)
			if err != nil {
				log.Println(err)
				errors = true
			}
			res.Body.Close()

			for _, event := range events {
				// handle different events
				if event.Type == "StateChanged" &&
				event.Data["to"].(string) != "scanning" && // don't display scanning and cleaning
				event.Data["to"].(string) != "cleaning" &&
				folder[event.Data["folder"].(string)].state != event.Data["to"].(string) {
					log.Printf("Changed state")
					str := fmt.Sprintf("%s: %s -> %s", event.Data["folder"].(string), folder[event.Data["folder"].(string)].state, event.Data["to"].(string))
					log.Printf(str)
					folder[event.Data["folder"].(string)].state = event.Data["to"].(string)
					update_icon()
				} else if event.Type == "DeviceConnected" {
					str := fmt.Sprintf("connected: %s", event.Data["id"].(string))
					log.Printf(str)

					device[event.Data["id"].(string)].connected = true
					update_icon()
				} else if event.Type == "DeviceDisconnected" {
					str := fmt.Sprintf("disconnected: %s", event.Data["id"].(string))
					log.Printf(str)
					device[event.Data["id"].(string)].connected = false
					ulOutdated = true // we maybe uploaded to this device
					update_icon()
				} else if event.Type == "RemoteIndexUpdated" || event.Type == "LocalIndexUpdated" {
					ulOutdated = true
				}
				since_events = event.ID
			}

			if ulOutdated {
				update_ul()
				update_icon()
				time.Sleep(1 * time.Second) // sleep 1 second to prevent doing this too often because many "IndexUpdated" can be sent in a short time
			}
		}
		if errors {
			log.Printf("Found errors -> reinitialize")
			errors = false
			reinitialize()
		}
	}
}

func main() {

	url := flag.String("target", "http://localhost:8080", "Target Syncthing instance")
	config.ApiKey = *flag.String("apikey", "", "Syncthing API key (currently not needed)")
	flag.Parse()

	/*if *apikey == "" {
		log.Fatal("Must give -apikey argument")
	}*/
	config.Url = *url;

	
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

	trayhost.Initialize("Syncthing-Tray", icon_not_connected, menuItems)
	trayhost.SetClickHandler(onClick)
	trayhost.SetIconImage(trayhost.ICON_ALTERNATIVE, icon_not_connected)
	trayhost.SetIconImage(trayhost.ICON_ATTENTION, icon_error)
	trayhost.SetIconImage(trayhost.ICON_IDLE, icon_idle)
	trayhost.SetIconImage(trayhost.ICON_NOT_CONNECTED, icon_not_connected)
	trayhost.SetIconImage(trayhost.ICON_DL, icon_dl)
	trayhost.SetIconImage(trayhost.ICON_UL, icon_ul)
	trayhost.SetIconImage(trayhost.ICON_ERROR, icon_error)
	trayhost.SetIconImage(trayhost.ICON_UL_DL, icon_ul_dl)

	
	
	go func() {
		initialize()
		main_loop()
	}()

	// Enter the host system's event loop
	trayhost.EnterLoop()

	// This is only reached once the user chooses the Exit menu item
	fmt.Println("Exiting")
}

func onClick() { // not usable on ubuntu, left click also displays the menu
	fmt.Println("Opening webinterface in browser")
	webbrowser.Open(config.Url)
}
