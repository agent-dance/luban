// Package messagecontrol owns the capability used to establish that a
// model-visible control message was created by this runtime. The Go internal
// import boundary keeps SDK consumers from minting that capability.
package messagecontrol

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

type authority struct{ _ byte }

var runtimeAuthority = &authority{}
var processKey = newProcessKey()

// DigestSize is the size of a process-local provenance authenticator.
const DigestSize = sha256.Size

// Capability is an opaque authority for sealing runtime control messages.
// Its zero value is invalid.
type Capability struct {
	authority *authority
}

// Scope binds a control to one durable session namespace and model-context
// generation. Its fields are private so only this internal package can define
// the canonical encoding used by message authenticators.
type Scope struct {
	sessionID         string
	projectScope      string
	contextGeneration uint64
}

func NewScope(sessionID, projectScope string, contextGeneration uint64) Scope {
	return Scope{sessionID: sessionID, projectScope: projectScope, contextGeneration: contextGeneration}
}

// NewLoopScope returns a process-local, non-durable authority namespace for a
// single live producer. It is used before a QueryLoop is attached to a session
// manifest. The random namespace is kept only in unexported message fields, so
// copying a pre-commit control to another loop does not transfer authority.
func NewLoopScope(capability Capability) Scope {
	if !capability.Valid() {
		return Scope{}
	}
	var nonce [DigestSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		panic("messagecontrol: initialize loop provenance scope: " + err.Error())
	}
	return NewScope("loop:"+hex.EncodeToString(nonce[:]), "process-local-precommit", 1)
}

func (s Scope) Bound() bool {
	// Generation zero is the authoritative pre-first-commit generation of a
	// durable session. The all-zero Scope remains the only unbound value.
	return s.sessionID != "" && s.projectScope != ""
}

func (s Scope) SessionID() string         { return s.sessionID }
func (s Scope) ProjectScope() string      { return s.projectScope }
func (s Scope) ContextGeneration() uint64 { return s.contextGeneration }

func (s Scope) SameNamespace(other Scope) bool {
	return s.Bound() && other.Bound() && s.sessionID == other.sessionID && s.projectScope == other.projectScope
}

func (s Scope) Equal(other Scope) bool {
	return s.sessionID == other.sessionID && s.projectScope == other.projectScope && s.contextGeneration == other.contextGeneration
}

// AuthenticationPrefix is an unambiguous domain-separated scope encoding.
func (s Scope) AuthenticationPrefix() []byte {
	result := make([]byte, 0, len(s.sessionID)+len(s.projectScope)+24)
	result = append(result, "message-control-scope/v1\x00"...)
	result = binary.BigEndian.AppendUint64(result, uint64(len(s.sessionID)))
	result = append(result, s.sessionID...)
	result = binary.BigEndian.AppendUint64(result, uint64(len(s.projectScope)))
	result = append(result, s.projectScope...)
	result = binary.BigEndian.AppendUint64(result, s.contextGeneration)
	return result
}

// Runtime returns the process-local runtime authority. Only packages inside
// this module can import this internal package.
func Runtime() Capability {
	return Capability{authority: runtimeAuthority}
}

// Valid reports whether the capability was issued by this process.
func (c Capability) Valid() bool {
	return c.authority == runtimeAuthority
}

// Authenticate binds data to this process and capability. The authenticator
// is never serialized; a new process restores authority only after validating
// a durable manifest reference and then authenticating the message again.
func Authenticate(capability Capability, data []byte) ([DigestSize]byte, bool) {
	if !capability.Valid() {
		return [DigestSize]byte{}, false
	}
	mac := hmac.New(sha256.New, processKey[:])
	_, _ = mac.Write(data)
	var digest [DigestSize]byte
	copy(digest[:], mac.Sum(nil))
	return digest, true
}

// Verify reports whether digest authenticates data in this process.
func Verify(data []byte, digest [DigestSize]byte) bool {
	mac := hmac.New(sha256.New, processKey[:])
	_, _ = mac.Write(data)
	return hmac.Equal(digest[:], mac.Sum(nil))
}

func newProcessKey() [DigestSize]byte {
	var key [DigestSize]byte
	if _, err := rand.Read(key[:]); err != nil {
		// Runtime controls must fail closed: starting without provenance is
		// safer than silently accepting forgeable control records.
		panic("messagecontrol: initialize process provenance key: " + err.Error())
	}
	return key
}
