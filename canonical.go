package lunixi

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// MaxSafeInteger is the largest integer JSON.stringify preserves exactly.
const MaxSafeInteger int64 = 9007199254740991

type member struct {
	key   string
	value any
}
type object []member

// CanonicalJSON produces compact JSON with JavaScript's numeric-index key
// ordering, UTF-8 strings, and number formatting. Unsafe integer numbers and
// duplicate object keys are rejected; use strings where an API specifies them.
func CanonicalJSON(value any) ([]byte, error) {
	var initial bytes.Buffer
	enc := json.NewEncoder(&initial)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, invalid("JSON body")
	}
	dec := json.NewDecoder(&initial)
	dec.UseNumber()
	v, err := readJSON(dec, 0)
	if err != nil {
		return nil, err
	}
	if _, err = dec.Token(); err != io.EOF {
		return nil, invalid("JSON trailing content")
	}
	var out bytes.Buffer
	if err := writeJSON(&out, v); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func readJSON(dec *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, invalid("JSON nesting limit")
	}
	t, err := dec.Token()
	if err != nil {
		return nil, invalid("JSON body")
	}
	switch t {
	case json.Delim('{'):
		o := object{}
		seen := map[string]bool{}
		for dec.More() {
			k, e := dec.Token()
			if e != nil {
				return nil, invalid("JSON key")
			}
			key, ok := k.(string)
			if !ok || seen[key] {
				return nil, invalid("duplicate JSON key")
			}
			seen[key] = true
			v, e := readJSON(dec, depth+1)
			if e != nil {
				return nil, e
			}
			o = append(o, member{key, v})
		}
		if _, e := dec.Token(); e != nil {
			return nil, invalid("JSON object")
		}
		sort.SliceStable(o, func(i, j int) bool {
			a, ao := arrayIndex(o[i].key)
			b, bo := arrayIndex(o[j].key)
			if ao != bo {
				return ao
			}
			return ao && a < b
		})
		return o, nil
	case json.Delim('['):
		a := []any{}
		for dec.More() {
			v, e := readJSON(dec, depth+1)
			if e != nil {
				return nil, e
			}
			a = append(a, v)
		}
		if _, e := dec.Token(); e != nil {
			return nil, invalid("JSON array")
		}
		return a, nil
	default:
		return t, nil
	}
}

func arrayIndex(key string) (uint64, bool) {
	if key == "" || len(key) > 10 || (len(key) > 1 && key[0] == '0') {
		return 0, false
	}
	n, err := strconv.ParseUint(key, 10, 32)
	return n, err == nil && n < 4294967295 && strconv.FormatUint(n, 10) == key
}

func writeJSON(out *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case object:
		out.WriteByte('{')
		for i, m := range x {
			if i > 0 {
				out.WriteByte(',')
			}
			out.WriteString(jsonString(m.key))
			out.WriteByte(':')
			if e := writeJSON(out, m.value); e != nil {
				return e
			}
		}
		out.WriteByte('}')
	case []any:
		out.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeJSON(out, e); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case string:
		out.WriteString(jsonString(x))
	case json.Number:
		f, e := strconv.ParseFloat(string(x), 64)
		if e != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return invalid("JSON number")
		}
		if math.Trunc(f) == f && math.Abs(f) > float64(MaxSafeInteger) {
			return invalid("unsafe JSON integer; encode documented string amounts as strings")
		}
		if f == 0 {
			out.WriteByte('0')
			break
		}
		if math.Abs(f) >= 1e-6 && math.Abs(f) < 1e21 {
			out.WriteString(strconv.FormatFloat(f, 'f', -1, 64))
			break
		}
		s := strconv.FormatFloat(f, 'e', -1, 64)
		parts := strings.Split(s, "e")
		exp, _ := strconv.Atoi(parts[1])
		out.WriteString(parts[0])
		out.WriteByte('e')
		if exp >= 0 {
			out.WriteByte('+')
		}
		out.WriteString(strconv.Itoa(exp))
	case bool:
		out.WriteString(strconv.FormatBool(x))
	case nil:
		out.WriteString("null")
	default:
		return fmt.Errorf("%w: unsupported JSON value", ErrConfiguration)
	}
	return nil
}

func jsonString(s string) string {
	var out bytes.Buffer
	out.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteRune(r)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&out, `\u%04x`, r)
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
	return out.String()
}

// Digest returns the gateway's SHA-256 digest header value.
func Digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "SHA-256=" + base64.StdEncoding.EncodeToString(sum[:])
}

// CanonicalRequest returns the exact string signed by the gateway protocol.
func CanonicalRequest(method, path, date, nonce, digest string) string {
	s := strings.ToUpper(method) + "\n" + path + "\nX-Date:" + date + "\nX-Nonce:" + nonce
	if digest != "" {
		s += "\nDigest:" + digest
	}
	return s
}
