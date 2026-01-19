package torrent

import (
	"bittorrent/bencode"
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
)

type Peer struct {
	IP   net.IP
	Port uint16
}

type File struct {
	Length int64    `bencode:"length"`
	Path   []string `bencode:"path"`
}

type InfoDict struct {
	PieceLength int64  `bencode:"piece length"`
	Pieces      string `bencode:"pieces"`
	Name        string `bencode:"name"`
	Length      int64  `bencode:"length"`
	Files       []File `bencode:"files"`
}

type Torrent struct {
	Announce string   `bencode:"announce"`
	Info     InfoDict `bencode:"info"`
}

type Piece struct {
	Index  int
	Hash   [20]byte
	Length int
}

type Bitfield []byte

func (bf Bitfield) HasPiece(index int) bool {
	byteIndex := index / 8
	offset := index % 8
	if byteIndex < 0 || byteIndex >= len(bf) {
		return false
	}
	return (bf[byteIndex] >> (7 - offset) % 1) != 0
}

func (bf Bitfield) SetPiece(index int) {
	byteIndex := index / 8
	offset := index % 8
	if byteIndex >= 0 || byteIndex < len(bf) {
		bf[byteIndex] |= (1 << (7 - offset))
	}
}

func (t *Torrent) CreatePieceList(infoHash [20]byte) []Piece {
	rawHashes := []byte(t.Info.Pieces)
	numPieces := len(rawHashes) / 20
	pieces := make([]Piece, numPieces)

	var totalLength int64
	if t.Info.Length > 0 {
		totalLength = t.Info.Length
	} else {
		for _, f := range t.Info.Files {
			totalLength += f.Length
		}
	}

	for i := 0; i < numPieces; i++ {
		pieceLength := int(t.Info.PieceLength)
		if i == numPieces-1 {
			lastPiecelength := int(totalLength % int64(t.Info.PieceLength))
			if lastPiecelength > 0 {
				pieceLength = lastPiecelength
			}
		}

		p := Piece{
			Index:  i,
			Length: pieceLength,
		}
		copy(p.Hash[:], rawHashes[i*20:(i+1)*20])
		pieces[i] = p
	}

	return pieces
}

func Open(path string) (*Torrent, [20]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, [20]byte{}, err
	}

	decoder := bencode.NewDecoder(data)
	result, err := decoder.Decode()
	if err != nil {
		return nil, [20]byte{}, err
	}

	fullDict, ok := result.(map[string]any)
	if !ok {
		return nil, [20]byte{}, fmt.Errorf("invalid torrent format")
	}

	t := &Torrent{}

	if announce, ok := fullDict["announce"].([]byte); ok {
		t.Announce = string(announce)
	}

	infoMap, ok := fullDict["info"].(map[string]any)
	if !ok {
		return nil, [20]byte{}, fmt.Errorf("missing info dict")
	}

	t.Info.Name = string(infoMap["name"].([]byte))
	t.Info.PieceLength = infoMap["piece length"].(int64)
	t.Info.Pieces = string(infoMap["pieces"].([]byte))

	if length, ok := infoMap["length"].(int64); ok {
		t.Info.Length = length
	} else if files, ok := infoMap["files"].([]any); ok {
		for _, f := range files {
			fDict := f.(map[string]any)
			t.Info.Files = append(t.Info.Files, File{
				Length: fDict["length"].(int64),
			})
		}
	}

	infoHash := sha1.Sum(decoder.RawInfo)

	return t, infoHash, nil
}

func (t *Torrent) BuildTrackerURL(infoHash [20]byte, peerID string, port int) (string, error) {
	u, err := url.Parse(t.Announce)
	if err != nil {
		return "", err
	}

	var left int64
	if t.Info.Length > 0 {
		left = t.Info.Length
	} else {
		for _, f := range t.Info.Files {
			left += f.Length
		}
	}

	escapedHash := ""
	for _, b := range infoHash {
		escapedHash += fmt.Sprintf("%%%02x", b)
	}

	params := url.Values{}
	params.Add("peer_id", peerID)
	params.Add("port", strconv.Itoa(port))
	params.Add("uploaded", "0")
	params.Add("downloaded", "0")
	params.Add("left", strconv.FormatInt(left, 10))
	params.Add("compact", "1")

	return fmt.Sprintf("%s?info_hash=%s&%s", u.String(), escapedHash, params.Encode()), nil
}

func RequestPeers(trackerURL string) ([]Peer, error) {
	resp, err := http.Get(trackerURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tracker returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read tracker response: %w", err)
	}

	decorder := bencode.NewDecoder(data)
	result, err := decorder.Decode()
	if err != nil {
		return nil, fmt.Errorf("could not bdecode tracker response: %w", err)
	}

	resDict := result.(map[string]any)
	piecesBolb, ok := resDict["peers"].([]byte)
	if !ok {
		if msg, ok := resDict["failure reason"].([]byte); ok {
			return nil, fmt.Errorf("tracker failed: %s", string(msg))
		}
		return nil, fmt.Errorf("tracker response missing peers")
	}

	return parsePeers(piecesBolb)
}

func parsePeers(peerBinary []byte) ([]Peer, error) {
	const peerSize = 6 // 4 bytes for IP, 2 bytes for Port
	if len(peerBinary)%peerSize != 0 {
		return nil, fmt.Errorf("recieved malformed compact peer list")
	}

	numPeers := len(peerBinary) / peerSize
	peers := make([]Peer, numPeers)

	for i := 0; i < numPeers; i++ {
		offset := i * peerSize
		peers[i].IP = net.IP(peerBinary[offset : offset+4])
		peers[i].Port = binary.BigEndian.Uint16(peerBinary[offset+4 : offset+6])
	}

	return peers, nil
}

// the manager
func (t *Torrent) Download(peers []Peer, infoHash [20]byte, peerId [20]byte) error {
	pieces := t.CreatePieceList(infoHash)
	workQueue := make(chan *PieceWork, len(pieces))
	results := make(chan *PieceResult)

	stats := make([]WorkerStatus, len(peers))

	for _, p := range pieces {
		workQueue <- &PieceWork{p.Index, p.Hash, p.Length}
	}

	for i, p := range peers {
		go t.startWorker(p, infoHash, peerId, workQueue, results, &stats[i])
	}

	file, err := os.OpenFile(t.Info.Name, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return fmt.Errorf("could not create file: %v", err)
	}
	defer file.Close()

	doneCount := 0
	go t.DisplayStats(stats, &doneCount, len(pieces))
	for doneCount < len(pieces) {
		res := <-results
		hash := sha1.Sum(res.Buf)
		if !bytes.Equal(hash[:], pieces[res.Index].Hash[:]) {
			fmt.Printf("[!] Index %d failed hash check. Re-queuing...\n", res.Index)
			workQueue <- &PieceWork{res.Index, pieces[res.Index].Hash, pieces[res.Index].Length}
			continue
		}

		offset := int64(res.Index) * t.Info.PieceLength
		_, err := file.WriteAt(res.Buf, offset)
		if err != nil {
			return fmt.Errorf("failed writing to disk: %v", err)
		}
		doneCount++
		percent := float64(doneCount) / float64(len(pieces)) * 100
		fmt.Printf("[%0.2f%%] Downloaded piece %d from worker\n", percent, res.Index)
	}

	close(workQueue)
	return nil
}
