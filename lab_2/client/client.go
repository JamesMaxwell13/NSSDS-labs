package client

import (
	"bufio"
	"fmt"
	"lab_2/udp"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Client struct {
	Conn       *net.UDPConn
	ServerAddr *net.UDPAddr
	CurrentDir string
	Timeout    time.Duration
}

func (c *Client) RunClient() {
	c.CurrentDir, _ = os.Getwd()
	c.Timeout = 5 * time.Second

	for {
		if err := c.connectToServer(); err != nil {
			fmt.Printf("Connection error: %v\n", err)
			time.Sleep(2 * time.Second)
			continue
		}
		if err := c.handleCommands(); err != nil {
			fmt.Printf("Command error: %v\n", err)
		}
	}
}

func (c *Client) connectToServer() error {
	fmt.Print("Enter server address (default: 127.0.0.1:8000): ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	serverAddr := scanner.Text()
	if serverAddr == "" {
		serverAddr = "127.0.0.1:8000"
	}

	var err error
	c.ServerAddr, err = net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return fmt.Errorf("error resolving address: %v", err)
	}

	c.Conn, err = net.ListenUDP("udp", nil)
	if err != nil {
		return fmt.Errorf("error creating connection: %v", err)
	}

	fmt.Printf("Connected to server at %s\n", serverAddr)
	return nil
}

func (c *Client) handleCommands() error {
	defer c.Conn.Close()
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Printf("[%s] >> ", c.ServerAddr.String())
		if !scanner.Scan() {
			break
		}
		command := scanner.Text()
		parts := strings.Fields(command)
		if len(parts) == 0 {
			continue
		}

		response, err := c.executeCommand(parts)
		if err != nil {
			return err
		}
		if response == "goodbye!" {
			fmt.Println(response)
			break
		}
		fmt.Println(response)
	}
	return nil
}

func (c *Client) executeCommand(parts []string) (string, error) {
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "echo":
		return c.sendCommand("echo " + strings.Join(args, " "))
	case "time":
		return c.sendCommand("time")
	case "quit", "exit", "close":
		return c.sendCommand("quit")
	case "ls":
		return c.sendCommand("ls")
	case "cd":
		if len(args) == 0 {
			return "error: path required", nil
		}
		return c.sendCommand("cd " + args[0])
	case "download":
		return c.handleDownload(args...)
	case "upload":
		return c.handleUpload(args...)
	default:
		return "error: unknown command", nil
	}
}

func (c *Client) sendCommand(cmd string) (string, error) {
	return udp.SendCommandWithResponse(c.Conn, c.ServerAddr, cmd, c.Timeout)
}

func (c *Client) handleUpload(args ...string) (string, error) {
	if len(args) == 0 {
		return "error: file name required", nil
	}

	response, err := c.sendCommand("upload " + args[0])
	if err != nil {
		return "", fmt.Errorf("upload command failed: %v", err)
	}

	if response != "ready" {
		return "", fmt.Errorf("server not ready: %s", response)
	}

	filePath := filepath.Join(c.CurrentDir, args[0])
	if err := udp.Upload(filePath, c.Conn, c.ServerAddr); err != nil {
		return "", fmt.Errorf("upload failed: %v", err)
	}

	// Wait for completion
	c.Conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	buf := make([]byte, 1024)
	n, _, err := c.Conn.ReadFromUDP(buf)
	if err != nil {
		return "", fmt.Errorf("failed to get completion: %v", err)
	}

	if string(buf[:n]) != "upload complete" {
		return "", fmt.Errorf("upload failed: %s", string(buf[:n]))
	}

	return "file uploaded successfully", nil
}

func (c *Client) handleDownload(args ...string) (string, error) {
	if len(args) == 0 {
		return "error: file name required", nil
	}

	response, err := c.sendCommand("download " + args[0])
	if err != nil {
		return "", fmt.Errorf("download command failed: %v", err)
	}

	if response != "ready" {
		return "", fmt.Errorf("server not ready: %s", response)
	}

	localFile := args[0]
	if len(args) > 1 {
		localFile = args[1]
	}

	filePath := filepath.Join(c.CurrentDir, localFile)
	if err := udp.Download(filePath, c.Conn, c.ServerAddr); err != nil {
		return "", fmt.Errorf("download failed: %v", err)
	}

	// Wait for completion
	c.Conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	buf := make([]byte, 1024)
	n, _, err := c.Conn.ReadFromUDP(buf)
	if err != nil {
		return "", fmt.Errorf("failed to get completion: %v", err)
	}

	if string(buf[:n]) != "download complete" {
		return "", fmt.Errorf("download failed: %s", string(buf[:n]))
	}

	return fmt.Sprintf("file downloaded successfully to %s", filePath), nil
}
