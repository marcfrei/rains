package rainsd

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io/ioutil"
	"strings"
	"time"

	log "github.com/inconshreveable/log15"
	"github.com/netsec-ethz/rains/internal/pkg/message"
)

//Server represents a rainsd server instance.
type Server struct {
	//config contains configurations of this server
	config rainsdConfig
	//authority states the names over which this server has authority
	authority map[zoneContext]bool
	//certPool stores received certificates
	certPool *x509.CertPool
	//tlsCert holds the tls certificate of this server
	tlsCert tls.Certificate
	//capabilityHash contains the sha256 hash of this server's capability list
	capabilityHash string
	//capabilityList contains the string representation of this server's capability list.
	capabilityList string
	//shutdown can be used to stop the go routines handling the input channels and closes them.
	shutdown chan bool
	//queues store the incoming sections and keeps track of how many go routines are working on it.
	queues InputQueues
	//caches contains all caches of this server
	caches *Caches
}

//New returns a pointer to a newly created rainsd server instance with the given config. The server
//logs with the provided level of logging.
func New(configPath string, logLevel int) (server *Server, err error) {
	h := log.CallerFileHandler(log.StdoutHandler)
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(logLevel), h))
	if server.config, err = loadConfig(configPath); err != nil {
		return nil, err
	}
	server.authority = make(map[zoneContext]bool)
	for i, context := range server.config.ContextAuthority {
		server.authority[zoneContext{Zone: server.config.ZoneAuthority[i], Context: context}] = true
	}
	if server.certPool, server.tlsCert, err = loadTLSCertificate(server.config.TLSCertificateFile,
		server.config.TLSPrivateKeyFile); err != nil {
		return nil, err
	}
	server.capabilityHash, server.capabilityList = initOwnCapabilities(server.config.Capabilities)
	if err = loadRootZonePublicKey(server.config.RootZonePublicKeyPath); err != nil {
		log.Warn("Failed to load root zone public key")
		return nil, err
	}
	server.shutdown = make(chan bool)
	server.queues = InputQueues{
		Prio:    make(chan msgSectionSender, server.config.PrioBufferSize),
		Normal:  make(chan msgSectionSender, server.config.NormalBufferSize),
		Notify:  make(chan msgSectionSender, server.config.NotificationBufferSize),
		PrioW:   make(chan struct{}, server.config.PrioWorkerCount),
		NormalW: make(chan struct{}, server.config.NormalWorkerCount),
		NotifyW: make(chan struct{}, server.config.NotificationWorkerCount),
	}
	server.caches = initCaches(server.config)
	if err = loadRootZonePublicKey(server.config.RootZonePublicKeyPath); err != nil {
		log.Warn("Failed to load root zone public key")
		return nil, err
	}
	return
}

//Start starts up the server and it begins to listen for incoming connections according to its
//config.
func (s *Server) Start() error {
	go workPrio(s.queues.Prio, s.queues.PrioW, s.shutdown)
	go workBoth(s.queues.Prio, s.queues.Normal, s.queues.PrioW, s.queues.NormalW, s.shutdown)
	go workNotification(s.queues.Notify, s.queues.NotifyW, s.shutdown, s.caches, s.config.ServerAddress, s.capabilityList)
	initReapers(s.config, s.caches, s.shutdown)
	log.Debug("Goroutines working on input queue started")
	//TODO init engine
	//initEngine()
	log.Debug("Successfully initiated engine")
	// Initialize Rayhaan's tracer?
	/*if traceAddr != "" {
		t, err := NewTracer(traceSrvID, traceAddr)
		if err != nil {
			return fmt.Errorf("failed to initialize the tracer: %v", err)
		}
		globalTracer = t
		go t.SendLoop()
	}
	log.Debug("successfully initialized tracer")*/
	Listen(s.queues.Prio, s.queues.Normal, s.queues.Notify, s.caches.PendingKeys)
	return nil
}

//Shutdown closes the input channels and stops the function creating new go routines to handle the
//input. Already running worker go routines will finish eventually.
func (s *Server) Shutdown() {
	for i := 0; i < 3; i++ {
		s.shutdown <- true
	}
}

//LoadConfig loads and stores server configuration
func loadConfig(configPath string) (rainsdConfig, error) {
	config := rainsdConfig{}
	file, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Warn("Could not open config file...", "path", configPath, "error", err)
		return rainsdConfig{}, err
	}
	if err = json.Unmarshal(file, &config); err != nil {
		log.Warn("Could not unmarshal json format of config", "error", err)
		return rainsdConfig{}, err
	}
	config.KeepAlivePeriod *= time.Second
	config.TCPTimeout *= time.Second
	config.DelegationQueryValidity *= time.Second
	config.ReapVerifyTimeout *= time.Second
	config.QueryValidity *= time.Second
	config.AddressQueryValidity *= time.Second
	config.ReapEngineTimeout *= time.Second
	config.MaxCacheValidity.AddressAssertionValidity *= time.Hour
	config.MaxCacheValidity.AddressZoneValidity *= time.Hour
	config.MaxCacheValidity.AssertionValidity *= time.Hour
	config.MaxCacheValidity.ShardValidity *= time.Hour
	config.MaxCacheValidity.ZoneValidity *= time.Hour
	return config, nil
}

//loadTLSCertificate load a tls certificate from certPath
func loadTLSCertificate(certPath string, TLSPrivateKeyPath string) (*x509.CertPool, tls.Certificate, error) {
	pool := x509.NewCertPool()
	file, err := ioutil.ReadFile(certPath)
	if err != nil {
		log.Error("error", err)
		return nil, tls.Certificate{}, err
	}

	if ok := pool.AppendCertsFromPEM(file); !ok {
		log.Error("failed to parse root certificate")
		return nil, tls.Certificate{}, errors.New("failed to parse root certificate")
	}
	cert, err := tls.LoadX509KeyPair(certPath, TLSPrivateKeyPath)
	if err != nil {
		log.Error("Cannot load certificate. Path to CertificateFile or privateKeyFile might be invalid.",
			"CertPath", certPath, "KeyPath", TLSPrivateKeyPath, "error", err)
		return nil, tls.Certificate{}, err
	}
	return pool, cert, nil
}

//initOwnCapabilities sorts capabilities in lexicographically increasing order.
//It stores the hex encoded sha256 hash of the sorted capabilities to capabilityHash
//and a string representation of the capability list to capabilityList
func initOwnCapabilities(capabilities []message.Capability) (string, string) {
	//TODO CFE when we have CBOR use it to normalize&serialize the array before hashing it.
	//Currently we use the hard coded version from the draft.
	capabilityHash := "e5365a09be554ae55b855f15264dbc837b04f5831daeb321359e18cdabab5745"
	cs := make([]string, len(capabilities))
	for i, c := range capabilities {
		cs[i] = string(c)
	}
	return capabilityHash, strings.Join(cs, " ")
}
