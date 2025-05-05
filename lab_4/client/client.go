package client

import (
	"bufio"
	"fmt"
	"lab_4/tcp"
	"net"
	"os"
	"path/filepath"
	"strings"
)

type Client struct {
	Conn       net.Conn
	ServerAddr string
	CurrentDir string
}

func (c *Client) RunClient() {
	for {
		err := c.initiateConnection()
		if err != nil {
			fmt.Println("Failed to connect to server:", err)
			continue
		}
		c.handleServer()
	}
}

func (c *Client) initiateConnection() error {
	fmt.Print("Enter server address (default: 127.0.0.1:8000): ")
	_, err := fmt.Scanln(&(c.ServerAddr))
	if err != nil || c.ServerAddr == "" {
		c.ServerAddr = "127.0.0.1:8000"
	}

	c.Conn, err = net.Dial("tcp", c.ServerAddr)
	if err != nil {
		return fmt.Errorf("error connecting to server: %v", err)
	}

	c.CurrentDir, _ = os.Getwd()
	err = tcp.SetKeepalive(c.Conn)
	if err != nil {
		_ = c.Conn.Close()
		return fmt.Errorf("failed to set keepalive: %v", err)
	}

	fmt.Printf("Connected to server at %s\n", c.ServerAddr)
	return nil
}

func (c *Client) handleServer() {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("[%s] >> ", c.ServerAddr)
		if !scanner.Scan() {
			break
		}

		command := scanner.Text()
		parts := strings.Fields(command)
		if len(parts) == 0 {
			continue
		}

		response := c.parseCommand(parts)
		if response == "goodbye!" {
			fmt.Println(response)
			break
		}
		fmt.Println(response)
	}
}

func (c *Client) parseCommand(parts []string) string {
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "echo":
		return c.handleEcho(args...)
	case "time":
		return c.handleTime()
	case "quit", "exit", "close":
		return c.handleQuit()
	case "ls":
		return c.handleLs()
	case "cd":
		return c.handleCd(args...)
	case "download":
		return c.handleDownload(args...)
	case "upload":
		return c.handleUpload(args...)
	case "cls":
		return fmt.Sprintf("Client local directory: %s", c.CurrentDir)
	default:
		return "error: unknown command"
	}
}

func (c *Client) handleEcho(args ...string) string {
	command := "echo " + strings.Join(args, " ")
	err := tcp.SendData(c.Conn, command)
	if err != nil {
		return fmt.Sprintf("error sending echo command: %v", err)
	}
	response, err := tcp.ReadData(c.Conn)
	if err != nil {
		return fmt.Sprintf("error reading echo response: %v", err)
	}
	return response
}

func (c *Client) handleTime() string {
	err := tcp.SendData(c.Conn, "time")
	if err != nil {
		return fmt.Sprintf("error sending time command: %v", err)
	}
	response, err := tcp.ReadData(c.Conn)
	if err != nil {
		return fmt.Sprintf("error reading time response: %v", err)
	}
	return response
}

func (c *Client) handleQuit() string {
	err := tcp.SendData(c.Conn, "quit")
	if err != nil {
		return fmt.Sprintf("error sending quit command: %v", err)
	}
	_ = c.Conn.Close()
	return "goodbye!"
}

func (c *Client) handleLs() string {
	err := tcp.SendData(c.Conn, "ls")
	if err != nil {
		return fmt.Sprintf("error sending ls command: %v", err)
	}
	response, err := tcp.ReadData(c.Conn)
	if err != nil {
		return fmt.Sprintf("error reading ls response: %v", err)
	}
	return response
}

func (c *Client) handleCd(args ...string) string {
	if len(args) == 0 {
		return "error: path required"
	}
	command := "cd " + args[0]
	err := tcp.SendData(c.Conn, command)
	if err != nil {
		return fmt.Sprintf("error sending cd command: %v", err)
	}
	response, err := tcp.ReadData(c.Conn)
	if err != nil {
		return fmt.Sprintf("error reading cd response: %v", err)
	}
	return response
}

func (c *Client) handleDownload(args ...string) string {
	if len(args) == 0 {
		return "error: file name required"
	}
	command := "download " + args[0]
	err := tcp.SendData(c.Conn, command)
	if err != nil {
		return fmt.Sprintf("error sending download command: %v", err)
	}
	tcp.Download(c.CurrentDir, bufio.NewReader(c.Conn), args...)
	return fmt.Sprintf("File downloaded to: %s", filepath.Join(c.CurrentDir, args[0]))
}

func (c *Client) handleUpload(args ...string) string {
	if len(args) == 0 {
		return "error: file name required"
	}
	command := "upload " + args[0]
	err := tcp.SendData(c.Conn, command)
	if err != nil {
		return fmt.Sprintf("error sending upload command: %v", err)
	}
	tcp.Upload(c.CurrentDir, bufio.NewWriter(c.Conn), args...)
	return fmt.Sprintf("File uploaded: %s", args[0])
}
