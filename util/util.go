package util

import (
	"log"
	"reflect"
)

func VerifyChecksum(serverCheck []byte, clientCheck []byte) bool {
	log.Printf("Server checksum: %x\n", serverCheck)
	log.Printf("Client checksum: %x\n", clientCheck)
	if reflect.DeepEqual(clientCheck, serverCheck) {
		log.Println("Checksums match")
		return true
	} else {
		log.Println("Checksums DO NOT match")
		return false
	}
}


