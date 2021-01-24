#!/bin/bash

if ! hash arm-linux-gnueabi-gcc > /dev/null; then
	exit 1
fi

CC=arm-linux-gnueabi-gcc CGO_ENABLED=1 GOOS=linux GOARM=6 GOARCH=arm \
	go build -o build/main.arm main.go
