package server

import (
	"fmt"
	"lab_4/tcp"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Server struct {
	Listener    net.Listener
	ServerAddr  string
	CurrentDir  string
	Clients     map[int]*ClientConn
	PollFds     []syscall.PollFd
	ClientCount int
}

type ClientConn struct {
	Fd         int
	Conn       net.Conn
	Addr       string
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
	s.Listener, err = net.Listen("tcp", s.ServerAddr)
	if err != nil {
		fmt.Printf("error starting server: %v\n", err)
		return
	}

	defer s.Listener.Close()
	fmt.Printf("server started on address %s and port %d\n", address, tcp.Port)

	listenerFd, err := tcp.GetFd(s.Listener)
	if err != nil {
		fmt.Printf("error getting listener fd: %v\n", err)
		return
	}

	s.Clients = make(map[int]*ClientConn)
	s.PollFds = []syscall.PollFd{
		{Fd: int32(listenerFd), Events: syscall.POLLIN},
	}

	for {
		n, err := syscall.Poll(s.PollFds, -1)
		if err != nil {
			fmt.Printf("poll error: %v\n", err)
			continue
		}

		if n == 0 {
			continue
		}

		for i := 0; i < len(s.PollFds); i++ {
			if s.PollFds[i].Revents == 0 {
				continue
			}

			if s.PollFds[i].Fd == int32(listenerFd) {
				s.handleNewConnection()
			} else {
				s.handleClientRequest(int(s.PollFds[i].Fd))
			}
		}
	}
}

func (s *Server) handleNewConnection() {
	conn, err := s.Listener.Accept()
	if err != nil {
		fmt.Printf("error accepting connection: %v\n", err)
		return
	}

	if err := tcp.SetKeepalive(conn); err != nil {
		fmt.Printf("error setting keepalive: %v\n", err)
		conn.Close()
		return
	}

	fd, err := tcp.GetFd(conn)
	if err != nil {
		fmt.Printf("error getting connection fd: %v\n", err)
		conn.Close()
		return
	}

	clientAddr := conn.RemoteAddr().String()
	s.ClientCount++
	client := &ClientConn{
		Fd:         fd,
		Conn:       conn,
		Addr:       clientAddr,
		CurrentDir: s.CurrentDir,
	}

	s.Clients[fd] = client
	s.PollFds = append(s.PollFds, syscall.PollFd{
		Fd:     int32(fd),
		Events: syscall.POLLIN,
	})

	fmt.Printf("new connection from %s (fd: %d)\n", clientAddr, fd)
}

func (s *Server) handleClientRequest(fd int) {
	client, ok := s.Clients[fd]
	if !ok {
		return
	}

	command, err := tcp.ReadData(client.Conn)
	if err != nil {
		fmt.Printf("client %s (fd: %d) disconnected: %v\n", client.Addr, fd, err)
		s.removeClient(fd)
		return
	}

	if command == "" {
		return
	}

	fmt.Printf("[%s] command: %s\n", client.Addr, command)
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return
	}

	response := s.ParseCommand(client, parts)
	if response != "" {
		if err := tcp.SendData(client.Conn, response); err != nil {
			fmt.Printf("error sending response to %s: %v\n", client.Addr, err)
			s.removeClient(fd)
			return
		}
		if response == "goodbye!" {
			s.removeClient(fd)
		}
	}
}

func (s *Server) removeClient(fd int) {
	client, ok := s.Clients[fd]
	if !ok {
		return
	}

	client.Conn.Close()
	delete(s.Clients, fd)

	for i := 0; i < len(s.PollFds); i++ {
		if s.PollFds[i].Fd == int32(fd) {
			s.PollFds = append(s.PollFds[:i], s.PollFds[i+1:]...)
			break
		}
	}

	fmt.Printf("connection closed (fd: %d, addr: %s)\n", fd, client.Addr)
}

func (s *Server) ParseCommand(client *ClientConn, parts []string) string {
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
		response = handleLs(client.CurrentDir)
	case "cd":
		response = handleCd(&client.CurrentDir, args...)
	case "download":
		tcp.Upload(client.CurrentDir, client.Conn, args...)
	case "upload":
		tcp.Download(client.CurrentDir, client.Conn, args...)
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
