package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net"

	quic "github.com/lucas-clemente/quic-go"
)

type connectInfo struct {
	port, remoteAddr           *net.UDPAddr
	configListen, configDial   *quic.Config
	tlsConfListen, tlsConfDial *tls.Config
}

func DefaultConnectInfo(port, remoteAddr string) (*connectInfo, error) {
	c := &connectInfo{}
	_, port, err := net.SplitHostPort(port)
	if err != nil {
		return nil, err
	}
	listenAddr, err := net.ResolveUDPAddr("udp4", port)
	if err != nil {
		return nil, err
	}
	rAddr, err := net.ResolveUDPAddr("udp4", remoteAddr)
	if err != nil {
		return nil, err
	}
	c.port = listenAddr
	c.remoteAddr = rAddr

	tlsConfListen := GenerateTLSConfig()
	tlsConfDial := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"},
	}
	c.tlsConfListen = tlsConfListen
	c.tlsConfDial = tlsConfDial

	return c, nil
}

func listenAndDial(ci *connectInfo) (quic.Listener, quic.Session, error) {
	conn, err := net.ListenUDP("udp", ci.port)
	if err != nil {
		return nil, nil, err
	}

	listener, err := quic.Listen(conn, ci.tlsConfListen, ci.configListen)
	if err != nil {
		return nil, nil, err
	}
	sess, err := quic.Dial(conn, ci.remoteAddr, ci.remoteAddr.String(), ci.tlsConfDial, ci.configDial)
	if err != nil {
		return nil, nil, err
	}

	return listener, sess, nil
}

// GenerateTLSConfig : Setup a bare-bones TLS config for the server
func GenerateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"quic-echo-example"},
	}
}

// PrefixStringWithLen : prefixes a string with its length, for use with ReadPrefixedStringWithLen
// fails if len(s) == 0
func PrefixStringWithLen(s string) []byte {
	if len(s) == 0 {
		log.Fatal("Cannot prefix a string with length 0")
	}
	lbuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lbuf, uint32(len(s)))
	return append(lbuf, []byte(s)...)
}

// ReadLenPrefixedString : reads in a string from the reader assuming that the first 4 bytes
// are the length of the string
func ReadLenPrefixedString(r io.Reader) (string, error) {
	buf := make([]byte, 4)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return "", err
	}
	len := binary.BigEndian.Uint32(buf)

	ret := make([]byte, len)
	_, err = io.ReadFull(r, ret)
	if err != nil {
		return "", err
	}

	return string(ret), nil
}

// ReadByte : reads a byte from a reader and returns it
func ReadByte(r io.Reader) (byte, error) {
	buf := make([]byte, 1)
	_, err := r.Read(buf)
	if err != nil {
		return 0xFF, err
	}
	return buf[0], nil
}

func ValidatePort(port string) (*net.UDPConn, error) {
	_, port, err := net.SplitHostPort(port)
	if err != nil {
		return nil, err
	}

	listenAddr, err := net.ResolveUDPAddr("udp4", port)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		return nil, err
	}

	return conn, nil
}
