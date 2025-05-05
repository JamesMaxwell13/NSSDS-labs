package server

import (
	"bufio"
	"fmt"
	"lab_4/tcp"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Server struct {
	Listener   net.Listener
	ServerAddr string
	CurrentDir string
	ClientPool *ClientPool
}

type ClientPool struct {
	Workers    int
	JobQueue   chan net.Conn
	Wg         sync.WaitGroup
	CurrentDir string
}

func NewClientPool(workers int, dir string) *ClientPool {
	return &ClientPool{
		Workers:    workers,
		JobQueue:   make(chan net.Conn),
		CurrentDir: dir,
	}
}

func (p *ClientPool) Start() {
	for i := 0; i < p.Workers; i++ {
		p.Wg.Add(1)
		go p.worker()
	}
}

func (p *ClientPool) worker() {
	defer p.Wg.Done()
	for conn := range p.JobQueue {
		handleClient(conn, p.CurrentDir)
	}
}

func (p *ClientPool) Stop() {
	close(p.JobQueue)
	p.Wg.Wait()
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

	// Create client pool with 10 workers
	s.ClientPool = NewClientPool(10, s.CurrentDir)
	s.ClientPool.Start()
	defer s.ClientPool.Stop()

	for {
		conn, err := s.Listener.Accept()
		if err != nil {
			fmt.Printf("error accepting connection: %v\n", err)
			continue
		}

		if err := tcp.SetKeepalive(conn); err != nil {
			fmt.Printf("error setting keepalive: %v\n", err)
			conn.Close()
			continue
		}

		s.ClientPool.JobQueue <- conn
	}
}

func handleClient(conn net.Conn, currentDir string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("recovered from panic in client handler: %v\n", r)
		}
		conn.Close()
	}()

	clientAddr := conn.RemoteAddr().String()
	fmt.Printf("new connection from %s\n", clientAddr)

	client := &ClientConn{
		Conn:       conn,
		Addr:       clientAddr,
		CurrentDir: currentDir,
	}

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		command := scanner.Text()
		parts := strings.Fields(command)
		if len(parts) == 0 {
			continue
		}
		fmt.Printf("[%s] command: %s\n", clientAddr, command)
		response := client.ParseCommand(parts)
		if response != "" {
			if err := tcp.SendData(conn, response); err != nil {
				fmt.Printf("error sending response to %s: %v\n", clientAddr, err)
				return
			}
			if response == "goodbye!" {
				return
			}
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("client %s disconnected with error: %v\n", clientAddr, err)
	}
}

type ClientConn struct {
	Conn       net.Conn
	Addr       string
	CurrentDir string
}

func (c *ClientConn) ParseCommand(parts []string) string {
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
		response = handleLs(c.CurrentDir)
	case "cd":
		response = handleCd(&c.CurrentDir, args...)
	case "download":
		tcp.Upload(c.CurrentDir, c.Conn, args...)
	case "upload":
		tcp.Download(c.CurrentDir, c.Conn, args...)
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
