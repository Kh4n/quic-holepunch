package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	quic "github.com/lucas-clemente/quic-go"
)

type rendezvousConn struct {
	mux      sync.Mutex
	stream   quic.Stream
	peerAddr *net.UDPAddr
}

type peerStreams struct {
	mux     sync.Mutex
	streams []quic.SendStream
}

func startRendezvousServer(port string) error {
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
	log.Println("Listening for peers")
	listener, err := quic.Listen(conn, GenerateTLSConfig(), &quic.Config{KeepAlive: true})
	if err != nil {
		return err
	}

	conns := make(map[string]*rendezvousConn, 0)
	for {
		sess, err := listener.Accept(context.Background())
		if err != nil {
			return err
		}
		log.Printf("Accepted connection from %s\n", sess.RemoteAddr().String())
		go func() {
			stream, err := sess.AcceptStream(context.Background())
			if err != nil {
				return
			}
			peerID, err := ReadLenPrefixedString(stream)
			if err != nil {
				return
			}
			rAddr, ok := sess.RemoteAddr().(*net.UDPAddr)
			if !ok {
				return
			}
			for pID, rc := range conns {
				if pID != peerID {
					rc.mux.Lock()
					_, err = rc.stream.Write(PrefixStringWithLen(rAddr.String()))
					if err != nil {
						log.Printf("Error: %s\n", err)
						rc.mux.Unlock()
						return
					}
					rc.mux.Unlock()
					_, err := stream.Write(PrefixStringWithLen(rc.peerAddr.String()))
					if err != nil {
						return
					}
				}
			}
			rc := &rendezvousConn{
				stream:   stream,
				peerAddr: rAddr,
			}
			conns[peerID] = rc
		}()
	}
}

func holepunchRendezvous(peerID, port, rendezvousAddr string) error {
	_, port, err := net.SplitHostPort(port)
	if err != nil {
		return err
	}
	listenAddr, err := net.ResolveUDPAddr("udp4", ":"+port)
	if err != nil {
		return err
	}
	rAddr, err := net.ResolveUDPAddr("udp4", rendezvousAddr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		return err
	}
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-holepunch"},
	}
	log.Printf("Dialing rendezvous: %s\n", rendezvousAddr)
	rSess, err := quic.Dial(conn, rAddr, rendezvousAddr, tlsConf, &quic.Config{KeepAlive: true})
	if err != nil {
		return err
	}
	log.Printf("Successfully connected to rendezvous: %s\n", rSess.RemoteAddr().String())
	rStream, err := rSess.OpenStreamSync(context.Background())
	if err != nil {
		return err
	}
	_, err = rStream.Write(PrefixStringWithLen(peerID))
	if err != nil {
		return err
	}
	peerConns := make(chan *net.UDPAddr)

	go func() {
		for {
			addr, err := ReadLenPrefixedString(rStream)
			if err != nil {
				log.Printf("Error: %s\n", err)
				return
			}
			peerAddr, err := net.ResolveUDPAddr("udp4", addr)
			if err != nil {
				log.Printf("Error: %s\n", err)
				return
			}
			peerConns <- peerAddr
		}
	}()

	streams := peerStreams{
		streams: make([]quic.SendStream, 0),
	}
	listener, err := quic.Listen(conn, GenerateTLSConfig(), nil)
	if err != nil {
		return err
	}
	go func() {
		for {
			peerConn := <-peerConns
			go func() {
				listenerCtx, cancel := context.WithCancel(context.Background())
				defer cancel()
				go func() {
					log.Printf("Trying to listen for connection from %s\n", peerConn.String())
					sess, err := listener.Accept(listenerCtx)
					if err != nil {
						log.Printf("Error: %s\n", err)
						return
					}
					stream, err := sess.AcceptUniStream(listenerCtx)
					if err != nil {
						log.Printf("Error: %s\n", err)
						return
					}
					for {
						msg, err := ReadLenPrefixedString(stream)
						if err != nil {
							log.Printf("Error: %s\n", err)
							return
						}
						fmt.Printf("Message recieved: %s\n", msg)
					}
				}()
				var sess quic.Session
				for i := 0; i < 5; i++ {
					sess, err = quic.Dial(conn, peerConn, peerConn.String(), tlsConf, nil)
					if err == nil {
						break
					}
					log.Println("Dial failed, reattempting")
				}
				if sess == nil {
					log.Println("Unable to establish connection")
					return
				}
				log.Println("Successfully established connection")
				stream, err := sess.OpenUniStreamSync(context.Background())
				if err != nil {
					log.Printf("Error: %s\n", err)
					return
				}
				log.Println("Adding stream to streams list")
				streams.mux.Lock()
				streams.streams = append(streams.streams, stream)
				streams.mux.Unlock()
			}()
		}
	}()
	for {
		if len(streams.streams) > 0 {
			break
		}
		time.Sleep(1 * time.Second)
	}
	for {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter message: ")
		text, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		msg := fmt.Sprintf("%s: %s", peerID, text)
		for _, stream := range streams.streams {
			_, err = stream.Write(PrefixStringWithLen(msg))
			if err != nil {
				return err
			}
			fmt.Println()
		}
	}
}
