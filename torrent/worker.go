package torrent

import (
	"bittorrent/p2p"
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
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

func (t *Torrent) startWorker(peer Peer, infoHash [20]byte, peerID [20]byte, workQueue chan *PieceWork, results chan *PieceResult) {
	address := net.JoinHostPort(peer.IP.String(), strconv.Itoa(int(peer.Port)))

	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		fmt.Printf("[Worker] %s: Dial failed: %v\n", address, err)
		return
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))
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

	// Signal interest
	interested := p2p.Message{ID: p2p.MsgInterested}
	conn.Write(interested.Serialize())

	for pw := range workQueue {
		buf, err := t.attemptDownloadPiece(conn, pw)
		if err != nil {
			fmt.Printf("[Worker] %s: Piece %d failed: %v\n", address, pw.Index, err)
			workQueue <- pw
			return
		}
		results <- &PieceResult{Index: pw.Index, Buf: buf}
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
