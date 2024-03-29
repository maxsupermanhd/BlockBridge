package main

import (
	"encoding/hex"
	"log"
	"runtime/debug"
)

func must(err error) {
	if err != nil {
		debug.PrintStack()
		log.Fatal(err)
	}
}

func noerr[T any](ret T, err error) T {
	must(err)
	return ret
}

func uuidToString(uuid [16]byte) string {
	var buf [36]byte
	encodeUUIDToHex(buf[:], uuid)
	return string(buf[:])
}

func encodeUUIDToHex(dst []byte, uuid [16]byte) {
	hex.Encode(dst[:], uuid[:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], uuid[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], uuid[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], uuid[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:], uuid[10:])
}
