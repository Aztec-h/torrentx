package torrent

import (
	"bittorrent/p2p"
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// pieceWork is what we send to a peer
type PieceWork struct {
	Index  int
	Hash   [20]byte
	Length int
}

// pieceResult is what we get back from a peer
type PieceResult struct {
	Index int
	Buf   []byte
}

type WorkerStatus struct {
	Address string
	Piece   int
	Status  string // "Connecting", "Downloading", "Choked", "Idle"
}

func (t *Torrent) startWorker(peer Peer, infoHash [20]byte, peerID [20]byte, workQueue chan *PieceWork, results chan *PieceResult, ws *WorkerStatus) {
	address := net.JoinHostPort(peer.IP.String(), strconv.Itoa(int(peer.Port)))
	ws.Address = address
	ws.Status = "Connecting"

	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		fmt.Printf("[Worker] %s: Dial failed: %v\n", address, err)
		return
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))
	ws.Status = "Handshaking"
	hs := p2p.Handshake{Pstr: "BitTorrent protocol", InfoHash: infoHash, PeerID: peerID}
	if _, err := conn.Write(hs.Serialize()); err != nil {
		fmt.Printf("[Worker] %s: Handshake send failed: %v\n", address, err)
		return
	}
	res, err := p2p.Unserialize(conn)
	if err != nil {
		fmt.Printf("[Worker] %s: Handshake read failed: %v\n", address, err)
		return
	}
	if !bytes.Equal(res.InfoHash[:], infoHash[:]) {
		fmt.Printf("[Worker] %s: InfoHash mismatch\n", address)
		return
	}
	conn.SetDeadline(time.Time{})

	fmt.Printf("[Worker] %s: Handshake SUCCESS\n", address)
	ws.Status = "Interested"
	// Signal interest
	interested := p2p.Message{ID: p2p.MsgInterested}
	conn.Write(interested.Serialize())

	for pw := range workQueue {
		ws.Piece = pw.Index
		ws.Status = "Downloading"

		buf, err := t.attemptDownloadPiece(conn, pw)
		if err != nil {
			fmt.Printf("[Worker] %s: Piece %d failed: %v\n", address, pw.Index, err)
			ws.Status = "Error"
			workQueue <- pw
			return
		}
		results <- &PieceResult{Index: pw.Index, Buf: buf}
		ws.Status = "Idle"
	}
}

func (t *Torrent) attemptDownloadPiece(conn net.Conn, pw *PieceWork) ([]byte, error) {
	buf := make([]byte, pw.Length)
	var requested, downloaded int
	const maxBacklog = 10
	const blockSize = 16384

	conn.SetDeadline(time.Now().Add(30 * time.Second))
	defer conn.SetDeadline(time.Time{})

	for downloaded < pw.Length {
		for requested < pw.Length && (requested-downloaded) < maxBacklog*blockSize {
			length := blockSize
			if pw.Length-requested < length {
				length = pw.Length - requested
			}

			req := p2p.FormatRequest(pw.Index, requested, length)
			_, err := conn.Write(req.Serialize())
			if err != nil {
				return nil, err
			}
			requested += length
		}

		msg, err := p2p.Read(conn)
		if err != nil {
			return nil, err
		}
		if msg == nil {
			continue
		}

		if msg.ID == p2p.MsgPiece {
			begin := int(binary.BigEndian.Uint32(msg.Payload[4:8]))
			blockData := msg.Payload[8:]
			copy(buf[begin:], blockData)
			downloaded += len(blockData)
		}
	}
	return buf, nil
}

// status display
func (t *Torrent) DisplayStats(stats []WorkerStatus, doneCount *int, total int) {
	for {
		// ANSI escape code to clear screen and move cursor to top-left
		fmt.Print("\033[H\033[2J")

		fmt.Printf("File: %s\n", t.Info.Name)
		pct := float64(*doneCount) / float64(total) * 100

		// Simple progress bar
		barLen := 40
		filled := int(float64(barLen) * (float64(*doneCount) / float64(total)))
		bar := strings.Repeat("█", filled) + strings.Repeat("-", barLen-filled)

		fmt.Printf("[%s] %.2f%% (%d/%d pieces)\n", bar, pct, *doneCount, total)
		fmt.Println(strings.Repeat("=", 50))
		fmt.Printf("%-20s %-8s %-12s\n", "PEER", "PIECE", "STATUS")

		// Show only the first 15 active peers to keep the screen clean
		count := 0
		for _, s := range stats {
			if s.Status == "Downloading" || s.Status == "Interested" {
				fmt.Printf("%-20s %-8d %-12s\n", s.Address, s.Piece, s.Status)
				count++
			}
			if count > 15 {
				break
			}
		}

		time.Sleep(200 * time.Millisecond)
		if *doneCount >= total {
			break
		}
	}
}
