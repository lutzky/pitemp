#!/bin/bash

if ! hash arm-linux-gnueabi-gcc > /dev/null; then
	echo "Error: arm-linux-gnueabi-gcc not installed" 1>&2
	exit 1
fi

export CC=arm-linux-gnueabi-gcc CGO_ENABLED=1 GOOS=linux GOARM=6 GOARCH=arm

# go build is cached, so rebuilding is cheap even if few files
# changed. Use "go clean -cache" for a full rebuild if necessary

for i in cmd/*; do
	echo "$i -> build/$(basename $i).arm"
	go build -o "build/$(basename $i).arm" ${i}/main.go
done