#!/bin/bash

if ! hash arm-linux-gnueabi-gcc > /dev/null; then
	exit 1
fi

# TODO: This should be a Makefile

CC=arm-linux-gnueabi-gcc CGO_ENABLED=1 GOOS=linux GOARM=6 GOARCH=arm \
	go build -o build/pitemp.arm cmd/pitemp/main.go
CC=arm-linux-gnueabi-gcc CGO_ENABLED=1 GOOS=linux GOARM=6 GOARCH=arm \
	go build -o build/pitemp_pioled.arm cmd/pitemp_pioled/main.go
