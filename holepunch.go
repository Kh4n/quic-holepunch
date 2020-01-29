package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	quic "github.com/lucas-clemente/quic-go"
)

func multiConnTest(port, remoteAddr1, remoteAddr2 string) error {
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

	go func() {
		for {
			sess, err := listener.Accept(context.Background())
			if err != nil {
				return
			}
			go func() {
				stream, err := sess.AcceptUniStream(context.Background())
				if err != nil {
					return
				}
				for {
					msg, err := ReadLenPrefixedString(stream)
					if err != nil {
						return
					}
					fmt.Printf("Message recieved: %s\n", msg)
				}
			}()
		}
	}()
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-holepunch"},
	}
	rAddr1, err := net.ResolveUDPAddr("udp4", remoteAddr1)
	if err != nil {
		return err
	}
	rAddr2, err := net.ResolveUDPAddr("udp4", remoteAddr2)
	if err != nil {
		return err
	}
	go func() {
		sess, err := quic.Dial(conn, rAddr1, remoteAddr1, tlsConf, nil)
		if err != nil {
			return
		}
		stream, err := sess.OpenUniStreamSync(context.Background())
		if err != nil {
			return
		}
		_, err = stream.Write(PrefixStringWithLen("helloworld"))
		if err != nil {
			return
		}
		time.Sleep(1 * time.Second)
	}()
	sess, err := quic.Dial(conn, rAddr2, remoteAddr2, tlsConf, nil)
	if err != nil {
		return err
	}
	stream, err := sess.OpenUniStreamSync(context.Background())
	if err != nil {
		return err
	}
	_, err = stream.Write(PrefixStringWithLen("helloworld"))
	if err != nil {
		return err
	}
	select {}
}

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

	connects := make(chan *net.UDPAddr, 1)
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
		NextProtos:         []string{"quic-holepunch"},
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

	time.Sleep(10 * time.Second)
	return nil
}

func main() {
	port := flag.String("port", ":10200", "the port to listen on")
	peerID := flag.String("peerID", "", "the unique peerID to use")
	rendezvousAddr := flag.String("rendezvousAddr", "", "address of rendezvous server")

	remoteAddr := flag.String("remoteAddr", "", "remote address to dial, if doing simple holepunch. use none to listen")
	flag.Parse()

	if *port != "" && *rendezvousAddr != "" && *peerID != "" {
		err := holepunchRendezvous(*peerID, *port, *rendezvousAddr)
		if err != nil {
			log.Fatal(err)
		}
	} else if *port != "" && *remoteAddr != "" {
		if *remoteAddr == "none" {
			*remoteAddr = ""
		}
		err := holepunch(*port, *remoteAddr)
		if err != nil {
			log.Fatal(err)
		}
	} else if *port != "" {
		err := startRendezvousServer(*port)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		log.Fatal(flag.ErrHelp)
	}
}
