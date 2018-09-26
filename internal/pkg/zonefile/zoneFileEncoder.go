package zonefile

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/netsec-ethz/rains/internal/pkg/algorithmTypes"
	"github.com/netsec-ethz/rains/internal/pkg/keys"
	"github.com/netsec-ethz/rains/internal/pkg/object"
	"github.com/netsec-ethz/rains/internal/pkg/sections"
	"github.com/netsec-ethz/rains/internal/pkg/signature"

	log "github.com/inconshreveable/log15"
	"golang.org/x/crypto/ed25519"
)

//encodeZone return z in zonefile format. If addZoneAndContext is true, the context and subject zone
//are present also for all contained sections.
func encodeZone(z *sections.ZoneSection, addZoneAndContext bool) string {
	zone := fmt.Sprintf("%s %s %s [\n", TypeZone, z.SubjectZone, z.Context)
	for _, section := range z.Content {
		switch section := section.(type) {
		case *sections.AssertionSection:
			zone += encodeAssertion(section, z.Context, z.SubjectZone, indent4, addZoneAndContext)
		case *sections.ShardSection:
			zone += encodeShard(section, z.Context, z.SubjectZone, indent4, addZoneAndContext)
		case *sections.PshardSection:
			zone += encodePshard(section, z.Context, z.SubjectZone, indent4, addZoneAndContext)
		default:
			log.Warn("Unsupported message section type", "msgSection", section)
		}
	}
	if z.Signatures != nil {
		var sigs []string
		for _, sig := range z.Signatures {
			sigs = append(sigs, encodeEd25519Signature(sig))
		}
		if len(sigs) == 1 {
			return fmt.Sprintf("%s] ( %s )\n", zone, sigs[0])
		}
		return fmt.Sprintf("%s] ( \n%s%s\n  )\n", zone, indent4, strings.Join(sigs, "\n"+indent4))
	}
	return fmt.Sprintf("%s]\n", zone)
}

//encodeShard returns s in zonefile format. If addZoneAndContext is true, the context and subject
//zone are present for the shard and for all contained assertions.
func encodeShard(s *sections.ShardSection, context, subjectZone, indent string, addZoneAndContext bool) string {
	rangeFrom := s.RangeFrom
	rangeTo := s.RangeTo
	if rangeFrom == "" {
		rangeFrom = "<"
	}
	if rangeTo == "" {
		rangeTo = ">"
	}
	var shard string
	if addZoneAndContext {
		shard = fmt.Sprintf("%s %s %s %s %s [\n", TypeShard, subjectZone, context, rangeFrom, rangeTo)
	} else {
		shard = fmt.Sprintf("%s %s %s [\n", TypeShard, rangeFrom, rangeTo)
	}
	for _, assertion := range s.Content {
		shard += encodeAssertion(assertion, context, subjectZone, indent+indent4, addZoneAndContext)
	}
	if s.Signatures != nil {
		var sigs []string
		for _, sig := range s.Signatures {
			sigs = append(sigs, encodeEd25519Signature(sig))
		}
		if len(sigs) == 1 {
			return fmt.Sprintf("%s%s%s] ( %s )\n", indent, shard, indent, sigs[0])
		}
		return fmt.Sprintf("%s%s%s] ( \n%s%s\n%s  )\n", indent, shard, indent, indent+indent4, strings.Join(sigs, "\n"+indent+indent4), indent)
	}
	return fmt.Sprintf("%s%s%s]\n", indent, shard, indent)
}

//encodePshard returns s in zonefile format. If addZoneAndContext is true, the context and subject
//zone are present for the pshard.
func encodePshard(s *sections.PshardSection, context, subjectZone, indent string, addZoneAndContext bool) string {
	rangeFrom := s.RangeFrom
	rangeTo := s.RangeTo
	if rangeFrom == "" {
		rangeFrom = "<"
	}
	if rangeTo == "" {
		rangeTo = ">"
	}
	var pshard string
	if addZoneAndContext {
		pshard = fmt.Sprintf("%s %s %s %s %s %s", TypePshard, subjectZone, context, rangeFrom,
			rangeTo, encodeBloomFilter(s.Datastructure))
	} else {
		pshard = fmt.Sprintf("%s %s %s %s", TypePshard, rangeFrom, rangeTo,
			encodeBloomFilter(s.Datastructure))
	}
	if s.Signatures != nil {
		var sigs []string
		for _, sig := range s.Signatures {
			sigs = append(sigs, encodeEd25519Signature(sig))
		}
		if len(sigs) == 1 {
			return fmt.Sprintf("%s%s ( %s )\n", indent, pshard, sigs[0])
		}
		return fmt.Sprintf("%s%s ( \n%s%s\n%s  )\n", indent, pshard, indent+indent4, strings.Join(sigs, "\n"+indent+indent4), indent)
	}
	return fmt.Sprintf("%s%s\n", indent, pshard)
}

//encodeBloomFilter returns d containing a bloom filter in zonefile format.
func encodeBloomFilter(d sections.DataStructure) string {
	bloomFilter, ok := d.Data.(sections.BloomFilter)
	if !ok {
		log.Error("Data Type is not a bloom filter", "type", fmt.Sprintf("%T", d.Data))
	}
	var hashFamily []string
	for _, hash := range bloomFilter.HashFamily {
		hashFamily = append(hashFamily, encodeHashAlgo(hash))
	}
	opMode := ""
	switch bloomFilter.ModeOfOperation {
	case sections.StandardOpType:
		opMode = TypeStandard
	case sections.KirschMitzenmacher1:
		opMode = TypeKM1
	case sections.KirschMitzenmacher2:
		opMode = TypeKM2
	default:
		log.Error("Unsupported mode of operation", "modeOfOperation", bloomFilter.ModeOfOperation)
	}
	return fmt.Sprintf("%s [ %s ] %d %s %s", TypeBloomFilter, strings.Join(hashFamily, " "),
		bloomFilter.NofHashFunctions, opMode, hex.EncodeToString(bloomFilter.Filter))
}

//encodeAssertion returns a in zonefile format. If addZoneAndContext is true, the context and
//subject zone are also present.
func encodeAssertion(a *sections.AssertionSection, context, zone, indent string, addZoneAndContext bool) string {
	var assertion string
	if addZoneAndContext {
		assertion = fmt.Sprintf("%s%s %s %s %s [ ", indent, TypeAssertion, a.SubjectName, zone, context)
	} else {
		assertion = fmt.Sprintf("%s%s %s [ ", indent, TypeAssertion, a.SubjectName)
	}
	signature := ""
	if a.Signatures != nil {
		var sigs []string
		for _, sig := range a.Signatures {
			sigs = append(sigs, encodeEd25519Signature(sig))
		}
		if len(sigs) == 1 {
			signature = fmt.Sprintf(" ( %s )\n", sigs[0])
		} else {
			signature = fmt.Sprintf(" ( \n%s%s\n%s)\n", indent+indent4, strings.Join(sigs, "\n"+indent+indent4), indent)
		}
	}
	if len(a.Content) > 1 {
		return fmt.Sprintf("%s\n%s\n%s]%s", assertion, encodeObjects(a.Content, indent+indent4), indent, signature)
	}
	return fmt.Sprintf("%s%s ]%s", assertion, encodeObjects(a.Content, ""), signature)
}

//encodeObjects returns o in zonefile format.
func encodeObjects(o []object.Object, indent string) string {
	var objects []string
	for _, obj := range o {
		encoding := indent
		switch obj.Type {
		case object.OTName:
			if nameObj, ok := obj.Value.(object.NameObject); ok {
				encoding += fmt.Sprintf("%s%s", addIndentToType(TypeName), encodeNameObject(nameObj))
			} else {
				log.Error("Type assertion failed. Expected object.NameObject", "actualType", fmt.Sprintf("%T", obj.Value))
				return ""
			}
		case object.OTIP6Addr:
			encoding += fmt.Sprintf("%s%s", addIndentToType(TypeIP6), obj.Value)
		case object.OTIP4Addr:
			encoding += fmt.Sprintf("%s%s", addIndentToType(TypeIP4), obj.Value)
		case object.OTRedirection:
			encoding += fmt.Sprintf("%s%s", addIndentToType(TypeRedirection), obj.Value)
		case object.OTDelegation:
			if pkey, ok := obj.Value.(keys.PublicKey); ok {
				encoding += fmt.Sprintf("%s%s", addIndentToType(TypeDelegation), encodeEd25519PublicKey(pkey))
			} else {
				log.Warn("Type assertion failed. Expected object.PublicKey", "actualType", fmt.Sprintf("%T", obj.Value))
				return ""
			}
		case object.OTNameset:
			encoding += fmt.Sprintf("%s%s", addIndentToType(TypeNameSet), obj.Value)
		case object.OTCertInfo:
			if cert, ok := obj.Value.(object.CertificateObject); ok {
				encoding += fmt.Sprintf("%s%s", addIndentToType(TypeCertificate), encodeCertificate(cert))
			} else {
				log.Warn("Type assertion failed. Expected object.CertificateObject", "actualType", fmt.Sprintf("%T", obj.Value))
				return ""
			}
		case object.OTServiceInfo:
			if srvInfo, ok := obj.Value.(object.ServiceInfo); ok {
				encoding += fmt.Sprintf("%s%s %d %d", addIndentToType(TypeServiceInfo), srvInfo.Name, srvInfo.Port, srvInfo.Priority)
			} else {
				log.Warn("Type assertion failed. Expected object.ServiceInfo", "actualType", fmt.Sprintf("%T", obj.Value))
				return ""
			}
		case object.OTRegistrar:
			encoding += fmt.Sprintf("%s%s", addIndentToType(TypeRegistrar), obj.Value)
		case object.OTRegistrant:
			encoding += fmt.Sprintf("%s%s", addIndentToType(TypeRegistrant), obj.Value)
		case object.OTInfraKey:
			if pkey, ok := obj.Value.(keys.PublicKey); ok {
				encoding += fmt.Sprintf("%s%s", addIndentToType(TypeInfraKey), encodeEd25519PublicKey(pkey))
			} else {
				log.Warn("Type assertion failed. Expected object.PublicKey", "actualType", fmt.Sprintf("%T", obj.Value))
				return ""
			}
		case object.OTExtraKey:
			if pkey, ok := obj.Value.(keys.PublicKey); ok {
				encoding += fmt.Sprintf("%s%s %s", addIndentToType(TypeExternalKey), encodeKeySpace(pkey.KeySpace), encodeEd25519PublicKey(pkey))
			} else {
				log.Warn("Type assertion failed. Expected object.PublicKey", "actualType", fmt.Sprintf("%T", obj.Value))
				return ""
			}
		case object.OTNextKey:
			if pkey, ok := obj.Value.(keys.PublicKey); ok {
				encoding += fmt.Sprintf("%s%s %d %d", addIndentToType(TypeNextKey), encodeEd25519PublicKey(pkey), pkey.ValidSince, pkey.ValidUntil)
			} else {
				log.Warn("Type assertion failed. Expected object.PublicKey", "actualType", fmt.Sprintf("%T", obj.Value))
				return ""
			}
		default:
			log.Warn("Unsupported obj type", "type", fmt.Sprintf("%T", obj.Type))
			return ""
		}
		objects = append(objects, encoding)
	}
	return strings.Join(objects, "\n")
}

//addIndentToType returns the object type ot with appropriate indent such that the object value start at the same
//indent.
func addIndentToType(ot string) string {
	result := "          "
	return ot + result[len(ot):]
}

//encodeNameObject returns no represented as a string in zone file format.
func encodeNameObject(no object.NameObject) string {
	nameObject := []string{}
	for _, oType := range no.Types {
		switch oType {
		case object.OTName:
			nameObject = append(nameObject, TypeName)
		case object.OTIP6Addr:
			nameObject = append(nameObject, TypeIP6)
		case object.OTIP4Addr:
			nameObject = append(nameObject, TypeIP4)
		case object.OTRedirection:
			nameObject = append(nameObject, TypeRedirection)
		case object.OTDelegation:
			nameObject = append(nameObject, TypeDelegation)
		case object.OTNameset:
			nameObject = append(nameObject, TypeNameSet)
		case object.OTCertInfo:
			nameObject = append(nameObject, TypeCertificate)
		case object.OTServiceInfo:
			nameObject = append(nameObject, TypeServiceInfo)
		case object.OTRegistrar:
			nameObject = append(nameObject, TypeRegistrar)
		case object.OTRegistrant:
			nameObject = append(nameObject, TypeRegistrant)
		case object.OTInfraKey:
			nameObject = append(nameObject, TypeInfraKey)
		case object.OTExtraKey:
			nameObject = append(nameObject, TypeExternalKey)
		case object.OTNextKey:
			nameObject = append(nameObject, TypeNextKey)
		default:
			log.Warn("Unsupported object type in nameObject", "actualType", oType, "nameObject", no)
		}
	}
	return fmt.Sprintf("%s [ %s ]", no.Name, strings.Join(nameObject, " "))
}

//encodeEd25519PublicKey returns pkey represented as a string in zone file format.
func encodeEd25519PublicKey(pkey keys.PublicKey) string {
	if key, ok := pkey.Key.(ed25519.PublicKey); ok {
		return fmt.Sprintf("%s %d %s", TypeEd25519, pkey.KeyPhase, hex.EncodeToString(key))
	}
	log.Warn("Type assertion failed. Expected keys.Ed25519PublicKey", "actualType", fmt.Sprintf("%T", pkey.Key))
	return ""
}

//encodeKeySpace returns keySpace represented as a string in zone file format.
func encodeKeySpace(keySpace keys.KeySpaceID) string {
	switch keySpace {
	case keys.RainsKeySpace:
		return TypeKSRains
	default:
		log.Warn("Unsupported key space type", "actualType", keySpace)
	}
	return ""
}

//encodeCertificate returns cert represented as a string in zone file format.
func encodeCertificate(cert object.CertificateObject) string {
	var pt, cu, ca string
	switch cert.Type {
	case object.PTUnspecified:
		pt = TypeUnspecified
	case object.PTTLS:
		pt = TypePTTLS
	default:
		log.Warn("Unsupported protocol type", "protocolType", cert.Type)
		return ""
	}
	switch cert.Usage {
	case object.CUTrustAnchor:
		cu = TypeCUTrustAnchor
	case object.CUEndEntity:
		cu = TypeCUEndEntity
	default:
		log.Warn("Unsupported certificate usage", "certUsage", cert.Usage)
		return ""
	}
	ca = encodeHashAlgo(cert.HashAlgo)
	return fmt.Sprintf("%s %s %s %s", pt, cu, ca, hex.EncodeToString(cert.Data))
}

func encodeHashAlgo(h algorithmTypes.HashAlgorithmType) string {
	switch h {
	case algorithmTypes.NoHashAlgo:
		return TypeNoHash
	case algorithmTypes.Sha256:
		return TypeSha256
	case algorithmTypes.Sha384:
		return TypeSha384
	case algorithmTypes.Sha512:
		return TypeSha512
	case algorithmTypes.Fnv64:
		return TypeFnv64
	case algorithmTypes.Murmur364:
		return TypeMurmur364
	default:
		log.Warn("Unsupported certificate hash algorithm type", "hashAlgoType", h)
		return ""
	}
}

func encodeEd25519Signature(sig signature.Signature) string {
	signature := fmt.Sprintf("%s %s %s %d %d %d", TypeSignature, TypeEd25519, TypeKSRains, sig.PublicKeyID.KeyPhase, sig.ValidSince, sig.ValidUntil)
	if pkey, ok := sig.Data.(ed25519.PublicKey); ok {
		return fmt.Sprintf("%s %s", signature, hex.EncodeToString(pkey))
	}
	return signature
}
