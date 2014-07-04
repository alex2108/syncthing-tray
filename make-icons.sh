#/bin/sh
OUTPUT=icon_unix.go

echo "//+build linux darwin" > "$OUTPUT"
echo "" >> "$OUTPUT"
echo "package main" >> "$OUTPUT"
echo "" >> "$OUTPUT"
for ICON in "icon_dl" "icon_error" "icon_idle" "icon_not_connected" "icon_ul" "icon_ul_dl"
do
    $GOPATH/bin/2goarray $ICON main < img/$ICON.png |  grep -v package >> "$OUTPUT"
done
