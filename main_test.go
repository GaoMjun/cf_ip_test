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

func TestPing(t *testing.T) {
	log.Println(ping("1.1.1.1", 10))
}

func TestParsePingResponsePacketLoss(t *testing.T) {
	log.Print(parsePingResponsePacketLoss("5 packets transmitted, 4 packets received, 20.0% packet loss"))
}

func TestParsePingResponseAVG(t *testing.T) {
	log.Print(parsePingResponseAVG("round-trip min/avg/max/stddev = 164.740/165.221/165.544/0.305 ms"))
}
