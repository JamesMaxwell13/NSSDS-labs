package server

import (
	"fmt"
	"lab_2/udp"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Server struct {
	Conn       *net.UDPConn
	ClientAddr *net.UDPAddr
	CurrentDir string
}

func (s *Server) RunServer() {
	s.CurrentDir, _ = os.Getwd()

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", udp.Port))
	if err != nil {
		fmt.Printf("Error resolving address: %v\n", err)
		return
	}

	s.Conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Printf("Error starting server: %v\n", err)
		return
	}
	defer s.Conn.Close()

	fmt.Printf("Server started on port %d\n", udp.Port)
	s.handleRequests()
}

func (s *Server) handleRequests() {
	buffer := make([]byte, udp.MaxPacketSize)

	for {
		n, clientAddr, err := s.Conn.ReadFromUDP(buffer)
		if err != nil {
			fmt.Printf("Error reading from UDP: %v\n", err)
			continue
		}

		command := string(buffer[:n])
		parts := strings.Fields(command)
		if len(parts) == 0 {
			continue
		}

		s.ClientAddr = clientAddr
		fmt.Printf("[%s] Command: %s\n", clientAddr.String(), command)

		response := s.processCommand(parts)
		if response != "" {
			if _, err := s.Conn.WriteToUDP([]byte(response), clientAddr); err != nil {
				fmt.Printf("Error sending response: %v\n", err)
			}
		}
	}
}

func (s *Server) processCommand(parts []string) string {
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "echo":
		return strings.Join(args, " ")
	case "time":
		return time.Now().Format("15:04:05.000")
	case "quit", "exit", "close":
		return "goodbye!"
	case "ls":
		return s.listDirectory()
	case "cd":
		return s.changeDirectory(args...)
	case "upload":
		go s.handleUpload(args...)
		return "ready"
	case "download":
		go s.handleDownload(args...)
		return "ready"
	default:
		return "error: unknown command"
	}
}

func (s *Server) listDirectory() string {
	files, err := os.ReadDir(s.CurrentDir)
	if err != nil {
		return fmt.Sprintf("Error reading directory: %v", err)
	}

	var result []string
	for _, file := range files {
		info, _ := file.Info()
		result = append(result, fmt.Sprintf("%s [%s, %d bytes]",
			file.Name(),
			file.Type().String(),
			info.Size()))
	}
	return strings.Join(result, "\n")
}

func (s *Server) changeDirectory(args ...string) string {
	if len(args) == 0 {
		return "error: path required"
	}

	newPath := filepath.Join(s.CurrentDir, args[0])
	absPath, err := filepath.Abs(newPath)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return fmt.Sprintf("error: %s is not a valid directory", absPath)
	}

	s.CurrentDir = absPath
	return fmt.Sprintf("Changed directory to %s", absPath)
}

func (s *Server) handleDownload(args ...string) {
	if len(args) == 0 {
		_, _ = s.Conn.WriteToUDP([]byte("error: filename required"), s.ClientAddr)
		return
	}

	fileName := args[0]
	filePath := filepath.Join(s.CurrentDir, fileName)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		_, _ = s.Conn.WriteToUDP([]byte("error: file not found"), s.ClientAddr)
		return
	}

	if err := udp.Upload(filePath, s.Conn, s.ClientAddr); err != nil {
		_, _ = s.Conn.WriteToUDP([]byte("error: download failed"), s.ClientAddr)
		return
	}

	_, _ = s.Conn.WriteToUDP([]byte("download complete"), s.ClientAddr)
}

func (s *Server) handleUpload(args ...string) {
	if len(args) == 0 {
		_, _ = s.Conn.WriteToUDP([]byte("error: filename required"), s.ClientAddr)
		return
	}

	fileName := args[0]
	filePath := filepath.Join(s.CurrentDir, fileName)

	if _, err := os.Stat(filePath); err == nil {
		_, _ = s.Conn.WriteToUDP([]byte("error: file exists"), s.ClientAddr)
		return
	}

	if err := udp.Download(filePath, s.Conn, s.ClientAddr); err != nil {
		_, _ = s.Conn.WriteToUDP([]byte("error: upload failed"), s.ClientAddr)
		return
	}

	_, _ = s.Conn.WriteToUDP([]byte("upload complete"), s.ClientAddr)
}
