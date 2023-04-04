package utils

import (
	"encoding/binary"
	"strconv"
)

func Uint64ToBytes(num uint64) []byte {
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, num)
	return bytes
}

func StringToUint64(s string) (uint64, error) {
	num, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return num, nil
}

func StringUint64ToBytes(s string) ([]byte, error) {
	num, err := StringToUint64(s)
	if err != nil {
		return nil, err
	}
	return Uint64ToBytes(num), nil
}
