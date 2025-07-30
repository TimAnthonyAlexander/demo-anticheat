package demo

import (
	"encoding/binary"
	"fmt"
	"strings"
)

var alpha = "ABCDEFGHJKLMNOPQRSTUVWXYZabcdefhijkmnopqrstuvwxyz23456789"

// Decode converts a CS2 share code to match ID, outcome ID, and token
func Decode(code string) (match, outcome uint64, token uint16) {
	if strings.HasPrefix(code, "CSGO-") {
		code = code[5:]
	}
	code = strings.ReplaceAll(code, "-", "")

	buf := make([]byte, 18)
	for i := len(code) - 1; i >= 0; i-- {
		carry := uint32(strings.IndexByte(alpha, code[i]))
		for j := 0; j < 18; j++ {
			carry += uint32(buf[j]) * 57
			buf[j] = byte(carry)
			carry >>= 8
		}
	}
	token = uint16(buf[0]) | uint16(buf[1])<<8
	outcome = binary.BigEndian.Uint64(buf[2:10])
	match = binary.BigEndian.Uint64(buf[10:18])
	return
}

// ReplayURL converts a CS2 share code to a download URL for the demo
func ReplayURL(code string) string {
	m, o, _ := Decode(code)
	host := 128 + int(o>>8&0xFF)
	return fmt.Sprintf("http://replay%d.valve.net/730/%021d_%d.dem.bz2", host, m, o)
}
