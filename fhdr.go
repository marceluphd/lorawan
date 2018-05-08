package lorawan

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

// DevAddr represents the device address.
type DevAddr [4]byte

// NetIDType returns the NetID type of the DevAddr.
func (a DevAddr) NetIDType() int {
	for i := 0; i < 8; i++ {
		if a[0]&(0xff<<(byte(7-i))) == 0xff&(0xff<<(byte(8-i))) {
			return i
		}
	}
	panic("NetIDType bug!")
}

// NwkID returns the NwkID bits of the DevAddr.
func (a DevAddr) NwkID() []byte {
	switch a.NetIDType() {
	case 0:
		return a.getNwkID(1, 6)
	case 1:
		return a.getNwkID(2, 6)
	case 2:
		return a.getNwkID(3, 9)
	case 3:
		return a.getNwkID(4, 10)
	case 4:
		return a.getNwkID(5, 11)
	case 5:
		return a.getNwkID(6, 13)
	case 6:
		return a.getNwkID(7, 15)
	case 7:
		return a.getNwkID(8, 17)
	default:
		return nil
	}
}

func (a DevAddr) getNwkID(prefixLength, nwkIDBits int) []byte {
	// convert DevAddr to uint32
	temp := binary.BigEndian.Uint32(a[:])
	fmt.Println("devaddr", temp)

	// clear prefix
	temp = temp << uint32(prefixLength)
	fmt.Println("no prefix", temp)

	// shift to starting of NwkID
	temp = temp >> (32 - uint32(nwkIDBits))
	fmt.Println("beginning nwkid", temp)

	// back to bytes
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, temp)

	bLen := nwkIDBits / 8
	if nwkIDBits%8 != 0 {
		bLen++
	}

	return out[len(out)-bLen:]
}

// MarshalBinary marshals the object in binary form.
func (a DevAddr) MarshalBinary() ([]byte, error) {
	out := make([]byte, len(a))
	for i, v := range a {
		// little endian
		out[len(a)-i-1] = v
	}
	return out, nil
}

// UnmarshalBinary decodes the object from binary form.
func (a *DevAddr) UnmarshalBinary(data []byte) error {
	if len(data) != len(a) {
		return fmt.Errorf("lorawan: %d bytes of data are expected", len(a))
	}
	for i, v := range data {
		// little endian
		a[len(a)-i-1] = v
	}
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (a DevAddr) MarshalText() ([]byte, error) {
	return []byte(a.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (a *DevAddr) UnmarshalText(text []byte) error {
	b, err := hex.DecodeString(string(text))
	if err != nil {
		return err
	}

	if len(b) != len(a) {
		return fmt.Errorf("lorawan: exactly %d bytes are expected", len(a))
	}
	copy(a[:], b)
	return nil
}

// String implements fmt.Stringer.
func (a DevAddr) String() string {
	return hex.EncodeToString(a[:])
}

// Scan implements sql.Scanner.
func (a *DevAddr) Scan(src interface{}) error {
	b, ok := src.([]byte)
	if !ok {
		return errors.New("lorawan: []byte type expected")
	}
	if len(b) != len(a) {
		return fmt.Errorf("lorawan []byte must have length %d", len(a))
	}
	copy(a[:], b)
	return nil
}

// FCtrl represents the FCtrl (frame control) field.
// Please note that the FPending and ClassB are mapped to the same bit. This
// means that when unmarshaling from a byte-slice, both fields will contain
// the same value (either true or false).
type FCtrl struct {
	ADR       bool  `json:"adr"`
	ADRACKReq bool  `json:"adrAckReq"`
	ACK       bool  `json:"ack"`
	FPending  bool  `json:"fPending"` // only used for downlink messages
	ClassB    bool  `json:"classB"`   // only used for uplink messages
	fOptsLen  uint8 // will be set automatically by the FHDR when serialized to []byte
}

// MarshalBinary marshals the object in binary form.
func (c FCtrl) MarshalBinary() ([]byte, error) {
	if c.fOptsLen > 15 {
		return []byte{}, errors.New("lorawan: max value of FOptsLen is 15")
	}
	b := byte(c.fOptsLen)
	if c.FPending || c.ClassB {
		b = b ^ (1 << 4)
	}
	if c.ACK {
		b = b ^ (1 << 5)
	}
	if c.ADRACKReq {
		b = b ^ (1 << 6)
	}
	if c.ADR {
		b = b ^ (1 << 7)
	}
	return []byte{b}, nil
}

// UnmarshalBinary decodes the object from binary form.
func (c *FCtrl) UnmarshalBinary(data []byte) error {
	if len(data) != 1 {
		return errors.New("lorawan: 1 byte of data is expected")
	}
	c.fOptsLen = data[0] & ((1 << 3) ^ (1 << 2) ^ (1 << 1) ^ (1 << 0))
	c.FPending = data[0]&(1<<4) != 0
	c.ClassB = data[0]&(1<<4) != 0
	c.ACK = data[0]&(1<<5) != 0
	c.ADRACKReq = data[0]&(1<<6) != 0
	c.ADR = data[0]&(1<<7) != 0
	return nil
}

// FHDR represents the frame header.
type FHDR struct {
	DevAddr DevAddr   `json:"devAddr"`
	FCtrl   FCtrl     `json:"fCtrl"`
	FCnt    uint32    `json:"fCnt"`  // only the least-significant 16 bits will be marshalled
	FOpts   []Payload `json:"fOpts"` // max. number of allowed bytes is 15
}

// MarshalBinary marshals the object in binary form.
func (h FHDR) MarshalBinary() ([]byte, error) {
	var b []byte
	var err error
	var opts []byte

	for _, mac := range h.FOpts {
		b, err = mac.MarshalBinary()
		if err != nil {
			return []byte{}, err
		}
		opts = append(opts, b...)
	}
	h.FCtrl.fOptsLen = uint8(len(opts))
	if h.FCtrl.fOptsLen > 15 {
		return []byte{}, errors.New("lorawan: max number of FOpts bytes is 15")
	}

	out := make([]byte, 0, 7+h.FCtrl.fOptsLen)
	b, err = h.DevAddr.MarshalBinary()
	if err != nil {
		return []byte{}, err
	}
	out = append(out, b...)

	b, err = h.FCtrl.MarshalBinary()
	if err != nil {
		return []byte{}, err
	}
	out = append(out, b...)
	fCntBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(fCntBytes, h.FCnt)
	out = append(out, fCntBytes[0:2]...)
	out = append(out, opts...)

	return out, nil
}

// UnmarshalBinary decodes the object from binary form.
func (h *FHDR) UnmarshalBinary(uplink bool, data []byte) error {
	if len(data) < 7 {
		return errors.New("lorawan: at least 7 bytes are expected")
	}

	if err := h.DevAddr.UnmarshalBinary(data[0:4]); err != nil {
		return err
	}
	if err := h.FCtrl.UnmarshalBinary(data[4:5]); err != nil {
		return err
	}
	fCntBytes := make([]byte, 4)
	copy(fCntBytes, data[5:7])
	h.FCnt = binary.LittleEndian.Uint32(fCntBytes)

	if len(data) > 7 {
		h.FOpts = []Payload{
			&DataPayload{Bytes: data[7:]},
		}
	}

	return nil
}
