package main

import (
	"fmt"
	"github.com/alex2108/trayhost"
	"runtime"
	"time"
	"net"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"encoding/xml"
	"os"
	"log"
	"github.com/toqueteos/webbrowser"
)
const layout = "2006-01-02 at 15:04:05"
var client = &http.Client{}
var since_events=0

type event struct {
	ID   int                    `json:"id"`
	Type string                 `json:"type"`
	Time time.Time              `json:"time"`
	Data map[string]interface{} `json:"data"`
}



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
    connected bool
}
var node map[string]*Node

type Repo struct {
    id string
    completion int
    state string
    needFiles int
    sharedWith []string
}
var repo  map[string]*Repo


type Node_self struct {
    name string
    id string
    dl_completion int //0 syncing, 1 complete
    ul_completion int //0 syncing, 1 complete
    nodes_connected int
}

var node_self = Node_self{}

func get_connections() error {
    input,_ := query_syncthing(config.Url + "/rest/connections")
    
    var res map[string]interface{}
    err := json.Unmarshal([]byte(input), &res)
    

    for nodeId, _ := range node {
        node[nodeId].connected=false
    }
    
    for nodeId, _ := range res {
        //completion :=v.(map[string]interface{})["Completion"]
        if nodeId !="total" {
            //log.Printf("connected: %d",node_self.nodes_connected)
            node[nodeId].connected=true
        }
    }

    return err
}





func get_repo_state() error {
    for key,rep := range repo {
        //fmt.Println(rep.id)
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
                if m.Needfiles>0 {
                    repo[key].state="syncing"
                } else {
                    repo[key].state="idle"
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
        return "",err
    } else {
        defer response.Body.Close()
        contents, err := ioutil.ReadAll(response.Body)
        if err != nil {
            log.Printf("ERROR: %s\n", err)
            return "",err
        }
        return string(contents), err
    }
    return "",err
}

func get_config() error {
    //create empty state
    node = make(map[string]*Node)
    repo = make(map[string]*Repo)
    
    r_json,err := query_syncthing(config.Url + "/rest/config")
    
    if err == nil {
        type SyncthingConfigNode struct {
            Nodeid string
            Name string
        }
        type SyncthingConfigRepoNode struct {
            Nodeid string
        }
        
        type SyncthingConfigRepo struct {
            Id string
            Nodes []SyncthingConfigRepoNode
        }
        type SyncthingConfig struct {
            Nodes []SyncthingConfigNode
            Repositories []SyncthingConfigRepo
        }

        var m SyncthingConfig
        json_err := json.Unmarshal([]byte(r_json), &m)

        //fmt.Println(json_err)

        if json_err != nil {
            return json_err
        } else { // save config in structs
            
            //save Nodes
            //fmt.Println("Nodes:")
            for _,v := range m.Nodes{
                //fmt.Println(v.Nodeid, v.Name)
                node[v.Nodeid]=&Node{v.Name,-1,false}
            }
            //save Repos
            for _,v := range m.Repositories{
                //fmt.Println(v.Id)
                repo[v.Id]=&Repo{v.Id,0,"invald",0,make([]string,0)}
                for _,v2 := range v.Nodes {
                    repo[v.Id].sharedWith=append(repo[v.Id].sharedWith,v2.Nodeid)
                }
            }
            
        }
    } else {
        return err
    }
    
    node_self.name="invalid"
    node_self.id="invalid"
    node_self.dl_completion=0
    node_self.ul_completion=0
    node_self.nodes_connected=0
    

    //Display version
    resp,err := query_syncthing(config.Url + "/rest/version")
    if err == nil {
        trayhost.UpdateCh <- trayhost.MenuItemUpdate{0, trayhost.MenuItem{
				    fmt.Sprintf("Syncthing: %s", resp),
				    true,
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
}

func update_self_status(){
    node_self.nodes_connected=0
    for _,v := range node {
        if v.connected {
            //log.Printf("%s is connected", v.name)
            node_self.nodes_connected++

        }
    }
    trayhost.UpdateCh <- trayhost.MenuItemUpdate{2, trayhost.MenuItem{
			    fmt.Sprintf("Connected to %d Nodes", node_self.nodes_connected),
			    true,
			    nil,
		    },
		    }

    
    
    node_self.dl_completion=1
    for _,v := range repo {
        if v.state=="syncing" {
            log.Printf("%s is syncing, setting state to downloading", v.id)
            node_self.dl_completion=0
        }
    }
}

func update_icon(){
    log.Println("Updating icon...")
    

    
    update_self_status()


    if node_self.nodes_connected==0 {
        //not connected
        trayhost.SetIcon(trayhost.ICON_NOT_CONNECTED)
        log.Println("not connected")
    } else if node_self.ul_completion==0 && node_self.dl_completion==0 {
        //ul+dl
        log.Println("ul+dl")
        trayhost.SetIcon(trayhost.ICON_UL_DL)
    } else if node_self.ul_completion==1 && node_self.dl_completion==0 {
        //dl
        log.Println("dl")
        trayhost.SetIcon(trayhost.ICON_DL)
    } else if node_self.ul_completion==0 && node_self.dl_completion==1 {
        //ul
        log.Println("ul")
        trayhost.SetIcon(trayhost.ICON_UL)
    } else if node_self.ul_completion==1 && node_self.dl_completion==1 {
        //idle
        log.Println("idle")
        trayhost.SetIcon(trayhost.ICON_IDLE)
    }
}


func initialize(){
    log.Println("Starting Syncthing-Tray")
    
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

func reinitialize(){
    log.Println("reinitializing")
    since_events=0
    err :=get_tray_config()
    if err != nil {
        log.Println("Config not found or other error with config, exiting")
        panic(err) 
    }
    log.Println("reading syncthing config")
    err =get_config()
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
        log.Println("reading repo state")
        get_repo_state()
        log.Println("reading connections")
        get_connections()
        log.Println("reading upload status")
        update_ul()
        update_icon()
    }
}

func set_latest_id() error{
		req, err := http.NewRequest("GET", fmt.Sprintf("%s/rest/events?since=%d", config.Url, since_events), nil)
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Set("X-API-Key", config.ApiKey)
		res, err := client.Do(req)
		if err != nil {
		    log.Printf("Connection Error:")
		    log.Printf(time.Now().Format(layout))
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
			//bs, _ := json.MarshalIndent(event, "", "    ")
			//log.Printf("%s", bs)
			since_events = event.ID
		}

        return nil
}


func update_ul() error {

    type Completion struct {
        Completion float64
    }
    node_self.ul_completion=1
    for r,r_info := range repo {
        for _,n := range r_info.sharedWith {
            if node[n].connected { // only query connected nodes
                //log.Println("node="+ n +"repo=" + r)
                out,err :=query_syncthing(config.Url + "/rest/completion?node="+ n +"&repo=" + r)
                //log.Println(out)
                if err!= nil{
                    log.Println(err)
                    return err
                }


                var m Completion
                err = json.Unmarshal([]byte(out), &m)
                if err!= nil{
                    log.Println(err)
                    return err
                }
                if m.Completion<100{ //any repo on any node not updated -> uploading
                    node_self.ul_completion=0
                }
            }
        }
        
    }
    
    return nil
}


func main_loop() {
    errors :=false


	log.SetOutput(os.Stdout)
	//log.SetFlags(0)

    //target:="localhost:8080"
    //apikey:="asdf"
	//since_events = 0
	for {
	    ulOutdated:=false
		req, err := http.NewRequest("GET", fmt.Sprintf("%s/rest/events?since=%d", config.Url, since_events), nil)
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Set("X-API-Key", config.ApiKey)
		res, err := client.Do(req)
		if err != nil { //usually connection error -> continue
		    //log.Printf("Connection Error:")
			log.Println(err)
			errors=true
		} else {

		    var events []event
		    err = json.NewDecoder(res.Body).Decode(&events)
		    if err != nil {
			    log.Fatal(err)
		    }
		    res.Body.Close()

		    for _, event := range events {
			    //bs, _ := json.MarshalIndent(event, "", "    ")
			    //log.Printf("%s", bs)
			
		        if(event.Type=="StateChanged" &&
		                event.Data["to"].(string) != "scanning" && 
		                event.Data["to"].(string) != "cleaning" && 
		                repo[event.Data["repo"].(string)].state != event.Data["to"].(string)){
		            log.Printf("Changed state")
		            str:=fmt.Sprintf("%s: %s -> %s",event.Data["repo"].(string),repo[event.Data["repo"].(string)].state,event.Data["to"].(string))
		            log.Printf(str)
		            repo[event.Data["repo"].(string)].state=event.Data["to"].(string)
		            update_icon()
		        } else if(event.Type=="NodeConnected"){
		            str:=fmt.Sprintf("connected: %s",event.Data["id"].(string))
		            log.Printf(str)

		            node[event.Data["id"].(string)].connected=true
		            update_icon()
		        } else if(event.Type=="NodeDisconnected"){
		            str:=fmt.Sprintf("disconnected: %s",event.Data["id"].(string))
		            log.Printf(str)

		            node[event.Data["id"].(string)].connected=false
		            ulOutdated=true //we maybe uploaded to this node
		            update_icon()
		        } else if event.Type=="RemoteIndexUpdated" || event.Type=="LocalIndexUpdated"{
		            ulOutdated=true
		        }
			    since_events = event.ID

		    }
		
	        if ulOutdated {
	            update_ul()
	            update_icon()
	            time.Sleep(1 * time.Second) //sleep 1 second to prevent doing this because of too many "IndexUpdated" events
	        }	
	
	    }
	    
    if errors {
        log.Printf("Found errors -> reinitialize")
        errors=false
        reinitialize()
    }
	}
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
			"waiting for response of syncthing",
			true,
			nil,
		},
		3: trayhost.MenuItem{
			"config not read yet...",
			true,
			nil,
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
	    initialize()
		main_loop()
	}()

	// Enter the host system's event loop
	trayhost.EnterLoop()

	// This is only reached once the user chooses the Exit menu item
	fmt.Println("Exiting")
}

func onClick() { // not usable on ubuntu
	fmt.Println("You clicked tray icon")
}
