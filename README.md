UDP holepunch through QUIC. I've tested it via an aws server, and it seems to work.
It has two modes:
- Do a simple server-peer run like:

  `go run quic-holepunch -port=:<port> -remoteAddr=<remote address>:<remote port>`
  
  This will attempt to dial a remote peer who already has `<remote port>` open. On the server side, it is simply:
  
  `go run quic-holepunch -port=:<port> -remoteAddr=none`
  
  Note that you need the none there, otherwise it will run in rendezvous mode. If two peers have a predictable NAT (ie. external port == internal port), you can use the first command on each peer with the others remote address and port
- Do a rendezvous-(peer-peer) like:
  Start the server with:
  
  `go run quic-holepunch -port=:<port>`
  
  Then run
  
  `go run quic-holepunch -port=:<port> -peerID=<peerID> -rendezvousAddr=<rendezvous server>:<rendezvous port>`
  
  on each peer. Make sure to pick a different peerID for each one.
