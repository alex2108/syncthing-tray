#/bin/sh
OUTPUT=icon_unix.go

echo "//+build linux darwin" > "$OUTPUT"
echo "" >> "$OUTPUT"
echo "package main" >> "$OUTPUT"
echo "" >> "$OUTPUT"
for ICON in "icon_dl" "icon_error" "icon_idle" "icon_not_connected" "icon_ul" "icon_ul_dl"
do
    convert -background none img/$ICON.svg -resize 32x32 img/$ICON.png
    $GOPATH/bin/2goarray $ICON main < img/$ICON.png |  grep -v package >> "$OUTPUT"
done


OUTPUT=icon_windows.go

echo "//+build windows" > "$OUTPUT"
echo "" >> "$OUTPUT"
echo "package main" >> "$OUTPUT"
echo "" >> "$OUTPUT"
for ICON in "icon_dl" "icon_error" "icon_idle" "icon_not_connected" "icon_ul" "icon_ul_dl"
do
    convert img/$ICON.png img/$ICON.ico
    $GOPATH/bin/2goarray $ICON main < img/$ICON.ico |  grep -v package >> "$OUTPUT"
done
