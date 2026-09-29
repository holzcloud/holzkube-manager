package jobs

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Confirmation is server-side enforcement of "are you sure" (JOB-08).
//
// A dialog in the browser protects against a misclick and against nothing
// else. Anything that can reach the API can skip it, and the API is reachable
// by every script, every stale tab and every replayed request. So the server
// issues a token that describes **exactly** the action it is a confirmation
// for, and the destructive route re-derives that description from what was
// actually submitted and compares.
//
// That last part is the property worth having, and it is not the same as "a
// token was presented". A confirmation for `wipe_mode=user-disks` cannot
// authorise `wipe_mode=all`, because the two produce different tokens. The
// dialog the operator read and the request the server performs are the same
// action or the request is refused.
//
// The signing key lives in memory and is generated at startup. A token
// therefore does not survive a restart, which is correct: a confirmation is a
// statement about a decision somebody is making right now, and one that
// outlived the process it was issued by would be a decision about a fleet that
// may have changed.

var (
	// ErrConfirmationInvalid reports a token that does not verify.
	ErrConfirmationInvalid = errors.New("jobs: the confirmation does not match this request")

	// ErrConfirmationExpired reports a token that verified and is too old.
	// It is separate because the remedy differs: re-read the dialog, rather
	// than "something is wrong".
	ErrConfirmationExpired = errors.New("jobs: the confirmation has expired")

	// ErrConfirmationUsed reports a single-use confirmation (IssueOnce)
	// presented a second time. It wraps ErrConfirmationInvalid: to the client it is a
	// token that does not open this request, and the remedy is the same --
	// confirm again.
	ErrConfirmationUsed = fmt.Errorf("%w: it has been used already", ErrConfirmationInvalid)
)

// ConfirmationTTL is how long a confirmation is good for.
//
// Short, because it is a statement about a decision being made now. Long
// enough that reading a reset dialog carefully -- which is the behaviour the
// dialog is trying to produce -- does not invalidate it.
const ConfirmationTTL = 10 * time.Minute

// Confirmer issues and checks confirmation tokens.
type Confirmer struct {
	key []byte
	now func() time.Time

	// mu guards spent: the nonces of the single-use tokens CheckOnce has let
	// through, each with the moment its token expires, after which it is
	// forgotten -- CheckOnce refuses the token by then anyway.
	mu    sync.Mutex
	spent map[string]time.Time
}

// NewConfirmer generates a signing key.
func NewConfirmer() (*Confirmer, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("jobs: generate confirmation key: %w", err)
	}
	return &Confirmer{key: key, now: time.Now}, nil
}

// Intent is what a confirmation is *about*.
//
// Params is the whole parameter set and not a summary, because the token's
// value is that it cannot authorise a different action -- and "different" has
// to include every field that changes what happens.
type Intent struct {
	Action  string            `json:"action"`
	Machine string            `json:"machine"`
	Params  map[string]string `json:"params,omitempty"`
}

// Issue returns a token for an intent, and the moment it stops being valid.
func (c *Confirmer) Issue(in Intent) (token string, expires time.Time) {
	expires = c.now().Add(ConfirmationTTL).UTC()
	payload := fmt.Sprintf("%d|%s", expires.Unix(), canonical(in))

	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(payload))

	return fmt.Sprintf("%d.%s", expires.Unix(),
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))), expires
}

// Check verifies a token against the intent the request actually carries.
//
// The intent is rebuilt from the submitted request rather than taken from the
// token: a token that carried its own description would authorise whatever it
// said, and the request could say something else.
func (c *Confirmer) Check(token string, in Intent) error {
	stamp, sig, ok := strings.Cut(token, ".")
	if !ok {
		return ErrConfirmationInvalid
	}

	var unix int64
	if _, err := fmt.Sscanf(stamp, "%d", &unix); err != nil {
		return ErrConfirmationInvalid
	}

	payload := fmt.Sprintf("%s|%s", stamp, canonical(in))
	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(payload))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	// Constant time, because the comparison is against a value an attacker
	// supplies and can vary.
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return ErrConfirmationInvalid
	}

	// Expiry is checked only after the signature verifies, so that a forged
	// token cannot be told from an expired one by the answer it gets.
	if c.now().After(time.Unix(unix, 0)) {
		return ErrConfirmationExpired
	}
	return nil
}

// IssueOnce returns a single-use token for an intent: one CheckOnce lets it
// through, and no second.
//
// Issue alone cannot give that. Its token is a function of the intent and the
// expiry second, so two confirmations of the same intent in the same second
// are the same token, and spending one would spend the other. A single-use
// token carries a random nonce between its stamp and its signature, covered by
// the signature and kept apart from Issue's tokens by the payload's "once:"
// mark: Check refuses it, and CheckOnce refuses a token from Issue.
func (c *Confirmer) IssueOnce(in Intent) (token string, expires time.Time) {
	expires = c.now().Add(ConfirmationTTL).UTC()
	nonce := rand.Text()
	return fmt.Sprintf("%d.%s.%s", expires.Unix(), nonce, c.signOnce(strconv.FormatInt(expires.Unix(), 10), nonce, in)), expires
}

// signOnce is the signature of a single-use token.
func (c *Confirmer) signOnce(stamp, nonce string, in Intent) string {
	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(stamp + "|once:" + nonce + "|" + canonical(in)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// CheckOnce verifies a token from IssueOnce against the intent the request
// actually carries, like Check, and spends it.
//
// Check lets a token through as often as it is presented until it expires,
// which is right where a token stands for a decision about a parameter set (a
// reset retried after a timeout is the same reset). It is not right where
// typing the confirmation is meant to happen every time: the host actions
// (D-09), where one typed hostname must not place a second reboot minutes
// later from a replayed request. A token that verifies is spent here, under
// the same lock as the question whether it was spent before, so two requests
// racing with one token get one success and one ErrConfirmationUsed. A refused
// token is not spent.
func (c *Confirmer) CheckOnce(token string, in Intent) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ErrConfirmationInvalid
	}
	stamp, nonce, sig := parts[0], parts[1], parts[2]
	unix, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil || nonce == "" {
		return ErrConfirmationInvalid
	}
	// Constant time, as in Check.
	if !hmac.Equal([]byte(sig), []byte(c.signOnce(stamp, nonce, in))) {
		return ErrConfirmationInvalid
	}
	expires := time.Unix(unix, 0)

	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if now.After(expires) {
		return ErrConfirmationExpired
	}
	// Forget what has expired: Check-style expiry refuses it from here on.
	for n, at := range c.spent {
		if now.After(at) {
			delete(c.spent, n)
		}
	}
	if _, used := c.spent[nonce]; used {
		return ErrConfirmationUsed
	}
	if c.spent == nil {
		c.spent = map[string]time.Time{}
	}
	c.spent[nonce] = expires
	return nil
}

// canonical renders an intent so that two equal intents produce identical
// bytes and two different ones never do.
//
// The map is sorted, because Go's map iteration order is deliberately random
// and a signature over an unsorted encoding would verify or not depending on
// which way the runtime felt.
func canonical(in Intent) string {
	keys := make([]string, 0, len(in.Params))
	for k := range in.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(in.Action)
	b.WriteByte('|')
	b.WriteString(in.Machine)
	for _, k := range keys {
		b.WriteByte('|')
		// Both halves are quoted, so that a value containing a separator
		// cannot be rearranged into a different intent that encodes the same.
		b.WriteString(strconv.Quote(k))
		b.WriteByte('=')
		b.WriteString(strconv.Quote(in.Params[k]))
	}
	return b.String()
}

// MarshalIntent is what the issuing route echoes back, so a client can show
// the operator exactly what they are confirming.
func MarshalIntent(in Intent) (string, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
