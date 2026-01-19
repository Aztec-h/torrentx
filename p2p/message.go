package p2p

import (
	"encoding/binary"
	"io"
)

// Message IDs from the BitTorrent spec
const (
	MsgChoke         uint8 = 0
	MsgUnchoke       uint8 = 1
	MsgInterested    uint8 = 2
	MsgNotInterested uint8 = 3
	MsgHave          uint8 = 4
	MsgBitfield      uint8 = 5
	MsgRequest       uint8 = 6
	MsgPiece         uint8 = 7
	MsgCancel        uint8 = 8
)

// Message represents a BitTorrent message
type Message struct {
	ID      uint8
	Payload []byte
}

type PieceProgress struct {
	Index      int
	Buf        []byte
	Downloaded int
}

// Serialize serializes a message into a bitstream
// Format: <length prefix><message ID><payload>
func (m *Message) Serialize() []byte {
	if m == nil {
		return make([]byte, 4) // Keep-alive message
	}
	length := uint32(len(m.Payload) + 1) // +1 for the ID
	buf := make([]byte, 4+length)
	binary.BigEndian.PutUint32(buf[0:4], length)
	buf[4] = m.ID
	copy(buf[5:], m.Payload)
	return buf
}

func Read(r io.Reader) (*Message, error) {
	lengthBuf := make([]byte, 4)
	_, err := io.ReadFull(r, lengthBuf)
	if err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBuf)

	if length == 0 {
		return nil, nil
	}

	messageBuf := make([]byte, length)
	_, err = io.ReadFull(r, messageBuf)
	if err != nil {
		return nil, err
	}

	return &Message{
		ID:      uint8(messageBuf[0]),
		Payload: messageBuf[1:],
	}, nil
}

func FormatRequest(index, begin, length int) *Message {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], uint32(index))   // Which piece
	binary.BigEndian.PutUint32(payload[4:8], uint32(begin))   // Offset within piece
	binary.BigEndian.PutUint32(payload[8:12], uint32(length)) // Block size (16384)
	return &Message{ID: MsgRequest, Payload: payload}
}

// block buffer or in memory progress
// allocating memory for PieceProgress
func NewPieceProgress(index int, length int) *PieceProgress {
	return &PieceProgress{
		Index: index,
		Buf:   make([]byte, length),
	}
}

func (pp *PieceProgress) AddBlock(begin int, block []byte) {
	copy(pp.Buf[begin:], block)
	pp.Downloaded += len(block)
}

func (pp *PieceProgress) IsComplete() bool {
	return pp.Downloaded == len(pp.Buf)
}
