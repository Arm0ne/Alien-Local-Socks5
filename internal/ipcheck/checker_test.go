package ipcheck

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestCheckerUsesSOCKS5AndValidatesIP(t *testing.T) {
	web := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("203.0.113.42"))
	}))
	defer web.Close()
	socksAddress, closeSOCKS := startSOCKSServer(t)
	defer closeSOCKS()
	_, portText, _ := net.SplitHostPort(socksAddress)
	port, _ := strconv.Atoi(portText)

	address, err := (Checker{Endpoints: []string{web.URL}, Timeout: 3 * time.Second}).Check(context.Background(), port)
	if err != nil {
		t.Fatal(err)
	}
	if address != "203.0.113.42" {
		t.Fatalf("address = %q", address)
	}
}

func TestCheckerRejectsInvalidResponse(t *testing.T) {
	web := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("not-an-ip"))
	}))
	defer web.Close()
	socksAddress, closeSOCKS := startSOCKSServer(t)
	defer closeSOCKS()
	_, portText, _ := net.SplitHostPort(socksAddress)
	port, _ := strconv.Atoi(portText)
	if _, err := (Checker{Endpoints: []string{web.URL}, Timeout: 3 * time.Second}).Check(context.Background(), port); err == nil {
		t.Fatal("expected invalid response error")
	}
}

func startSOCKSServer(t *testing.T) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go serveSOCKS(connection)
		}
	}()
	return listener.Addr().String(), func() {
		_ = listener.Close()
		<-done
	}
}

func serveSOCKS(client net.Conn) {
	defer client.Close()
	reader := bufio.NewReader(client)
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil {
		return
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(reader, methods); err != nil {
		return
	}
	_, _ = client.Write([]byte{5, 0})
	request := make([]byte, 4)
	if _, err := io.ReadFull(reader, request); err != nil || request[1] != 1 {
		return
	}
	var host string
	switch request[3] {
	case 1:
		address := make([]byte, 4)
		_, _ = io.ReadFull(reader, address)
		host = net.IP(address).String()
	case 3:
		length, err := reader.ReadByte()
		if err != nil {
			return
		}
		address := make([]byte, int(length))
		_, _ = io.ReadFull(reader, address)
		host = string(address)
	case 4:
		address := make([]byte, 16)
		_, _ = io.ReadFull(reader, address)
		host = net.IP(address).String()
	default:
		return
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(reader, portBytes); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(portBytes)
	upstream, err := net.DialTimeout("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)), 2*time.Second)
	if err != nil {
		_, _ = client.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer upstream.Close()
	_, _ = client.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
	go io.Copy(upstream, reader)
	_, _ = io.Copy(client, upstream)
}
