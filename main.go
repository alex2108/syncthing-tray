package main

import (
	"fmt"
	"github.com/overlordtm/trayhost"
	"runtime"
	"time"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"encoding/xml"
//	"os"
)
type Config struct {
	XMLName   xml.Name `xml:"config"`
	Version        int      `xml:"version,attr"`
	ApiKey string   `xml:"apikey"`
	Url  string   `xml:"url"`
}
var config Config

type Node struct {
    name string
    completion int
}
var node map[string]*Node

type Repo struct {
    id string
    completion int
    state string
    needFiles int
}
var repo []Repo


type Node_self struct {
    name string
    id string
    dl_completion int //0 syncing, 1 complete
    ul_completion int //-1 not connected, 0 syncing, 1 complete
}

var node_self = Node_self{}

func get_connections() error {
    input,_ := query_syncthing(config.Url + "/rest/connections")
    
    var res map[string]interface{}
    err := json.Unmarshal([]byte(input), &res)
    
    
    node_self.ul_completion=1 //standard, changed if no nodes connected or not synced
    
    if len(res)<2 { //only entry "total"
        node_self.ul_completion=-1
    }
    
    for nodeId, v := range res {
        completion :=v.(map[string]interface{})["Completion"]
        if nodeId !="total" {
            fmt.Println(nodeId, "Completion: ",completion)
            //TODO: check if exists
            node[nodeId].completion=int(completion.(float64))
            
            if int(completion.(float64)) <100 {
                node_self.ul_completion=0
            } 
        }
    }

    
    return err
}





func get_repo_state() error {

    totalNeedFiles := 0

    for _,rep := range repo {
        fmt.Println(rep.id)
        r_json,err := query_syncthing(config.Url + "/rest/model?repo=" + rep.id)
        
        if err == nil {
            type Repostate struct {
                Needfiles int
                State string
            }

            var m Repostate
            json_err := json.Unmarshal([]byte(r_json), &m)

            if json_err != nil {
                return json_err
            } else {
                fmt.Println(m)
                totalNeedFiles += m.Needfiles
            }
        } else {
            return err
        }
        
    }
    
    if(totalNeedFiles==0){
        node_self.dl_completion=1
        
    } else {
        node_self.dl_completion=0
    }
    
    return nil
}



func query_syncthing(url string) (string, error) {
    
    client := &http.Client{}
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("X-API-Key", config.ApiKey)
    response, err := client.Do(req)    
    
    if err != nil {
        fmt.Printf("ERROR: %s\n", err)
        return "",err
    } else {
        defer response.Body.Close()
        contents, err := ioutil.ReadAll(response.Body)
        if err != nil {
            fmt.Printf("ERROR: %s\n", err)
            return "",err
        }
        //fmt.Printf("%s\n", string(contents))
        return string(contents), err
    }
    return "",err
}

func get_config() error {
    //create empty state
    node = make(map[string]*Node)
    repo=make([]Repo,0)
    
    r_json,err := query_syncthing(config.Url + "/rest/config")
    
    if err == nil {
        type SyncthingConfigNode struct {
            Nodeid string
            Name string
        }
        type SyncthingConfigRepo struct {
            Id string
        }
        type SyncthingConfig struct {
            Nodes []SyncthingConfigNode
            Repositories []SyncthingConfigRepo
        }

        var m SyncthingConfig
        json_err := json.Unmarshal([]byte(r_json), &m)

        fmt.Println(json_err)

        if json_err != nil {
            return json_err
        } else { // save config in structs
            
            //save Nodes
            for _,v := range m.Nodes{
                fmt.Println(v.Nodeid, v.Name)
                node[v.Nodeid]=&Node{v.Name,-1}
            }
            //save Repos
            for _,v := range m.Repositories{
                fmt.Println(v.Id)
                repo=append(repo,Repo{v.Id,0,"invald",0})
            }
            
        }
    } else {
        return err
    }
    
    node_self.name="invalid"
    node_self.id="invalid"
    node_self.dl_completion=0
    node_self.ul_completion=-1

    

    
    resp,err := query_syncthing(config.Url + "/rest/version")
    
    if err == nil {
        trayhost.UpdateCh <- trayhost.MenuItemUpdate{0, trayhost.MenuItem{
				    fmt.Sprintf("Syncthing: %s", resp),
				    false,
				    nil,
			    },
			    }
    } else{
        trayhost.UpdateCh <- trayhost.MenuItemUpdate{0, trayhost.MenuItem{
				    fmt.Sprintf("Syncthing: Not connected"),
				    false,
				    nil,
			    },
			    }
    }
    return err
}


func get_tray_config() error {
    b, err := ioutil.ReadFile("config.xml")
    if err != nil { panic(err) }
	err2 := xml.Unmarshal([]byte(b), &config)
	if err2 != nil {
		fmt.Printf("error: %v \n", err2)
		return err2
	}
    return nil
}


func update_icon(){
    fmt.Println("Updating icon...")
    get_config()
    get_connections()
    get_repo_state()
    
    
    if node_self.ul_completion==-1 {
        //not connected
        trayhost.SetIcon(trayhost.ICON_NOT_CONNECTED)
        fmt.Println("not connected")
    } else if node_self.ul_completion==0 && node_self.dl_completion==0 {
        //ul+dl
        fmt.Println("ul+dl")
        trayhost.SetIcon(trayhost.ICON_UL_DL)
    } else if node_self.ul_completion==1 && node_self.dl_completion==0 {
        //dl
        fmt.Println("dl")
        trayhost.SetIcon(trayhost.ICON_DL)
    } else if node_self.ul_completion==0 && node_self.dl_completion==1 {
        //ul
        fmt.Println("ul")
        trayhost.SetIcon(trayhost.ICON_UL)
    } else if node_self.ul_completion==1 && node_self.dl_completion==1 {
        //idle
        fmt.Println("idle")
        trayhost.SetIcon(trayhost.ICON_IDLE)
    }
}


func initialize(){
    fmt.Println("Starting Syncthing-Tray")
    err :=get_tray_config()
    if err != nil { panic(err) }
}

func main() {
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
			"placeholder",
			false,
			func() {
				fmt.Println("item A")
			},
		},
		3: trayhost.MenuItem{
			"Item B\nItem B1",
			true,
			nil,
		},
		4: trayhost.MenuItem{
			fmt.Sprintf("another placeholder"),
			false,
			nil,
		},
		5: trayhost.MenuItem{
			"Exit",
			false,
			trayhost.Exit,
		}}

	trayhost.Initialize("Trayhost example", icon_not_connected, menuItems)
	trayhost.SetClickHandler(onClick)
	trayhost.SetIconImage(trayhost.ICON_ALTERNATIVE, icon_not_connected)
	trayhost.SetIconImage(trayhost.ICON_ATTENTION, icon_error)
    trayhost.SetIconImage(trayhost.ICON_IDLE, icon_idle)
    trayhost.SetIconImage(trayhost.ICON_NOT_CONNECTED, icon_not_connected)
    trayhost.SetIconImage(trayhost.ICON_DL, icon_dl)
    trayhost.SetIconImage(trayhost.ICON_UL, icon_ul)
    trayhost.SetIconImage(trayhost.ICON_ERROR, icon_error)
    trayhost.SetIconImage(trayhost.ICON_UL_DL, icon_ul_dl)

    
    initialize()
    
    /*
	go func() {
		for now := range time.Tick(1 * time.Second) {
			trayhost.UpdateCh <- trayhost.MenuItemUpdate{4, trayhost.MenuItem{
				fmt.Sprintf("Time: %v", now),
				false,
				nil,
			},
			}
		}
	}()
    */
	go func() {
		for _ = range time.Tick(2 * time.Second) {
			update_icon()
		}
	}()

	// Enter the host system's event loop
	trayhost.EnterLoop()

	// This is only reached once the user chooses the Exit menu item
	fmt.Println("Exiting")
}

func onClick() { // not usable on ubuntu
	fmt.Println("You clicked tray icon")
}
