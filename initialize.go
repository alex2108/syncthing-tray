package main

import (
	"encoding/json"
	"fmt"
	"github.com/AislerHQ/trayhost"
	"log"
	"math"
	"time"
)

func get_folder_state() error {
	for key, rep := range folder {
		if folder[key].completion >= 0 {
			log.Println("already got info for folder",key,"from events, skipping")
			continue
		}
	
		r_json, err := query_syncthing(config.Url + "/rest/db/status?folder=" + rep.id)
		log.Println("getting state for folder",rep.id)
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
		
		// read events again to possibly prevent more expensive api calls
		since_events = since_events-1 // instantly return even if no new events are there
		readEvents()

	}

	return nil
}
func get_connections() error {
	log.Println("getting connections")
	input, err := query_syncthing(config.Url + "/rest/system/connections")
	if err != nil {
		log.Println(err)
		return err
	}
	var res map[string]interface{}
	err = json.Unmarshal([]byte(input), &res)

	for deviceId, _ := range device {
		device[deviceId].connected = false
	}

	for deviceId, m := range res["connections"].(map[string]interface{}) {
		connectionState := m.(map[string]interface{})
		device[deviceId].connected = connectionState["connected"].(bool)
	}

	return err
}
func update_ul() error {
	
	type Completion struct {
		Completion float64
	}
	for r, r_info := range folder {
		for _, n := range r_info.sharedWith {
			if device[n].folderCompletion[r] >= 0 {
				log.Println("already got info for device",n,"folder",r,"from events, skipping")
				continue
			}
		
			if device[n].connected { // only query connected devices
				out, err := query_syncthing(config.Url + "/rest/db/completion?device=" + n + "&folder=" + r)
				log.Println("updating upload status for device",n,"folder",r)
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
			// read events again to possibly prevent more expensive api calls
			since_events = since_events-1 // instantly return even if no new events are there
			readEvents()
			
		}
	}
	return nil
}
func set_latest_id() error {
	log.Println("reading latest event id")
	type event struct {
		ID int `json:"id"`
	}

	res, err := query_syncthing(fmt.Sprintf("%s/rest/events?since=%d", config.Url, since_events))

	if err != nil {
		log.Println(err)
		return err
	}

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

func getStartTime() (string, error){


	type StStatus struct {
		StartTime string
	}
	out, err := query_syncthing(config.Url + "/rest/system/status")
	
	if err != nil {
		log.Println(err)
		return "", err
	}
	var m StStatus
	err = json.Unmarshal([]byte(out), &m)
	if err != nil {
		log.Println(err)
		return "",err
	}

	return m.StartTime, nil


}





func initialize() {
	currentStartTime, err := getStartTime()
	if err == nil {
		
		if startTime != currentStartTime {
			log.Println("syncthing restarted at",currentStartTime)
			startTime = currentStartTime
			since_events = 0 
		}
		err = get_config()
	}
	// read events at the beginning to already set some values to decrease the number of expensive calls
	if err == nil {
		err = readEvents()
	}
	// get current state
	if err == nil {
		err = get_folder_state()
	}
	if err == nil {
		err = get_connections()
	}
	if err == nil {
		err = update_ul()
	}
	
	if err != nil {
		log.Println(err)
		log.Println("error getting syncthing config -> retry in 5s")

		trayhost.UpdateCh <- trayhost.MenuItemUpdate{0, trayhost.MenuItem{
			fmt.Sprintf("Syncthing: no connection to " + config.Url),
			true,
			nil,
		},
		}
		err = trayhost.SetIcon(ICON_ERROR)
		log.Println(err)
		time.Sleep(5 * time.Second)
		defer initialize()
		return
	}
	
	updateStatus()
	log.Println("starting main loop")
	main_loop()

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
				folder[v.Id] = &Folder{v.Id, -1, "invalid", 0, make([]string, 0)} //id, completion, state, needFiles, sharedWith
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
