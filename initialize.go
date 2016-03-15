package main

import (
	"encoding/json"
	"fmt"
	"github.com/thomasf/systray"
	"log"
	"math"
	"time"
)

func get_folder_state() error {
	for key, rep := range folder {
		mutex.Lock()
		if folder[key].completion >= 0 {
			log.Println("already got info for folder",key,"from events, skipping")
			mutex.Unlock()
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
				mutex.Unlock()
				return json_err
			} else {

				folder[key].state = m.State
				folder[key].needFiles = m.NeedFiles
				folder[key].completion = 100 - 100*float64(m.NeedFiles)/math.Max(float64(m.GlobalFiles), 1) // max to prevent division by zero

			}
		} else {
			mutex.Unlock()
			return err
		}
		mutex.Unlock()
		// let events be processed, might save some expensive api calls
		for len(eventChan)>0 {
			time.Sleep(time.Millisecond)
		}
	}

	return nil
}
func get_connections() error {
	mutex.Lock()
	defer mutex.Unlock()
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
			mutex.Lock()
			if device[n].folderCompletion[r] >= 0 {
				log.Println("already got info for device",n,"folder",r,"from events, skipping")
				mutex.Unlock()
				continue
			}
		
			if device[n].connected { // only query connected devices
				out, err := query_syncthing(config.Url + "/rest/db/completion?device=" + n + "&folder=" + r)
				log.Println("updating upload status for device",n,"folder",r)
				if err != nil {
					log.Println(err)
					mutex.Unlock()
					return err
				}
				var m Completion
				err = json.Unmarshal([]byte(out), &m)
				if err != nil {
					log.Println(err)
					mutex.Unlock()
					return err
				}
				device[n].folderCompletion[r] = m.Completion
			}
			mutex.Unlock()
			// let events be processed, might save some expensive api calls
			for len(eventChan)>0 {
				time.Sleep(time.Millisecond)
			}
		}
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

// helper to get a lock before starting the new thread that can run in background after a lock is aquired
func initialize() {
	// block all before config is read
	
	log.Println("wating for lock")
	mutex.Lock()
	log.Println("wating for event lock")
	eventMutex.Lock() 
	go initializeLocked()
}


func initializeLocked() {

	currentStartTime, err := getStartTime()
	if err == nil {
		
		if startTime != currentStartTime {
			log.Println("syncthing restarted at",currentStartTime)
			startTime = currentStartTime
			since_events = 0 
		}
		err = get_config()
	}
	eventChan = make(chan event,10000)
	go eventProcessor() //start processing events again
	eventMutex.Unlock()
	mutex.Unlock()
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
		eventMutex.Lock() 
		mutex.Lock()
		log.Println(err)
		log.Println("error getting syncthing config -> retry in 5s")


		trayMutex.Lock()
		trayEntries.stVersion.SetTitle(fmt.Sprintf("Syncthing: no connection to " + config.Url))
		trayMutex.Unlock()

		systray.SetIcon(icon_error)
		time.Sleep(5 * time.Second)
		initializeLocked()
		return
	}
	updateStatus()

	

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
			trayMutex.Lock()
			trayEntries.stVersion.SetTitle(fmt.Sprintf("Syncthing: %s", m.Version))
			trayMutex.Unlock()
			
		}
	}
	return err
}
