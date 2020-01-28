package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"

	quic "github.com/lucas-clemente/quic-go"
)

func holepunch(port, remoteAddr string) error {
	_, port, err := net.SplitHostPort(port)
	if err != nil {
		return err
	}
	listenAddr, err := net.ResolveUDPAddr("udp4", ":"+port)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		return err
	}
	listener, err := quic.Listen(conn, GenerateTLSConfig(), nil)
	if err != nil {
		return err
	}

	connects := make(chan *net.UDPAddr)
	if remoteAddr != "" {
		rAddr, err := net.ResolveUDPAddr("udp4", remoteAddr)
		if err != nil {
			return err
		}
		connects <- rAddr
	}
	go func() {
		for {
			log.Println("Listening for connections")
			sess, err := listener.Accept(context.Background())
			if err != nil {
				return
			}
			log.Printf("Accepted session from %s\n", sess.RemoteAddr().String())
			stream, err := sess.AcceptUniStream(context.Background())
			if err != nil {
				return
			}
			s, err := ReadLenPrefixedString(stream)
			if err != nil {
				return
			}
			fmt.Printf("Got message: %s\n", s)

			rAddr, err := net.ResolveUDPAddr("udp4", sess.RemoteAddr().String())
			if err != nil {
				return
			}
			connects <- rAddr
			for {
				s, err := ReadLenPrefixedString(stream)
				if err != nil {
					return
				}
				fmt.Printf("Got message: %s\n", s)
			}
		}
	}()

	rAddr := <-connects
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"},
	}
	log.Printf("Attempting to dial %+v\n", rAddr)
	var sess quic.Session
	for {
		sess, err = quic.Dial(conn, rAddr, rAddr.String(), tlsConf, nil)
		if err == nil {
			break
		}
		log.Println("Dial failed, attempting again")
	}
	stream, err := sess.OpenUniStreamSync(context.Background())
	if err != nil {
		return err
	}
	log.Println("Opened stream with peer")

	_, err = stream.Write(PrefixStringWithLen("helloworld"))
	if err != nil {
		return err
	}

	return nil
}

func main() {
	port := flag.String("port", ":10200", "the port to listen on")
	remoteAddr := flag.String("remoteAddr", "", "the address to dial")
	flag.Parse()
	log.Fatal(holepunch(*port, *remoteAddr))
}
