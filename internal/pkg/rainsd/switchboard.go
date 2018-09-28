//The switchboard handles incoming connections from servers and clients,
//opens connections to servers to which messages need to be sent but for which no active connection is available
//and provides the SendTo function which sends the message to the specified server.

package rainsd

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"time"

	log "github.com/inconshreveable/log15"

	"github.com/netsec-ethz/rains/internal/pkg/cbor"
	"github.com/netsec-ethz/rains/internal/pkg/connection"
	"github.com/netsec-ethz/rains/internal/pkg/message"
)

//sendTo sends message to the specified receiver.
func sendTo(msg message.Message, receiver connection.Info, retries, backoffMilliSeconds int,
	prioChannel chan msgSectionSender, normalChannel chan msgSectionSender,
	notificationChannel chan msgSectionSender, pendingKeys pendingKeyCache) (err error) {
	conns, ok := connCache.GetConnection(receiver)
	if !ok {
		conn, err := createConnection(receiver)
		//add connection to cache
		conns = append(conns, conn)
		if err != nil {
			log.Warn("Could not establish connection", "error", err, "receiver", receiver)
			return err
		}
		connCache.AddConnection(conn)
		//handle connection
		if tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
			go handleConnection(conn, connection.Info{Type: connection.TCP, TCPAddr: tcpAddr},
				prioChannel, normalChannel, notificationChannel, pendingKeys)
		} else {
			log.Warn("Type assertion failed. Expected *net.TCPAddr", "addr", conn.RemoteAddr())
		}
		//add capabilities to message
		msg.Capabilities = []message.Capability{message.Capability(capabilityHash)}
	}
	for _, conn := range conns {
		writer := cbor.NewWriter(conn)
		if err := writer.Marshal(&msg); err != nil {
			log.Warn(fmt.Sprintf("failed to marshal message to conn: %v", err))
			connCache.CloseAndRemoveConnection(conn)
			continue
		}
		log.Debug("Send successful", "receiver", receiver)
		return nil
	}
	if retries > 0 {
		time.Sleep(time.Duration(backoffMilliSeconds) * time.Millisecond)
		return sendTo(msg, receiver, retries-1, 2*backoffMilliSeconds, prioChannel, normalChannel,
			notificationChannel)
	}
	log.Error("Was not able to send the message. No retries left.", "receiver", receiver)
	return errors.New("Was not able to send the mesage. No retries left")
}

//createConnection establishes a connection with receiver
func createConnection(receiver connection.Info) (net.Conn, error) {
	switch receiver.Type {
	case connection.TCP:
		dialer := &net.Dialer{
			KeepAlive: Config.KeepAlivePeriod,
		}
		return tls.DialWithDialer(dialer, receiver.TCPAddr.Network(), receiver.String(), &tls.Config{RootCAs: roots, InsecureSkipVerify: true})
	default:
		return nil, errors.New("No matching type found for Connection info")
	}
}

//Listen listens for incoming connections and creates a go routine for each connection.
func Listen(prioChannel chan msgSectionSender, normalChannel chan msgSectionSender,
	notificationChannel chan msgSectionSender, pendingKeys pendingKeyCache) {
	srvLogger := log.New("addr", serverConnInfo.String())
	switch serverConnInfo.Type {
	case connection.TCP:
		srvLogger.Info("Start TCP listener")
		tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
		listener, err := tls.Listen(serverConnInfo.TCPAddr.Network(), serverConnInfo.String(), tlsConfig)
		if err != nil {
			srvLogger.Error("Listener error on startup", "error", err)
			return
		}
		defer listener.Close()
		defer srvLogger.Info("Shutdown listener")
		for {
			conn, err := listener.Accept()
			if err != nil {
				srvLogger.Error("listener could not accept connection", "error", err)
				continue
			}
			if isIPBlacklisted(conn.RemoteAddr()) {
				continue
			}
			connCache.AddConnection(conn)
			if tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
				go handleConnection(conn, connection.Info{Type: connection.TCP, TCPAddr: tcpAddr},
					prioChannel, normalChannel, notificationChannel, pendingKeys)
			} else {
				log.Warn("Type assertion failed. Expected *net.TCPAddr", "addr", conn.RemoteAddr())
			}
		}
	default:
		log.Warn("Unsupported Network address type.")
	}
}

//handleConnection deframes all incoming messages on conn and passes them to the inbox along with the dstAddr
func handleConnection(conn net.Conn, dstAddr connection.Info, prioChannel chan msgSectionSender,
	normalChannel chan msgSectionSender, notificationChannel chan msgSectionSender, pendingKeys pendingKeyCache) {
	var msg message.Message
	reader := cbor.NewReader(conn)
	for {
		//FIXME CFE how to check efficiently that message is not too large?
		if err := reader.Unmarshal(&msg); err != nil {
			log.Warn(fmt.Sprintf("failed to read from client: %v", err))
			break
		}
		deliver(&msg, connection.Info{Type: connection.TCP, TCPAddr: conn.RemoteAddr().(*net.TCPAddr)},
			prioChannel, normalChannel, notificationChannel, pendingKeys)
	}
	connCache.CloseAndRemoveConnection(conn)
}

//isIPBlacklisted returns true if addr is blacklisted
func isIPBlacklisted(addr net.Addr) bool {
	log.Warn("TODO CFE ip blacklist not yet implemented")
	return false
}
