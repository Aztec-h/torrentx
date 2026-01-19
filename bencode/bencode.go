package bencode

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
)

const (
	MaxStringLength = 10 * 1024 * 1024
	MaxDepth        = 50
)

var (
	ErrTooDeep              = errors.New("bencode: exceeded max depth")
	ErrInvalidIntegerFormat = errors.New("bencode: invalid integer format")
	ErrLeadingZero          = errors.New("bencode: leading zeros not allowed")
	ErrNegativeZero         = errors.New("bencode: negative zero not allowed")
	ErrInvalidStringFormat  = errors.New("bencode: invalid string format")
	ErrTooShort             = errors.New("bencode: string shorter than specified length")
	ErrInvalidListFormat    = errors.New("bencode: invalid list format")
	ErrInvalidDictFormat    = errors.New("bencode: invalid dictionary format")
	ErrEmptyData            = errors.New("bencode: empty data")
)

type Decoder struct {
	data    []byte
	depth   int
	RawInfo []byte
}

func NewDecoder(data []byte) *Decoder {
	return &Decoder{data: data}
}

// Decode is the entry point
func (d *Decoder) Decode() (any, error) {
	val, _, err := d.parseValue(d.data)
	return val, err
}

func (d *Decoder) parseValue(data []byte) (any, int, error) {
	if len(data) == 0 {
		return nil, 0, ErrEmptyData
	}

	switch {
	case data[0] == 'i':
		return d.parseInteger(data)
	case data[0] >= '0' && data[0] <= '9':
		return d.parseString(data)
	case data[0] == 'l':
		return d.parseList(data)
	case data[0] == 'd':
		return d.parseDict(data)
	default:
		return nil, 0, fmt.Errorf("bencode: unknown type identifier '%c'", data[0])
	}
}

func (d *Decoder) parseInteger(data []byte) (int64, int, error) {
	if len(data) < 3 {
		return 0, 0, ErrInvalidIntegerFormat
	}

	endIdx := bytes.IndexByte(data, 'e')
	if endIdx == -1 {
		return 0, 0, ErrInvalidIntegerFormat
	}

	body := data[1:endIdx]
	if len(body) == 0 {
		return 0, 0, ErrInvalidIntegerFormat
	}

	// Validate leading zeros and negative zero
	if body[0] == '0' && len(body) > 1 {
		return 0, 0, ErrLeadingZero
	}
	if body[0] == '-' {
		if len(body) == 1 {
			return 0, 0, ErrInvalidIntegerFormat
		}
		if body[1] == '0' {
			if len(body) == 2 {
				return 0, 0, ErrNegativeZero
			}
			return 0, 0, ErrLeadingZero
		}
	}

	num, err := strconv.ParseInt(string(body), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("bencode: invalid integer: %w", err)
	}

	return num, endIdx + 1, nil
}

func (d *Decoder) parseString(data []byte) ([]byte, int, error) {
	colonIdx := bytes.IndexByte(data, ':')
	if colonIdx <= 0 {
		return nil, 0, ErrInvalidStringFormat
	}

	lenBuf := data[:colonIdx]
	// Leading zeros in string length are not allowed unless length is 0
	if lenBuf[0] == '0' && len(lenBuf) > 1 {
		return nil, 0, ErrLeadingZero
	}

	length, err := strconv.Atoi(string(lenBuf))
	if err != nil || length < 0 {
		return nil, 0, ErrInvalidStringFormat
	}

	if length > MaxStringLength {
		return nil, 0, errors.New("bencode: string length exceeds limit")
	}

	totalNeeded := colonIdx + 1 + length
	if len(data) < totalNeeded {
		return nil, 0, ErrTooShort
	}

	// Return a slice of the original data (Zero-Copy)
	return data[colonIdx+1 : totalNeeded], totalNeeded, nil
}

func (d *Decoder) parseList(data []byte) ([]any, int, error) {
	d.depth++
	defer func() { d.depth-- }()

	if d.depth > MaxDepth {
		return nil, 0, ErrTooDeep
	}

	if len(data) < 2 || data[0] != 'l' {
		return nil, 0, ErrInvalidListFormat
	}

	var result []any
	currentPosition := 1

	// Safety: Check length before checking the 'e' character
	for currentPosition < len(data) && data[currentPosition] != 'e' {
		val, consumed, err := d.parseValue(data[currentPosition:])
		if err != nil {
			return nil, 0, err
		}

		result = append(result, val)
		currentPosition += consumed
	}

	// If we exited the loop and didn't find 'e', the bencode is truncated
	if currentPosition >= len(data) || data[currentPosition] != 'e' {
		return nil, 0, ErrInvalidListFormat
	}

	return result, currentPosition + 1, nil
}

func (d *Decoder) parseDict(data []byte) (map[string]any, int, error) {
	d.depth++
	defer func() { d.depth-- }()

	if d.depth > MaxDepth {
		return nil, 0, ErrTooDeep
	}

	if len(data) < 2 || data[0] != 'd' {
		return nil, 0, ErrInvalidDictFormat
	}

	result := make(map[string]any)
	currentPosition := 1

	for currentPosition < len(data) && data[currentPosition] != 'e' {
		keyBytes, consumedKey, err := d.parseString(data[currentPosition:])
		if err != nil {
			return nil, 0, fmt.Errorf("bencode: invalid dict key: %w", err)
		}
		key := string(keyBytes)
		currentPosition += consumedKey

		// Check if there's data left for the value
		if currentPosition >= len(data) {
			return nil, 0, ErrInvalidDictFormat
		}

		val, consumedVal, err := d.parseValue(data[currentPosition:])
		if err != nil {
			return nil, 0, fmt.Errorf("bencode: invalid dict value for key %s: %w", key, err)
		}

		// trapping key
		if key == "info" && d.depth == 1 {
			d.RawInfo = data[currentPosition : currentPosition+consumedVal]
		}

		result[key] = val
		currentPosition += consumedVal
	}

	if currentPosition >= len(data) || data[currentPosition] != 'e' {
		return nil, 0, ErrInvalidDictFormat
	}

	return result, currentPosition + 1, nil
}
