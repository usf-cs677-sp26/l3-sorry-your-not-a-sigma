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

	"github.com/shirou/gopsutil/v4/disk"
)

func handleStorage(msgHandler *messages.MessageHandler, request *messages.StorageRequest) {
	log.Println("Attempting to store", request.FileName)
	// Make sure the file doesn’t already exist (refuse to overwrite existing files)
	file, err := os.OpenFile(request.FileName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
	if err != nil {
		msgHandler.SendResponse(false, err.Error())
		msgHandler.Close()
		return
	}

	// Ensure there is enough space available on the disk
	// Using disk from https://github.com/shirou/gopsutil/blob/v3.21.11/disk/disk.go#L12
	usage, err := disk.Usage(".")
	if err != nil {
		msgHandler.SendResponse(false, err.Error())
		msgHandler.Close()
		return
	}

	msgSize := request.GetSize()
	usageSize := usage.Free
	if msgSize > usageSize {
		errMsg := fmt.Sprintf(
			"No disk space. Message size: %d, Available space: %d, Exceeds by: %d",
			msgSize, usageSize, msgSize-usageSize,
		)
		msgHandler.SendResponse(false, errMsg)
		msgHandler.Close()
		return
	}

	// Send an “OK” response to the client so it knows it can begin sending the file
	msgHandler.SendResponse(true, "Ready for data")

	// Receive data stream and store the file
	md5 := md5.New()
	w := io.MultiWriter(file, md5)
	io.CopyN(w, msgHandler, int64(request.Size)) /* Write and checksum as we go */
	file.Close()

	serverCheck := md5.Sum(nil)

	// Verify its checksum against the checksum sent by the client
	clientCheckMsg, _ := msgHandler.Receive()
	clientCheck := clientCheckMsg.GetChecksum().Checksum

	// Respond to the client with the status of the transfer (success or failure)
	if util.VerifyChecksum(serverCheck, clientCheck) {
		log.Println("Successfully stored file.")
		msgHandler.SendResponse(true, "File transfer completed and verified")
	} else {
		log.Println("FAILED to store file. Invalid checksum.")
		msgHandler.SendResponse(false, "Checksum mismatch")
	}

	// Disconnect the client
	msgHandler.Close()
}

func handleRetrieval(msgHandler *messages.MessageHandler, request *messages.RetrievalRequest) {
	log.Println("Attempting to retrieve", request.FileName)

	// Ensure the file requested actually exists
	// Get file size and make sure it exists
	info, err := os.Stat(request.FileName)
	if err != nil {
		log.Fatalln(err)
	}

	// Send a response back to the client with the file’s size and checksum (or indicate failure if it doesn’t exit)
	msgHandler.SendRetrievalResponse(true, "Ready to send", uint64(info.Size()))

	// Begin streaming file to the client
	file, _ := os.Open(request.FileName)
	md5 := md5.New()
	w := io.MultiWriter(msgHandler, md5)
	io.CopyN(w, file, info.Size()) // Checksum and transfer file at same time
	file.Close()

	checksum := md5.Sum(nil)
	msgHandler.SendChecksumVerification(checksum)

	// Disconnect the client when the transfer is complete
	msgHandler.Close()
}

func handleClient(msgHandler *messages.MessageHandler) {
	defer msgHandler.Close()

	for {
		wrapper, err := msgHandler.Receive()
		if err != nil {
			log.Println(err)
		}

		switch msg := wrapper.Msg.(type) {
		case *messages.Wrapper_StorageReq:
			handleStorage(msgHandler, msg.StorageReq)
			continue
		case *messages.Wrapper_RetrievalReq:
			handleRetrieval(msgHandler, msg.RetrievalReq)
			continue
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
	if err := os.Chdir(dir); err != nil {
		log.Fatalln(err)
	}

	fmt.Println("Listening on port:", port)
	fmt.Println("Download directory:", dir)
	for {
		if conn, err := listener.Accept(); err == nil {
			log.Println("Accepted connection", conn.RemoteAddr())
			handler := messages.NewMessageHandler(conn)
			go handleClient(handler)
		}
	}
}
