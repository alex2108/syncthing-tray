syncthing-tray
==============
Simple tray application for [syncthing](https://github.com/calmh/syncthing/)

Connects to syncthing at `http://localhost:8384` or any other url by setting the command line parameter ` -target="http://localhost:8384"`


Building
========

the multi-icon branch of [trayhost](https://github.com/overlordtm/trayhost/tree/feature-multi-icon) has to be in your src directory and it has to be modified for more icons, just edit the part where icon names are defined in `trayhost.go` to
```
const (
	ICON_PRIMARY     = iota
	ICON_ALTERNATIVE = iota
	ICON_ATTENTION   = iota
	ICON_IDLE        = iota
	ICON_NOT_CONNECTED = iota
	ICON_DL     = iota
	ICON_UL     = iota
	ICON_ERROR     = iota
	ICON_UL_DL     = iota
	ICON_UL_DL2     = iota
)
```

the following packages on Ubuntu 12.04 are needed: `libgtk2.0-dev libappindicator-dev`
