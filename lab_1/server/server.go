package server

import (
	"bufio"
	"fmt"
	"lab_1/tcp"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Server struct {
	Conn       net.Conn
	ClientAddr string
	ServerAddr string
	CurrentDir string
}

func (s *Server) RunServer() {
	address, err := tcp.GetIP()
	if err != nil {
		fmt.Printf("error getting IP address: %v\n", err)
		return
	}

	s.CurrentDir, _ = os.Getwd()
	s.ServerAddr = fmt.Sprintf("127.0.0.1:%d", tcp.Port)
	ln, err := net.Listen("tcp", s.ServerAddr)
	if err != nil {
		fmt.Printf("error starting server: %v\n", err)
		return
	}

	defer func(ln net.Listener) {
		_ = ln.Close()
	}(ln)
	fmt.Printf("server started on address %s and port %d\n", address, tcp.Port)

	for {
		s.Conn, err = ln.Accept()
		if err != nil {
			fmt.Printf("error accepting connection: %v\n", err)
			continue
		}

		if err := tcp.SetKeepalive(s.Conn); err != nil {
			fmt.Printf("error setting keepalive: %v\n", err)
			_ = s.Conn.Close()
			continue
		}
		s.HandleClient(s.Conn)
	}
}

func (s *Server) HandleClient(conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("recovered from panic in client handler: %v\n", r)
		}
		_ = conn.Close()
	}()
	s.ClientAddr = conn.RemoteAddr().String()
	fmt.Printf("new connection from %s\n", s.ClientAddr)

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		command := scanner.Text()
		parts := strings.Fields(command)
		if len(parts) == 0 {
			continue
		}
		fmt.Printf("[%s] command: %s\n", s.ClientAddr, command)
		response := s.ParseCommand(parts)
		if response != "" {
			if err := tcp.SendData(conn, response); err != nil {
				fmt.Printf("error sending response to %s: %v\n", s.ClientAddr, err)
				return
			}
			if response == "goodbye!" {
				return
			}
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("client %s disconnected with error: %v\n", s.ClientAddr, err)
	}
}

func (s *Server) ParseCommand(parts []string) string {
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	response := ""
	switch cmd {
	case "echo":
		response = handleEcho(args...)
	case "time":
		response = handleTime()
	case "quit", "exit", "close":
		response = "goodbye!"
	case "ls":
		response = handleLs(s.CurrentDir)
	case "cd":
		response = handleCd(&s.CurrentDir, args...)
	case "download":
		tcp.Upload(s.CurrentDir, s.Conn, args...)
	case "upload":
		tcp.Download(s.CurrentDir, s.Conn, args...)
	default:
		response = "error: unknown command"
	}
	return response
}

func handleEcho(args ...string) string {
	return strings.Join(args, " ")
}

func handleTime() string {
	return time.Now().Format("15:04:05.000")
}

func handleLs(dir string) string {
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Sprintf("error reading directory '%s': %v", dir, err)
	}

	var result []string
	for _, file := range files {
		fileType := "file"
		if file.IsDir() {
			fileType = "directory"
		}
		result = append(result, fmt.Sprintf("%s - [%s]", file.Name(), fileType))
	}

	if len(result) == 0 {
		return "directory is empty"
	}
	return strings.Join(result, "   ")
}

func handleCd(currentDir *string, args ...string) string {
	if len(args) == 0 {
		return "error: path required"
	}

	newPath := filepath.Join(*currentDir, args[0])
	absPath, err := filepath.Abs(newPath)
	if err != nil {
		return fmt.Sprintf("error getting absolute path: %v", err)
	}

	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return fmt.Sprintf("error: path does not exist or is not a directory: %s", absPath)
	}

	*currentDir = absPath
	return fmt.Sprintf("changed directory to %s", absPath)
}
