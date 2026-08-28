package audit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"
)

// Outcome values. An audit entry is written twice: once as the intent, before
// anything happens, and once as the result afterwards. Success-only logging
// discards exactly the forensics that matter, and an attempt with no matching
// outcome is itself a finding.
const (
	OutcomeAttempt = "attempt"
	OutcomeSuccess = "success"
	OutcomeError   = "error"
)

// Record is one line of the audit log.
//
// The field set is closed (D-14) and is part of the hash chain. Adding,
// removing or renaming a field is not a compatible change: rotated files are
// kept indefinitely (D-16), so every record ever written must stay verifiable.
type Record struct {
	Seq       uint64         `json:"seq"`
	TS        time.Time      `json:"ts"`
	Actor     string         `json:"actor"`
	Session   string         `json:"session"`
	SrcIP     string         `json:"src_ip"`
	ClusterID string         `json:"cluster_id"`
	MachineID string         `json:"machine_id"`
	Action    string         `json:"action"`
	Params    map[string]any `json:"params"`
	JobID     string         `json:"job_id"`
	Outcome   string         `json:"outcome"`
	PrevHash  string         `json:"prev_hash"`
	Hash      string         `json:"hash"`
}

// sessionPrefix is how much of a session token survives into a record.
const sessionPrefix = 8

// ShortSession truncates a session token to a correlation handle.
//
// The full token is a live credential. Writing it into a log that is kept
// forever (D-16) would turn the archive into a store of every session that ever
// existed, which is exactly the "the log itself becomes the secret store"
// outcome D-14 forbids. Eight characters of a 256-bit token are far too few to
// guess the rest and quite enough to tell two sessions apart, which is all the
// forensic question needs.
func ShortSession(token string) string {
	if len(token) <= sessionPrefix {
		return token
	}
	return token[:sessionPrefix] + "…"
}

// canonicalTS is the one true timestamp rendering for both the written line and
// the hashed bytes. Round-tripping through it is stable: format -> parse ->
// format yields the identical string.
const canonicalTS = time.RFC3339Nano

// canonicalFields projects a Record onto the map that gets hashed. The hash
// field is excluded by construction -- it is the output, it cannot be an input.
func (r Record) canonicalFields() (map[string]any, error) {
	params, err := normalizeValue(r.Params)
	if err != nil {
		return nil, fmt.Errorf("normalize params: %w", err)
	}
	if params == nil {
		params = map[string]any{}
	}
	return map[string]any{
		"seq":        r.Seq,
		"ts":         r.TS.UTC().Format(canonicalTS),
		"actor":      r.Actor,
		"session":    r.Session,
		"src_ip":     r.SrcIP,
		"cluster_id": r.ClusterID,
		"machine_id": r.MachineID,
		"action":     r.Action,
		"params":     params,
		"job_id":     r.JobID,
		"outcome":    r.Outcome,
		"prev_hash":  r.PrevHash,
	}, nil
}

// CanonicalJSON renders the record, without its hash field, in canonical form:
// keys sorted lexicographically, no whitespace, UTF-8, no HTML escaping.
//
// This is deliberately not encoding/json's output. The chain must survive a Go
// upgrade, a change in escaping behaviour, and a reordered struct field --
// because D-16 keeps every rotated file forever, so a chain anchored to the
// encoder's byte output would break retroactively across the entire archive.
// Once these bytes are hashed, the line actually written to disk may be
// formatted however is convenient.
func (r Record) CanonicalJSON() ([]byte, error) {
	fields, err := r.canonicalFields()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := writeCanonical(&buf, fields); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ComputeHash returns hash_n = sha256(hash_{n-1} || canonical_json(record_n
// without the hash field)). The previous hash is also carried inside the record
// as prev_hash, so a rewritten link is detectable from either direction.
func (r Record) ComputeHash() (string, error) {
	canon, err := r.CanonicalJSON()
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(r.PrevHash))
	h.Write(canon)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// normalizeValue collapses an arbitrary value into a tree built only from
// map[string]any, []any, string, json.Number, bool and nil.
//
// Nested params must be normalized explicitly rather than left to Go's map
// iteration order, which is randomized per run. Routing through the decoder
// with UseNumber also pins numeric rendering to the literal that was written,
// so an integer never silently becomes 1e+06.
func normalizeValue(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		writeCanonicalString(buf, t)
	case json.Number:
		buf.WriteString(t.String())
	case uint64:
		buf.WriteString(strconv.FormatUint(t, 10))
	case int:
		buf.WriteString(strconv.FormatInt(int64(t), 10))
	case int64:
		buf.WriteString(strconv.FormatInt(t, 10))
	case float64:
		buf.WriteString(strconv.FormatFloat(t, 'g', -1, 64))
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeCanonicalString(buf, k)
			buf.WriteByte(':')
			if err := writeCanonical(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	default:
		return fmt.Errorf("audit: value of type %T cannot be canonicalized", v)
	}
	return nil
}

// writeCanonicalString emits a JSON string with our own escaping rules, so the
// bytes do not depend on encoding/json's HTML-escaping behaviour.
func writeCanonicalString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		default:
			switch {
			case r < 0x20:
				fmt.Fprintf(buf, `\u%04x`, r)
			case r == utf8.RuneError:
				buf.WriteString(`�`)
			default:
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte('"')
}
