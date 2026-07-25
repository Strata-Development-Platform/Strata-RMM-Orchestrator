package probe

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/gosnmp/gosnmp"
)

type SNMPResult struct {
	OID    string
	Value  string
	Type   string
	Uptime int64
}

func PollSNMP(ctx context.Context, target SNMPTarget) ([]SNMPResult, error) {
	gs := &gosnmp.GoSNMP{
		Target:    target.Host,
		Port:      uint16(target.Port),
		Timeout:   time.Duration(10) * time.Second,
		Retries:   2,
	}

	switch target.Version {
	case "v3":
		gs.Version = gosnmp.Version3
		gs.SecurityModel = gosnmp.UserSecurityModel
		gs.MsgFlags = gosnmp.AuthPriv
		gs.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 target.V3.Username,
			AuthenticationProtocol:   authProto(target.V3.AuthProto),
			AuthenticationPassphrase: target.V3.AuthPass,
			PrivacyProtocol:          privProto(target.V3.PrivProto),
			PrivacyPassphrase:        target.V3.PrivPass,
		}
		if target.V3.Context != "" {
			gs.ContextName = target.V3.Context
		}
	case "v2c":
		gs.Version = gosnmp.Version2c
		gs.Community = target.Community
	default:
		gs.Version = gosnmp.Version1
		gs.Community = target.Community
	}

	if err := gs.Connect(); err != nil {
		return nil, fmt.Errorf("connect to %s: %w", target.Host, err)
	}
	defer gs.Conn.Close()

	sysUptime, _ := getUptime(gs)
	var results []SNMPResult

	for _, oid := range target.OIDs {
		if isTableOID(oid) {
			rows, err := bulkWalk(gs, oid, sysUptime)
			if err != nil {
				continue
			}
			results = append(results, rows...)
		} else {
			result, err := getSingle(gs, oid, sysUptime)
			if err != nil {
				continue
			}
			results = append(results, result)
		}
	}

	if len(target.OIDs) == 0 {
		results, _ = bulkWalk(gs, ".1.3.6.1.2.1", sysUptime)
	}

	return results, nil
}

func getSingle(gs *gosnmp.GoSNMP, oid string, uptime int64) (SNMPResult, error) {
	pkt, err := gs.Get([]string{oid})
	if err != nil {
		return SNMPResult{}, err
	}
	if len(pkt.Variables) == 0 {
		return SNMPResult{}, fmt.Errorf("no response for %s", oid)
	}
	return parseVar(pkt.Variables[0], uptime), nil
}

func bulkWalk(gs *gosnmp.GoSNMP, rootOID string, uptime int64) ([]SNMPResult, error) {
	var results []SNMPResult
	if err := gs.BulkWalk(rootOID, func(pdu gosnmp.SnmpPDU) error {
		results = append(results, parseVar(pdu, uptime))
		return nil
	}); err != nil {
		return results, err
	}
	return results, nil
}

func getUptime(gs *gosnmp.GoSNMP) (int64, error) {
	pkt, err := gs.Get([]string{".1.3.6.1.2.1.1.3.0"})
	if err != nil {
		return 0, err
	}
	if len(pkt.Variables) > 0 {
		return gosnmp.ToBigInt(pkt.Variables[0].Value).Int64(), nil
	}
	return 0, nil
}

func parseVar(pdu gosnmp.SnmpPDU, uptime int64) SNMPResult {
	r := SNMPResult{
		OID:    pdu.Name,
		Uptime: uptime,
	}
	switch v := pdu.Value.(type) {
	case string:
		r.Value = v
		r.Type = "string"
	case []byte:
		r.Value = string(v)
		r.Type = "string"
	case int:
		r.Value = fmt.Sprintf("%d", v)
		r.Type = "integer"
	case int64:
		r.Value = fmt.Sprintf("%d", v)
		r.Type = "integer"
	case uint:
		r.Value = fmt.Sprintf("%d", v)
		r.Type = "integer"
	case uint64:
		r.Value = fmt.Sprintf("%d", v)
		r.Type = "integer"
	case float32:
		r.Value = fmt.Sprintf("%.2f", v)
		r.Type = "float"
	case float64:
		r.Value = fmt.Sprintf("%.2f", v)
		r.Type = "float"
	case nil:
		r.Value = ""
		r.Type = "null"
	case net.IP:
		r.Value = v.String()
		r.Type = "ipaddress"
	default:
		r.Value = fmt.Sprintf("%v", v)
		r.Type = "unknown"
	}
	return r
}

func authProto(proto string) gosnmp.SnmpV3AuthProtocol {
	switch proto {
	case "MD5":
		return gosnmp.MD5
	case "SHA":
		return gosnmp.SHA
	case "SHA224":
		return gosnmp.SHA224
	case "SHA256":
		return gosnmp.SHA256
	case "SHA384":
		return gosnmp.SHA384
	case "SHA512":
		return gosnmp.SHA512
	default:
		return gosnmp.MD5
	}
}

func privProto(proto string) gosnmp.SnmpV3PrivProtocol {
	switch proto {
	case "DES":
		return gosnmp.DES
	case "AES":
		return gosnmp.AES
	case "AES192":
		return gosnmp.AES192
	case "AES256":
		return gosnmp.AES256
	default:
		return gosnmp.NoPriv
	}
}

func isTableOID(oid string) bool {
	return len(oid) > 0 && oid[len(oid)-1] == '*'
}
