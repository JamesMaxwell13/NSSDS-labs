package udp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

const (
	Port          = 8000
	BufferSize    = 1024 * 64
	MaxPacketSize = 1024 * 128
	ChunkSize     = 8192
	AckTimeout    = 2 * time.Second
	WindowSize    = 5
	MaxRetries    = 5
)

var Logger *log.Logger

func init() {
	Logger = log.New(os.Stdout, "[UDP] ", log.LstdFlags|log.Lmicroseconds)
}

func BuildPacket(seq uint32, data []byte) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, seq)
	buf.Write(data)
	return buf.Bytes()
}

func ParsePacket(packet []byte) (uint32, []byte) {
	if len(packet) < 4 {
		return 0, nil
	}
	seq := binary.BigEndian.Uint32(packet[:4])
	return seq, packet[4:]
}

func SendCommandWithResponse(conn *net.UDPConn, addr *net.UDPAddr, cmd string, timeout time.Duration) (string, error) {
	buffer := make([]byte, MaxPacketSize)

	for i := 0; i < MaxRetries; i++ {
		Logger.Printf("Sending command: %q (attempt %d)", cmd, i+1)
		if _, err := conn.WriteToUDP([]byte(cmd), addr); err != nil {
			Logger.Printf("Command send error: %v", err)
			continue
		}

		conn.SetReadDeadline(time.Now().Add(timeout))
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			Logger.Printf("Command response error: %v", err)
			continue
		}

		response := string(buffer[:n])
		Logger.Printf("Received response: %q", response)
		return response, nil
	}

	return "", fmt.Errorf("max retries (%d) exceeded for command %q", MaxRetries, cmd)
}

func Upload(filePath string, conn *net.UDPConn, addr *net.UDPAddr) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("error opening file: %v", err)
	}
	defer file.Close()

	fileInfo, _ := file.Stat()
	totalSize := fileInfo.Size()
	buffer := make([]byte, ChunkSize)
	seq := uint32(0)
	var sent int64

	for {
		n, err := file.Read(buffer)
		if err != nil {
			break
		}

		packet := BuildPacket(seq, buffer[:n])
	retry:
		for i := 0; i < MaxRetries; i++ {
			Logger.Printf("Sending packet %d (%d bytes)", seq, n)
			if _, err := conn.WriteToUDP(packet, addr); err != nil {
				Logger.Printf("Error sending packet %d: %v", seq, err)
				continue
			}

			conn.SetReadDeadline(time.Now().Add(AckTimeout))
			ackBuf := make([]byte, 4)
			n, _, err := conn.ReadFromUDP(ackBuf)
			if err != nil {
				Logger.Printf("Ack timeout for packet %d, retrying...", seq)
				continue
			}

			if n >= 4 {
				ackSeq := binary.BigEndian.Uint32(ackBuf[:4])
				if ackSeq == seq {
					sent += int64(n)
					printProgress(sent, totalSize)
					seq++
					break retry
				}
			}
		}
	}

	// Send EOF
	eof := BuildPacket(seq, []byte("EOF"))
	Logger.Printf("Sending EOF packet")
	conn.WriteToUDP(eof, addr)

	fmt.Println("\nUpload complete")
	return nil
}

func Download(savePath string, conn *net.UDPConn, addr *net.UDPAddr) error {
	file, err := os.Create(savePath)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer file.Close()

	buffer := make([]byte, ChunkSize+4)
	expectedSeq := uint32(0)
	var received int64

	for {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, remote, err := conn.ReadFromUDP(buffer)
		if err != nil {
			return fmt.Errorf("read timeout: %v", err)
		}

		if remote.String() != addr.String() {
			continue
		}

		seq, data := ParsePacket(buffer[:n])
		if seq == 0 && data == nil {
			continue
		}

		Logger.Printf("Received packet %d (%d bytes)", seq, len(data))

		if string(data) == "EOF" {
			ack := make([]byte, 4)
			binary.BigEndian.PutUint32(ack, seq)
			conn.WriteToUDP(ack, addr)
			break
		}

		if seq == expectedSeq {
			if _, err := file.Write(data); err != nil {
				Logger.Printf("Error writing packet %d: %v", seq, err)
				continue
			}
			received += int64(len(data))
			printProgress(received, 0)
			expectedSeq++

			ack := make([]byte, 4)
			binary.BigEndian.PutUint32(ack, seq)
			Logger.Printf("Sending ACK for packet %d", seq)
			conn.WriteToUDP(ack, addr)
		} else if seq < expectedSeq {
			ack := make([]byte, 4)
			binary.BigEndian.PutUint32(ack, seq)
			Logger.Printf("Re-sending ACK for old packet %d", seq)
			conn.WriteToUDP(ack, addr)
		}
	}

	Logger.Printf("Download completed (%d bytes)", received)
	fmt.Println("\nDownload complete")
	return nil
}

func printProgress(current, total int64) {
	var percent float64
	if total > 0 {
		percent = float64(current) / float64(total) * 100
	}
	barWidth := 50
	filled := int(percent / 100 * float64(barWidth))
	fmt.Printf("\r[%s%s] %.2f%% (%d KB)",
		strings.Repeat("=", filled),
		strings.Repeat(" ", barWidth-filled),
		percent,
		current/1024,
	)
}
