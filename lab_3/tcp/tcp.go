package tcp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	Port          = 8000
	KeepaliveIdle = 30 * time.Second
	BufferSize    = 128 * 1024
	ProgressWidth = 50
)

func GetFd(conn net.Conn) (int, error) {
	switch c := conn.(type) {
	case *net.TCPConn:
		file, err := c.File()
		if err != nil {
			return 0, err
		}
		return int(file.Fd()), nil
	case *net.UnixConn:
		file, err := c.File()
		if err != nil {
			return 0, err
		}
		return int(file.Fd()), nil
	default:
		return 0, fmt.Errorf("unsupported connection type")
	}
}

func SetKeepalive(conn net.Conn) error {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return fmt.Errorf("not a TCP connection")
	}
	if err := tcpConn.SetKeepAlive(true); err != nil {
		return fmt.Errorf("error setting keepalive: %v", err)
	}
	if err := tcpConn.SetKeepAlivePeriod(KeepaliveIdle); err != nil {
		return fmt.Errorf("error setting keepalive period: %v", err)
	}
	return nil
}

func SendData(conn net.Conn, data string) error {
	if _, err := fmt.Fprintln(conn, data); err != nil {
		return fmt.Errorf("error writing to connection: %v", err)
	}
	return nil
}

func ReadData(conn net.Conn) (string, error) {
	reader := bufio.NewReader(conn)

	data, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("error reading data: %v", err)
	}

	return strings.TrimSpace(data), nil
}

func GetIP() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		fmt.Println("Error getting network interfaces:", err)
		return "", err
	}

	for _, interf := range interfaces {
		if IsWirelessInterface(interf.Name) {
			addrs, err := interf.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				ipNet, ok := addr.(*net.IPNet)
				if ok && !ipNet.IP.IsLoopback() {
					if ipNet.IP.To4() != nil {
						return ipNet.String(), nil
					}
				}
			}
		}
	}
	return "can't find wireless interface", nil
}

func IsWirelessInterface(name string) bool {
	return strings.HasPrefix(name, "wlan") ||
		strings.HasPrefix(name, "wl") ||
		strings.Contains(name, "Wi-Fi") ||
		name == "Беспроводная сеть"
}

func Download(localDir string, conn net.Conn, args ...string) {
	metaData, err := ReadData(conn)
	if err != nil {
		fmt.Printf("error receiving metadata: %v\n", err)
		return
	}
	metaParts := strings.Split(metaData, "|")
	if len(metaParts) != 2 {
		fmt.Println("error: invalid metadata format")
		return
	}
	fileName := metaParts[0]
	var fileSize int64
	_, err = fmt.Sscanf(metaParts[1], "%d", &fileSize)
	if err != nil {
		fmt.Printf("error parsing file size: %v\n", err)
		return
	}
	localFilePath := filepath.Join(localDir, fileName)
	localFilePath = GetUniqueFileName(localFilePath)

	file, err := os.Create(localFilePath)
	if err != nil {
		fmt.Printf("error creating file: %v\n", err)
		return
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	buffer := make([]byte, BufferSize)
	var receivedBytes int64
	startTime := time.Now()

	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if n == 0 && err == io.EOF {
				break
			}
			fmt.Printf("error reading data: %v\n", err)
			return
		}
		data := string(buffer[:n])
		if strings.Contains(data, "[EOF]") {
			eofIndex := strings.Index(data, "[EOF]")
			_, err = file.Write(buffer[:eofIndex])
			if err != nil {
				fmt.Printf("error writing to file: %v\n", err)
			}
			break
		}
		_, err = file.Write(buffer[:n])
		if err != nil {
			fmt.Printf("error writing to file: %v\n", err)
			return
		}
		receivedBytes += int64(n)
		PrintProgress(receivedBytes, fileSize, startTime)
	}

	duration := time.Since(startTime)
	speed := float64(receivedBytes) / duration.Seconds() / 1024 // KB/s
	fmt.Printf("\ndownload completed: %d bytes in %.2f seconds (%.2f KB/s)\n",
		receivedBytes, duration.Seconds(), speed)
}

func Upload(localDir string, conn net.Conn, args ...string) {
	if len(args) == 0 {
		_ = SendData(conn, "error: file name required")
		return
	}
	localFileName := args[0]
	localFilePath := filepath.Join(localDir, localFileName)
	file, err := os.Open(localFilePath)
	if err != nil {
		_ = SendData(conn, "error: failed to open file")
		return
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	fileInfo, err := file.Stat()
	if err != nil {
		_ = SendData(conn, "error: failed to get file info")
		return
	}
	totalBytes := fileInfo.Size()

	metaData := fmt.Sprintf("%s|%d", localFileName, totalBytes)
	err = SendData(conn, metaData)
	if err != nil {
		fmt.Printf("error sending metadata: %v\n", err)
		return
	}

	buffer := make([]byte, BufferSize)
	var sentBytes int64
	startTime := time.Now()

	for {
		n, err := file.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Printf("error reading file: %v\n", err)
			return
		}
		_, err = conn.Write(buffer[:n])
		if err != nil {
			fmt.Printf("error sending data: %v\n", err)
			return
		}
		sentBytes += int64(n)
		PrintProgress(sentBytes, totalBytes, startTime)
	}

	_, _ = conn.Write([]byte("[EOF]"))
	duration := time.Since(startTime)
	speed := float64(totalBytes) / duration.Seconds() / 1024
	fmt.Printf("\nupload completed: %d bytes in %.2f seconds (%.2f KB/s)\n",
		totalBytes, duration.Seconds(), speed)
}

func PrintProgress(current, total int64, startTime time.Time) {
	percent := float64(current) / float64(total) * 100
	completed := int(percent / (100.0 / ProgressWidth))
	remaining := ProgressWidth - completed
	elapsed := time.Since(startTime).Seconds()
	speed := float64(current) / elapsed
	remainingTime := float64(total-current) / speed

	fmt.Printf("\r[%s%s] %.2f%% (%d / %d KB) | %.2f KB/s | ETA: %.1f sec",
		strings.Repeat("=", completed),
		strings.Repeat(" ", remaining),
		percent,
		current/1024,
		total/1024,
		speed/1024,
		remainingTime)
}

func GetUniqueFileName(filePath string) string {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return filePath
	}

	dir := filepath.Dir(filePath)
	ext := filepath.Ext(filePath)
	base := strings.TrimSuffix(filepath.Base(filePath), ext)
	i := 1
	for {
		newFileName := fmt.Sprintf("%s(%d)%s", base, i, ext)
		newPath := filepath.Join(dir, newFileName)
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
		i++
	}
}
