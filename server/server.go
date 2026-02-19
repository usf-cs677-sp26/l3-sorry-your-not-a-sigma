package main

import (
	"crypto/md5"
	"file-transfer/messages"
	"file-transfer/util"
	"fmt"
	"github.com/shirou/gopsutil/v4/disk"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
)

func handleStorage(msgHandler *messages.MessageHandler, request *messages.StorageRequest) {
	// this is required to flatten file hierarchy as Note 1 states there is no concept
	// of directories
	fileName := filepath.Base(request.FileName)
	log.Println("Attempting to store", fileName)
	// os.O_EXCL ensure no overwrite
	// SPEC: 1. Make sure the file doesn’t already exist (refuse to overwrite existing files)
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
	if err != nil {
		log.Println(err)
		msgHandler.SendResponse(false, err.Error())
		msgHandler.Close()
		return
	}

	// since we did chdir in main this should be fine
	usage, err := disk.Usage(".")
	if err != nil {
		log.Println(err)
		msgHandler.SendResponse(false, err.Error())
		msgHandler.Close()
		return
	}

	log.Printf("Free space: %d\n", usage.Free)
	log.Printf("File size: %d\n", request.Size)
	// SPEC: 2. Ensure there is enough space available on the disk
	if usage.Free <= request.Size {
		log.Println(err)
		msgHandler.SendResponse(false, "Server does not have enough disk space for file.")
		msgHandler.Close()
		return
	}

	// SPEC: 3. Send an “OK” response to the client so it knows it can begin sending the file
	msgHandler.SendResponse(true, "Ready for data")
	md5 := md5.New()
	// SPEC: 4. Receive data stream and store the file
	w := io.MultiWriter(file, md5)
	io.CopyN(w, msgHandler, int64(request.Size)) /* Write and checksum as we go */
	file.Close()

	log.Println("file closed")

	serverCheck := md5.Sum(nil)

	// SPEC: 5. Verify its checksum against the checksum sent by the client
	clientCheckMsg, _ := msgHandler.Receive()
	clientCheck := clientCheckMsg.GetChecksum().Checksum

	// no delete on checksum failure??
	// SPEC: 6. Respond to the client with the status of the transfer (success or failure)
	if util.VerifyChecksum(serverCheck, clientCheck) {
		log.Println("Successfully stored file.")
		msgHandler.SendResponse(true, "Successfully stored file.")
	} else {
		log.Println("FAILED to store file. Invalid checksum.")
		msgHandler.SendResponse(false, "FAILED to store file.")
	}

	// SPEC: 7. Disconnect the client
	msgHandler.Close()
}

func handleRetrieval(msgHandler *messages.MessageHandler, request *messages.RetrievalRequest) {
	log.Println("Attempting to retrieve", request.FileName)

	// Get file size and make sure it exists
	info, err := os.Stat(request.FileName)
	if err != nil {
		log.Fatalln(err)
	}

	msgHandler.SendRetrievalResponse(true, "Ready to send", uint64(info.Size()))

	file, _ := os.Open(request.FileName)
	md5 := md5.New()
	w := io.MultiWriter(msgHandler, md5)
	io.CopyN(w, file, info.Size()) // Checksum and transfer file at same time
	file.Close()

	checksum := md5.Sum(nil)
	msgHandler.SendChecksumVerification(checksum)
}

func handleClient(msgHandler *messages.MessageHandler) {
	defer msgHandler.Close()

	for {
		wrapper, err := msgHandler.Receive()
		if err != nil {
			log.Println(err)
		}

		// TODO close here or in handleStorage??
		// spec says "Disconnect the client" after succesful store and retrieve
		switch msg := wrapper.Msg.(type) {
		case *messages.Wrapper_StorageReq:
			handleStorage(msgHandler, msg.StorageReq)
			continue
		case *messages.Wrapper_RetrievalReq:
			handleRetrieval(msgHandler, msg.RetrievalReq)
			msgHandler.Close()
			return
		case nil:
			log.Println("Received an empty message, terminating client")
			return
		default:
			log.Printf("Unexpected message type: %T", msg)
		}
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Not enough arguments. Usage: %s port [download-dir]\n", os.Args[0])
		os.Exit(1)
	}

	port := os.Args[1]
	// SPEC: 2. Listen on the specified port for incoming client connections
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalln(err.Error())
		os.Exit(1)
	}
	defer listener.Close()

	dir := "."
	if len(os.Args) >= 3 {
		dir = os.Args[2]
	}

	// SPEC: 1. Start up and make sure storage directory exists
	if err := os.Chdir(dir); err != nil {
		log.Fatalln(err)
	}

	fmt.Println("Listening on port:", port)
	fmt.Println("Download directory:", dir)
	for {
		// Listen on the specified port for incoming client connections
		if conn, err := listener.Accept(); err == nil {
			log.Println("Accepted connection", conn.RemoteAddr())
			handler := messages.NewMessageHandler(conn)
			// SPEC: 3. Handle each request with a separate goroutine (to allow multiple client connections)
			go handleClient(handler)
		}
	}
}
