package main

import (
	"bittorrent/torrent"
	"crypto/rand"
	"fmt"
	"log"
)

func main() {
	t, infoHash, err := torrent.Open("debian-13.3.0-amd64-netinst.iso.torrent")
	if err != nil {
		log.Fatal(err)
	}

	peerIDString := generatePeerID()
	var peerID [20]byte
	copy(peerID[:], peerIDString)

	fmt.Println("Contacting tracker...")
	url, _ := t.BuildTrackerURL(infoHash, peerIDString, 6881)
	peers, err := torrent.RequestPeers(url)
	if err != nil {
		log.Fatalf("Tracker error: %v", err)
	}

	fmt.Printf("Found %d peers. Starting download of: %s\n", len(peers), t.Info.Name)

	err = t.Download(peers, infoHash, peerID)
	if err != nil {
		log.Fatalf("Download failed: %v", err)
	}

	fmt.Println("\nDownload Complete! Check your folder.")
}

func generatePeerID() string {
	id := make([]byte, 12)
	rand.Read(id)
	return "-BT0001-" + string(id)
}
