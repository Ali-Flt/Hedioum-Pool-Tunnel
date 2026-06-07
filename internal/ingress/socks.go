package ingress

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

const (
	socksVersion5    = 0x05
	cmdConnect       = 0x01
	addrTypeIPv4     = 0x01
	addrTypeDomain   = 0x03
	addrTypeIPv6     = 0x04
	handshakeTimeout = 3 * time.Second
)

// HandleSocks5Handshake processes the initial SOCKS5 negotiation from local X-UI.
// It extracts and returns the target destination address (e.g., "google.com:443").
// Strict timeouts are enforced to prevent resource exhaustion from port scanners.
func HandleSocks5Handshake(conn net.Conn) (string, error) {
	// Enforce a strict deadline for the entire handshake process
	if err := conn.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return "", err
	}

	// Step 1: Read Greeting (Version and NMETHODS)
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(conn, greeting); err != nil {
		return "", errors.New("failed to read SOCKS5 greeting")
	}

	if greeting[0] != socksVersion5 {
		return "", errors.New("invalid SOCKS version")
	}

	numMethods := int(greeting[1])
	methods := make([]byte, numMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", errors.New("failed to read SOCKS5 auth methods")
	}

	// Respond with No-Authentication method (0x00)
	if _, err := conn.Write([]byte{socksVersion5, 0x00}); err != nil {
		return "", errors.New("failed to write SOCKS5 auth response")
	}

	// Step 2: Read Client Connection Request
	// Header: VER(1), CMD(1), RSV(1), ATYP(1)
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", errors.New("failed to read SOCKS5 request header")
	}

	if header[0] != socksVersion5 || header[1] != cmdConnect {
		return "", errors.New("unsupported SOCKS5 command (only CONNECT is supported)")
	}

	addrType := header[3]
	var destAddr string

	// Parse Destination Address based on ATYP
	switch addrType {
	case addrTypeIPv4:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return "", errors.New("failed to read IPv4 address")
		}
		destAddr = net.IP(ip).String()

	case addrTypeDomain:
		domainLenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, domainLenBuf); err != nil {
			return "", errors.New("failed to read domain length")
		}
		domainLen := int(domainLenBuf[0])
		domain := make([]byte, domainLen)
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", errors.New("failed to read domain name")
		}
		destAddr = string(domain)

	case addrTypeIPv6:
		ip := make([]byte, 16)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return "", errors.New("failed to read IPv6 address")
		}
		// Wrap IPv6 in brackets for standard URL formatting
		destAddr = fmt.Sprintf("[%s]", net.IP(ip).String())

	default:
		return "", errors.New("unsupported address type")
	}

	// Step 3: Read Destination Port (2 bytes, Big Endian)
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", errors.New("failed to read destination port")
	}
	destPort := binary.BigEndian.Uint16(portBuf)

	// Construct final target string
	targetAddress := net.JoinHostPort(destAddr, strconv.Itoa(int(destPort)))

	// Step 4: Send Success Response
	// Reply: VER(0x05), REP(0x00 Success), RSV(0x00), ATYP(0x01 IPv4), BND.ADDR(4 bytes 0), BND.PORT(2 bytes 0)
	successReply := []byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}
	if _, err := conn.Write(successReply); err != nil {
		return "", errors.New("failed to write SOCKS5 success response")
	}

	// Remove the deadline so the ongoing tunnel connection doesn't timeout
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return "", err
	}

	return targetAddress, nil
}
