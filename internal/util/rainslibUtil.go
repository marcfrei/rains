package util

import (
	"encoding/gob"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	log "github.com/inconshreveable/log15"
	"github.com/netsec-ethz/rains/internal/pkg/keys"
	"github.com/netsec-ethz/rains/internal/pkg/message"
	"github.com/netsec-ethz/rains/internal/pkg/object"
	"github.com/netsec-ethz/rains/internal/pkg/sections"
	"github.com/netsec-ethz/rains/internal/pkg/token"

	"golang.org/x/crypto/ed25519"
)

func init() {
	gob.Register(keys.PublicKey{})
	gob.RegisterName("ed25519.PublicKey", ed25519.PublicKey{})
}

//MaxCacheValidity defines the maximum duration each section containing signatures can be valid, starting from time.Now()
type MaxCacheValidity struct {
	AssertionValidity        time.Duration
	ShardValidity            time.Duration
	ZoneValidity             time.Duration
	AddressAssertionValidity time.Duration
	AddressZoneValidity      time.Duration
}

//Save stores the object to the file located at the specified path gob encoded.
func Save(path string, object interface{}) error {
	file, err := os.Create(path)
	defer file.Close()
	if err == nil {
		encoder := gob.NewEncoder(file)
		encoder.Encode(object)
	}
	return err
}

//Load fetches the gob encoded object from the file located at path
func Load(path string, object interface{}) error {
	file, err := os.Open(path)
	defer file.Close()
	if err != nil {
		log.Error("Was not able to open file", "path", path, "error", err)
		return err
	}
	decoder := gob.NewDecoder(file)
	err = decoder.Decode(object)
	if err != nil {
		log.Error("Was not able to decode file.", "path", path, "error", err)
	}
	return err
}

//UpdateSectionValidity updates the validity of the section according to the signature validity and the publicKey validity used to verify this signature
func UpdateSectionValidity(sec sections.MessageSectionWithSig, pkeyValidSince, pkeyValidUntil, sigValidSince, sigValidUntil int64, maxVal MaxCacheValidity) {
	if sec != nil {
		var maxValidity time.Duration
		switch sec.(type) {
		case *sections.AssertionSection:
			maxValidity = maxVal.AssertionValidity
		case *sections.ShardSection:
			maxValidity = maxVal.ShardValidity
		case *sections.ZoneSection:
			maxValidity = maxVal.ZoneValidity
		case *sections.AddressAssertionSection:
			maxValidity = maxVal.AddressAssertionValidity
		case *sections.AddressZoneSection:
			maxValidity = maxVal.AddressZoneValidity
		default:
			log.Warn("Not supported section", "type", fmt.Sprintf("%T", sec))
			return
		}
		if pkeyValidSince < sigValidSince {
			if pkeyValidUntil < sigValidUntil {
				sec.UpdateValidity(sigValidSince, pkeyValidUntil, maxValidity)
			} else {
				sec.UpdateValidity(sigValidSince, sigValidUntil, maxValidity)
			}

		} else {
			if pkeyValidUntil < sigValidUntil {
				sec.UpdateValidity(pkeyValidSince, pkeyValidUntil, maxValidity)
			} else {
				sec.UpdateValidity(pkeyValidSince, sigValidUntil, maxValidity)
			}
		}
	}
}

//NewQueryMessage creates a new message containing a query body with values obtained from the input parameter
func NewQueryMessage(name, context string, expTime int64, objType []object.ObjectType,
	queryOptions []sections.QueryOption, token token.Token) message.RainsMessage {
	query := sections.QuerySection{
		Context:    context,
		Name:       name,
		Expiration: expTime,
		Types:      objType,
		Options:    queryOptions,
	}
	return message.RainsMessage{Token: token, Content: []sections.MessageSection{&query}}
}

//NewAddressQueryMessage creates a new message containing an addressQuery body with values obtained from the input parameter
func NewAddressQueryMessage(context string, ipNet *net.IPNet, expTime int64, objType []object.ObjectType,
	queryOptions []sections.QueryOption, token token.Token) message.RainsMessage {
	addressQuery := sections.AddressQuerySection{
		Context:     context,
		SubjectAddr: ipNet,
		Expiration:  expTime,
		Types:       objType,
		Options:     queryOptions,
	}
	return message.RainsMessage{Token: token, Content: []sections.MessageSection{&addressQuery}}
}

//NewNotificationsMessage creates a new message containing notification bodies with values obtained from the input parameter
func NewNotificationsMessage(tokens []token.Token, types []sections.NotificationType, data []string) (message.RainsMessage, error) {
	if len(tokens) != len(types) || len(types) != len(data) {
		log.Warn("input slices have not the same length", "tokenLen", len(tokens), "typesLen", len(types), "dataLen", len(data))
		return message.RainsMessage{}, errors.New("input slices have not the same length")
	}
	msg := message.RainsMessage{Token: token.GenerateToken(), Content: []sections.MessageSection{}}
	for i := range tokens {
		notification := &sections.NotificationSection{
			Token: tokens[i],
			Type:  types[i],
			Data:  data[i],
		}
		msg.Content = append(msg.Content, notification)
	}
	return msg, nil
}

//NewNotificationMessage creates a new message containing one notification body with values obtained from the input parameter
func NewNotificationMessage(tok token.Token, t sections.NotificationType, data string) message.RainsMessage {
	msg, _ := NewNotificationsMessage([]token.Token{tok}, []sections.NotificationType{t}, []string{data})
	return msg
}
