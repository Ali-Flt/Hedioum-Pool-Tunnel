package mimic

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"time"
)

const (
	// Default fallback banner if dynamic extraction fails
	defaultSSHBanner = "SSH-2.0-OpenSSH_8.9p1 Ubuntu-3ubuntu0.10\r\n"
	maxPaddingLength = 64
	minPaddingLength = 16
	handshakeTimeout = 5 * time.Second
)

// GetDynamicSSHBanner executes local ssh binary to extract the exact OS version
// and formats it as a valid RFC 4253 SSH identification string.
func GetDynamicSSHBanner() string {
	// 'ssh -V' prints version info to stderr
	cmd := exec.Command("ssh", "-V")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil && stderr.Len() == 0 {
		return defaultSSHBanner
	}

	output := strings.TrimSpace(stderr.String())
	// Expected output: "OpenSSH_8.9p1 Ubuntu-3ubuntu0.10, OpenSSL 3.0.2..."
	parts := strings.Split(output, ",")
	if len(parts) > 0 {
		// Construct valid banner: "SSH-2.0-SoftwareVersion\r\n"
		return fmt.Sprintf("SSH-2.0-%s\r\n", strings.TrimSpace(parts[0]))
	}

	return defaultSSHBanner
}

// PerformClientHandshake sends the dynamic banner, reads the server banner,
// and sends the obfuscated metadata (target + token) wrapped as an SSH binary packet.
func PerformClientHandshake(conn net.Conn, token string, targetAddr string) error {
	if err := conn.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return err
	}
	defer conn.SetDeadline(time.Time{})

	// 1. Send our Client Banner
	banner := GetDynamicSSHBanner()
	if _, err := conn.Write([]byte(banner)); err != nil {
		return fmt.Errorf("failed to write client banner: %w", err)
	}

	// 2. Read Server Banner
	reader := make([]byte, 256)
	if _, err := conn.Read(reader); err != nil {
		return fmt.Errorf("failed to read server banner: %w", err)
	}

	// 3. Construct Obfuscated Metadata Payload
	// Payload structure: [1 byte TokenLen] [Token] [2 bytes TargetLen] [Target]
	payload := new(bytes.Buffer)
	payload.WriteByte(byte(len(token)))
	payload.WriteString(token)

	targetLenBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(targetLenBytes, uint16(len(targetAddr)))
	payload.Write(targetLenBytes)
	payload.WriteString(targetAddr)

	// 4. Wrap Payload in RFC 4253 SSH Binary Packet Format
	// Packet: [4 bytes PacketLen] [1 byte PaddingLen] [Payload] [Random Padding]
	paddingLen := generateRandomInt(minPaddingLength, maxPaddingLength)
	packetLen := uint32(1 + payload.Len() + paddingLen) // 1 byte for PaddingLen itself

	packet := new(bytes.Buffer)
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, packetLen)
	packet.Write(lengthBytes)
	packet.WriteByte(byte(paddingLen))
	packet.Write(payload.Bytes())

	// Append cryptographically secure random padding
	randomPadding := make([]byte, paddingLen)
	if _, err := rand.Read(randomPadding); err != nil {
		return errors.New("failed to generate secure padding noise")
	}
	packet.Write(randomPadding)

	// Send the completely obfuscated packet
	if _, err := conn.Write(packet.Bytes()); err != nil {
		return fmt.Errorf("failed to send obfuscated metadata packet: %w", err)
	}

	return nil
}

// PerformServerHandshake sends the server banner, reads the client banner,
// and extracts the target address from the obfuscated SSH binary packet.
func PerformServerHandshake(conn net.Conn, expectedToken string) (string, error) {
	if err := conn.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return "", err
	}
	defer conn.SetDeadline(time.Time{})

	// 1. Send our Server Banner
	banner := GetDynamicSSHBanner()
	if _, err := conn.Write([]byte(banner)); err != nil {
		return "", fmt.Errorf("failed to write server banner: %w", err)
	}

	// 2. Read Client Banner
	bannerBuf := make([]byte, 256)
	n, err := conn.Read(bannerBuf)
	if err != nil {
		return "", fmt.Errorf("failed to read client banner: %w", err)
	}

	// We only process if it starts with "SSH-2.0" to drop standard HTTP scanners immediately
	if !strings.HasPrefix(string(bannerBuf[:n]), "SSH-2.0") {
		return "", errors.New("invalid protocol banner signature")
	}

	// 3. Read Obfuscated Packet Header (4 bytes PacketLen + 1 byte PaddingLen)
	header := make([]byte, 5)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", errors.New("failed to read metadata packet header")
	}

	packetLen := binary.BigEndian.Uint32(header[0:4])
	paddingLen := int(header[4])

	// Validate packet sanity to prevent buffer overflow attacks
	payloadLen := int(packetLen) - 1 - paddingLen
	if payloadLen <= 0 || payloadLen > 1024 {
		return "", errors.New("malformed obfuscated packet dimensions")
	}

	// 4. Read Payload + Padding
	bodyBuf := make([]byte, payloadLen+paddingLen)
	if _, err := io.ReadFull(conn, bodyBuf); err != nil {
		return "", errors.New("failed to read obfuscated payload body")
	}

	// 5. Extract and Validate Token
	payloadData := bodyBuf[:payloadLen]
	tokenLen := int(payloadData[0])

	if tokenLen+1 > payloadLen {
		return "", errors.New("payload bounds exceeded reading token")
	}

	receivedToken := string(payloadData[1 : 1+tokenLen])
	if receivedToken != expectedToken {
		return "", errors.New("authentication token mismatch - rogue scanner dropped")
	}

	// 6. Extract Target Address
	targetLenOffset := 1 + tokenLen
	if targetLenOffset+2 > payloadLen {
		return "", errors.New("payload bounds exceeded reading target length")
	}

	targetLen := int(binary.BigEndian.Uint16(payloadData[targetLenOffset : targetLenOffset+2]))
	targetStrOffset := targetLenOffset + 2

	if targetStrOffset+targetLen > payloadLen {
		return "", errors.New("payload bounds exceeded reading target string")
	}

	targetAddr := string(payloadData[targetStrOffset : targetStrOffset+targetLen])

	return targetAddr, nil
}

// generateRandomInt returns a pseudo-random integer within the specified range
func generateRandomInt(min, max int) int {
	b := make([]byte, 1)
	_, _ = rand.Read(b)
	return min + int(b[0])%(max-min+1)
}
