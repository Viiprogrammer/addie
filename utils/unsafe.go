package utils

import "unsafe"

// Fully valid operation
func BytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// Read-only access because of not inited "capacity"
func StringToBytes(s *string) []byte {
	return *(*[]byte)(unsafe.Pointer(s))
}
