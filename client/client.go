package main

import (
	"crypto/md5"
	"file-transfer/messages"
	"file-transfer/util"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func put(msgHandler *messages.MessageHandler, fileName string) int {
	fmt.Println("PUT", fileName)

	// Get file size and make sure it exists
	info, err := os.Stat(fileName)
	if err != nil {
		log.Println("error when getting file size:")
		log.Fatalln(err)
	}

	// Tell the server we want to store this file
	log.Printf("Sending storage request for %s\n", fileName)
	msgHandler.SendStorageRequest(fileName, uint64(info.Size()))
	if ok, _ := msgHandler.ReceiveResponse(); !ok {
		return 1
	}

	log.Println("Server OKd send request")

	file, _ := os.Open(fileName)
	// md5 is a hash.Hash that has an embedded io.Writer
	md5 := md5.New()
	w := io.MultiWriter(msgHandler, md5)
	io.CopyN(w, file, info.Size()) // Checksum and transfer file at same time
	file.Close()

	log.Println("file closed")
	checksum := md5.Sum(nil)
	msgHandler.SendChecksumVerification(checksum)
	if ok, err := msgHandler.ReceiveResponse(); !ok {
		log.Fatalln(err)
		return 1
	}

	fmt.Println("Storage complete!")
	return 0
}

func get(msgHandler *messages.MessageHandler, fileName string) int {
	fmt.Println("GET", fileName)

	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
	if err != nil {
		return 1
	}

	msgHandler.SendRetrievalRequest(fileName)
	ok, _, size := msgHandler.ReceiveRetrievalResponse()
	if !ok {
		return 1
	}

	md5 := md5.New()
	w := io.MultiWriter(file, md5)
	io.CopyN(w, msgHandler, int64(size))
	file.Close()

	clientCheck := md5.Sum(nil)
	checkMsg, _ := msgHandler.Receive()
	serverCheck := checkMsg.GetChecksum().Checksum

	if util.VerifyChecksum(serverCheck, clientCheck) {
		log.Println("Successfully retrieved file.")
	} else {
		log.Println("FAILED to retrieve file. Invalid checksum.")
	}

	return 0
}

func main() {
	if len(os.Args) < 4 {
		fmt.Printf("Not enough arguments. Usage: %s server:port put|get file-name [download-dir]\n", os.Args[0])
		os.Exit(1)
	}

	host := os.Args[1]
	conn, err := net.Dial("tcp", host)
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	msgHandler := messages.NewMessageHandler(conn)
	defer conn.Close()

	action := strings.ToLower(os.Args[2])
	if action != "put" && action != "get" {
		log.Fatalln("Invalid action", action)
	}

	fileName := os.Args[3]

	dir := "."
	if len(os.Args) >= 5 {
		dir = os.Args[4]
	}
	openDir, err := os.Open(dir)
	if err != nil {
		log.Fatalln(err)
	}
	openDir.Close()

	if action == "put" {
		log.Println("putting")
		os.Exit(put(msgHandler, fileName))
	} else if action == "get" {
		log.Println("getting")
		os.Exit(get(msgHandler, filepath.Join(dir, fileName)))
	}
}
