package main

import (
	"log"
	"testing"
	"time"
)

func TestDownload(t *testing.T) {
	speed := download("https://universal.bigbuckbunny.workers.dev/Consti10/LiveVideo10ms/master/Screenshots/device2.png?xprotocol=https&xhost=raw.githubusercontent.com", "104.21.90.173", 50*time.Second)

	log.Println((speed / 1024))
}
