syncthing-tray
==============
Simple tray application for [syncthing](https://github.com/syncthing/syncthing/)

Connects to syncthing at `http://localhost:8384` or any other url by setting the command line parameter ` -target="http://localhost:8384"`. 

A syncthing api key needs to be provided via `-api STAPIKEY`

Releases
========

Binary releases for Windows and Linux are available on the [releases tab](https://github.com/alex2108/syncthing-tray/releases).

OSX binaries are not provided here but can be built from source and are included in [syncthing-mac](https://github.com/xor-gate/syncthing-mac/releases).

Building
========

The following packages on Ubuntu 14.04/16.04 are needed: `libgtk-3-dev libappindicator3-dev`. On other distributions other packages may be needed.

Windows binaries can be cross compiled from Linux using mingw.
Example:
```
CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build -i -v -ldflags -H=windowsgui github.com/alex2108/syncthing-tray
```
The option `-ldflags -H=windowsgui` prevents a console window from being shown and can be removed to see the log for debugging.
