package rainslib

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"sort"
	"time"

	log "github.com/inconshreveable/log15"
	"golang.org/x/crypto/ed25519"
)

//RainsMessage contains the data of a message
type RainsMessage struct {
	//Mandatory
	Token   Token
	Content []MessageSection

	//Optional
	Signatures []Signature
	//FIXME CFE capabilities can also be represented as a hash, how should we model this?
	Capabilities []Capability
}

//Sort sorts the sections in m.Content first by type (lexicographically) and second the sections of equal type according to their sort function.
func (m *RainsMessage) Sort() {
	var assertions []*AssertionSection
	var shards []*ShardSection
	var zones []*ZoneSection
	var queries []*QuerySection
	var addressAssertions []*AddressAssertionSection
	var addressZones []*AddressZoneSection
	var addressQueries []*AddressQuerySection
	var notifications []*NotificationSection
	for _, sec := range m.Content {
		sec.Sort()
		switch sec := sec.(type) {
		case *AssertionSection:
			assertions = append(assertions, sec)
		case *ShardSection:
			shards = append(shards, sec)
		case *ZoneSection:
			zones = append(zones, sec)
		case *QuerySection:
			queries = append(queries, sec)
		case *NotificationSection:
			notifications = append(notifications, sec)
		case *AddressAssertionSection:
			addressAssertions = append(addressAssertions, sec)
		case *AddressZoneSection:
			addressZones = append(addressZones, sec)
		case *AddressQuerySection:
			addressQueries = append(addressQueries, sec)
		default:
			log.Warn("Unsupported section type", "type", fmt.Sprintf("%T", sec))
		}
	}
	sort.Slice(assertions, func(i, j int) bool { return assertions[i].CompareTo(assertions[j]) < 0 })
	sort.Slice(shards, func(i, j int) bool { return shards[i].CompareTo(shards[j]) < 0 })
	sort.Slice(zones, func(i, j int) bool { return zones[i].CompareTo(zones[j]) < 0 })
	sort.Slice(queries, func(i, j int) bool { return queries[i].CompareTo(queries[j]) < 0 })
	sort.Slice(addressAssertions, func(i, j int) bool { return addressAssertions[i].CompareTo(addressAssertions[j]) < 0 })
	sort.Slice(addressZones, func(i, j int) bool { return addressZones[i].CompareTo(addressZones[j]) < 0 })
	sort.Slice(addressQueries, func(i, j int) bool { return addressQueries[i].CompareTo(addressQueries[j]) < 0 })
	sort.Slice(notifications, func(i, j int) bool { return notifications[i].CompareTo(notifications[j]) < 0 })
	m.Content = []MessageSection{}
	for _, section := range addressQueries {
		m.Content = append(m.Content, section)
	}
	for _, section := range addressZones {
		m.Content = append(m.Content, section)
	}
	for _, section := range addressAssertions {
		m.Content = append(m.Content, section)
	}
	for _, section := range assertions {
		m.Content = append(m.Content, section)
	}
	for _, section := range shards {
		m.Content = append(m.Content, section)
	}
	for _, section := range zones {
		m.Content = append(m.Content, section)
	}
	for _, section := range queries {
		m.Content = append(m.Content, section)
	}
	for _, section := range notifications {
		m.Content = append(m.Content, section)
	}
}

//Capability is a urn of a capability
type Capability string

const (
	NoCapability Capability = ""
	TLSOverTCP   Capability = "urn:x-rains:tlssrv"
)

//Token is used to identify a message
type Token [16]byte

func (t Token) String() string {
	return hex.EncodeToString(t[:])
}

//MessageSection can be either an Assertion, Shard, Zone, Query, Notification, AddressAssertion, AddressZone, AddressQuery section
type MessageSection interface {
	Sort()
}

//MessageSectionWithSig can be either an Assertion, Shard, Zone, AddressAssertion, AddressZone
type MessageSectionWithSig interface {
	Sigs() []Signature
	AddSig(sig Signature)
	DeleteSig(int)
	DeleteAllSigs()
	GetContext() string
	GetSubjectZone() string
	UpdateValidity(validSince, validUntil int64, maxValidity time.Duration)
	ValidSince() int64
	ValidUntil() int64
	Hash() string
	Sort()
}

//MessageSectionWithSigForward can be either an Assertion, Shard or Zone
type MessageSectionWithSigForward interface {
	Sigs() []Signature
	AddSig(sig Signature)
	DeleteSig(int)
	DeleteAllSigs()
	GetContext() string
	GetSubjectZone() string
	UpdateValidity(validSince, validUntil int64, maxValidity time.Duration)
	ValidSince() int64
	ValidUntil() int64
	Hash() string
	Sort()
	Interval
}

//Interval defines an interval over strings
type Interval interface {
	//Begin of the interval
	Begin() string
	//End of the interval
	End() string
}

//TotalInterval is an interval over the whole namespace
type TotalInterval struct{}

func (t TotalInterval) Begin() string {
	return ""
}

func (t TotalInterval) End() string {
	return ""
}

//StringInterval implements Interval for a single string value
type StringInterval struct {
	Name string
}

func (s StringInterval) Begin() string {
	return s.Name
}

func (s StringInterval) End() string {
	return s.Name
}

//Hashable can be implemented by objects that are not natively hashable.
type Hashable interface {
	//Hash must return a string uniquely identifying the object
	Hash() string
}

//Signature on a Rains message or section
type Signature struct {
	KeySpace   KeySpaceID
	Algorithm  SignatureAlgorithmType
	ValidSince int64
	ValidUntil int64
	Data       interface{}
}

//GetSignatureMetaData returns a string containing the signature's metadata (keyspace, algorithm type, validSince and validUntil) in signable format
func (sig Signature) GetSignatureMetaData() string {
	return fmt.Sprintf("%d %d %d %d", sig.KeySpace, sig.Algorithm, sig.ValidSince, sig.ValidUntil)
}

//SignData adds signature meta data to encoding. It then signs the encoding with privateKey and updates sig.Data field with the generated signature
//In case of an error an error is returned indicating the cause, otherwise nil is returned
func (sig *Signature) SignData(privateKey interface{}, encoding string) error {
	if privateKey == nil {
		log.Warn("PrivateKey is nil")
		return errors.New("privateKey is nil")
	}
	encoding += sig.GetSignatureMetaData()
	data := []byte(encoding)
	switch sig.Algorithm {
	case Ed25519:
		log.Debug("Sign data", "signature", sig, "privateKey", hex.EncodeToString(privateKey.(ed25519.PrivateKey)), "encoding", encoding)
		if pkey, ok := privateKey.(ed25519.PrivateKey); ok {
			sig.Data = ed25519.Sign(pkey, data)
			return nil
		}
		log.Warn("Could not assert type ed25519.PrivateKey", "privateKeyType", fmt.Sprintf("%T", privateKey))
		return errors.New("could not assert type ed25519.PrivateKey")
	case Ed448:
		return errors.New("ed448 not yet supported in SignData()")
	case Ecdsa256:
		if pkey, ok := privateKey.(*ecdsa.PrivateKey); ok {
			hash := sha256.Sum256(data)
			r, s, err := ecdsa.Sign(rand.Reader, pkey, hash[:])
			if err != nil {
				log.Warn("Could not sign data", "error", err)
				return err
			}
			sig.Data = []*big.Int{r, s}
			return nil
		}
		log.Warn("Could not assert type ecdsa.PrivateKey", "privateKeyType", fmt.Sprintf("%T", privateKey))
		return errors.New("could not assert type ecdsa.PrivateKey")
	case Ecdsa384:
		if pkey, ok := privateKey.(*ecdsa.PrivateKey); ok {
			hash := sha512.Sum384(data)
			r, s, err := ecdsa.Sign(rand.Reader, pkey, hash[:])
			if err != nil {
				log.Warn("Could not sign data", "error", err)
				return err
			}
			sig.Data = []*big.Int{r, s}
			return nil
		}
		log.Warn("Could not cast key to ecdsa.PrivateKey", "privateKeyType", fmt.Sprintf("%T", privateKey))
		return errors.New("could not assert type ecdsa.PrivateKey")
	default:
		log.Warn("Signature algorithm type not supported", "type", sig.Algorithm)
		return errors.New("signature algorithm type not supported")
	}
}

//VerifySignature adds signature meta data to the encoding. It then signs the encoding with privateKey and compares the resulting signature with the sig.Data.
//Returns true if there exist signatures and they are identical
func (sig *Signature) VerifySignature(publicKey interface{}, encoding string) bool {
	if sig.Data == nil {
		log.Warn("sig does not contain signature data", "sig", sig)
		return false
	}
	if publicKey == nil {
		log.Warn("PublicKey is nil")
		return false
	}
	encoding += sig.GetSignatureMetaData()
	data := []byte(encoding)
	switch sig.Algorithm {
	case Ed25519:
		if pkey, ok := publicKey.(ed25519.PublicKey); ok {
			return ed25519.Verify(pkey, data, sig.Data.([]byte))
		}
		log.Warn("Could not assert type ed25519.PublicKey", "publicKeyType", fmt.Sprintf("%T", publicKey))
	case Ed448:
		log.Warn("Ed448 not yet Supported!")
	case Ecdsa256:
		if pkey, ok := publicKey.(*ecdsa.PublicKey); ok {
			if sig, ok := sig.Data.([]*big.Int); ok && len(sig) == 2 {
				hash := sha256.Sum256(data)
				return ecdsa.Verify(pkey, hash[:], sig[0], sig[1])
			}
			log.Warn("Could not assert type []*big.Int", "signatureDataType", fmt.Sprintf("%T", sig.Data))
			return false
		}
		log.Warn("Could not assert type ecdsa.PublicKey", "publicKeyType", fmt.Sprintf("%T", publicKey))
	case Ecdsa384:
		if pkey, ok := publicKey.(*ecdsa.PublicKey); ok {
			if sig, ok := sig.Data.([]*big.Int); ok && len(sig) == 2 {
				hash := sha512.Sum384(data)
				return ecdsa.Verify(pkey, hash[:], sig[0], sig[1])
			}
			log.Warn("Could not assert type []*big.Int", "signature", sig.Data)
			return false
		}
		log.Warn("Could not assert type ecdsa.PublicKey", "publicKeyType", fmt.Sprintf("%T", publicKey))
	default:
		log.Warn("Signature algorithm type not supported", "type", sig.Algorithm)
	}
	return false
}

//String implements Stringer interface
func (sig Signature) String() string {
	data := "notYetImplementedInString()"
	if sig.Algorithm == Ed25519 {
		if sig.Data == nil {
			data = "nil"
		} else {
			data = hex.EncodeToString(sig.Data.([]byte))
		}
	}
	return fmt.Sprintf("keyspace=%d, algoType=%d, validSince=%d, validUntil=%d, data=%s", sig.KeySpace, sig.Algorithm, sig.ValidSince, sig.ValidUntil, data)
}

type NotificationType int

const (
	Heartbeat          NotificationType = 100
	CapHashNotKnown    NotificationType = 399
	BadMessage         NotificationType = 400
	RcvInconsistentMsg NotificationType = 403
	NoAssertionsExist  NotificationType = 404
	MsgTooLarge        NotificationType = 413
	UnspecServerErr    NotificationType = 500
	ServerNotCapable   NotificationType = 501
	NoAssertionAvail   NotificationType = 504
)

type QueryOption int

const (
	QOMinE2ELatency            QueryOption = 1
	QOMinLastHopAnswerSize     QueryOption = 2
	QOMinInfoLeakage           QueryOption = 3
	QOCachedAnswersOnly        QueryOption = 4
	QOExpiredAssertionsOk      QueryOption = 5
	QOTokenTracing             QueryOption = 6
	QONoVerificationDelegation QueryOption = 7
	QONoProactiveCaching       QueryOption = 8
)

//ConnInfo contains address information about one actor of a connection of the declared type
type ConnInfo struct {
	Type NetworkAddrType

	TCPAddr *net.TCPAddr
}

//String returns the string representation of the connection information according to its type
func (c ConnInfo) String() string {
	switch c.Type {
	case TCP:
		return c.TCPAddr.String()
	default:
		log.Warn("Unsupported network address", "typeCode", c.Type)
		return ""
	}
}

//Hash returns a string containing all information uniquely identifying a ConnInfo.
func (c ConnInfo) Hash() string {
	return fmt.Sprintf("%v_%s", c.Type, c.String())
}

//Equal returns true if both Connection Information have the same type and the values corresponding to this type are identical.
func (c ConnInfo) Equal(conn ConnInfo) bool {
	if c.Type == conn.Type {
		switch c.Type {
		case TCP:
			return c.TCPAddr.IP.Equal(conn.TCPAddr.IP) && c.TCPAddr.Port == conn.TCPAddr.Port && c.TCPAddr.Zone == conn.TCPAddr.Zone
		default:
			log.Warn("Not supported network address type")
		}
	}
	return false
}

//MaxCacheValidity defines the maximum duration each section containing signatures can be valid, starting from time.Now()
type MaxCacheValidity struct {
	AssertionValidity        time.Duration
	ShardValidity            time.Duration
	ZoneValidity             time.Duration
	AddressAssertionValidity time.Duration
	AddressZoneValidity      time.Duration
}

//RainsMsgParser can encode and decode RainsMessage.
//It is able to efficiently extract only the Token form an encoded RainsMessage
//It must always hold that: rainsMsg = Decode(Encode(rainsMsg)) && interface{} = Encode(Decode(interface{}))
type RainsMsgParser interface {
	//Decode extracts information from msg and returns a RainsMessage or an error
	Decode(msg []byte) (RainsMessage, error)

	//Encode encodes the given RainsMessage into a more compact representation.
	Encode(msg RainsMessage) ([]byte, error)

	//Token returns the extracted token from the given msg or an error
	Token(msg []byte) (Token, error)
}

//ZoneFileParser is the interface for all parsers of zone files for RAINS
type ZoneFileParser interface {
	//Decode takes as input the content of a zoneFile and the name from which the data was loaded.
	//It returns all contained assertions or an error in case of failure
	Decode(zoneFile []byte, filePath string) ([]*AssertionSection, error)

	//Encode returns the given section represented in the zone file format if it is a zoneSection.
	//In all other cases it returns the section in a displayable format similar to the zone file format
	Encode(section MessageSection) string
}

//SignatureFormatEncoder is used to deterministically transform a RainsMessage into a byte format that can be signed.
type SignatureFormatEncoder interface {
	//EncodeMessage transforms the given msg into a signable format.
	//It must have already been verified that the msg does not contain malicious content.
	//Signature meta data is not added
	EncodeMessage(msg *RainsMessage) string

	//EncodeSection transforms the given msg into a signable format
	//It must have already been verified that the section does not contain malicious content
	//Signature meta data is not added
	EncodeSection(section MessageSection) string
}

//MsgFramer is used to frame and deframe rains messages and send or receive them on the initialized stream.
type MsgFramer interface {
	//InitStreams defines 2 streams.
	//Deframe() and Data() are extracting information from streamReader
	//Frame() is sending data to streamWriter.
	//If a stream is readable and writable it is possible that streamReader = streamWriter
	InitStreams(streamReader io.Reader, streamWriter io.Writer)

	//Frame encodes the msg and writes it to the streamWriter defined in InitStream()
	//The following must hold: DeFrame(Frame(msg)); Data() = msg
	//If Frame() was not able to frame or write the message an error is returned indicating what the problem was
	Frame(msg []byte) error

	//DeFrame extracts the next frame from the streamReader defined in InitStream().
	//It blocks until it encounters the delimiter.
	//It returns false when the stream was not initialized or is already closed.
	//The data is available through Data
	DeFrame() bool

	//Data contains the frame read from the stream by DeFrame
	Data() []byte
}
