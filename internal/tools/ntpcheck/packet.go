package ntpcheck

import (
	"encoding/binary"
	"strings"
	"time"
)

// packetBytes is the fixed size of an NTP packet without extensions
// (RFC 4330 section 4).
const packetBytes = 48

// eraSeconds is the gap between the NTP epoch (1900-01-01) and the Unix epoch
// (1970-01-01). Confirmed against python3's datetime before being written here.
const eraSeconds = 2208988800

// clientFirstByte is leap indicator 0, version 4, mode 3 (client):
// 0<<6 | 4<<3 | 3.
const clientFirstByte = 0x23

// modeServer and modeBroadcast are the two modes a client may accept in a
// reply. Anything else is not an answer to the question that was asked.
const (
	modeServer    = 4
	modeBroadcast = 5
)

// buildRequest is the 48 byte client packet. Only the first byte and the
// transmit timestamp carry anything: a client has nothing else to say.
func buildRequest(sent time.Time) []byte {
	b := make([]byte, packetBytes)
	b[0] = clientFirstByte
	secs, frac := toNTP(sent)
	binary.BigEndian.PutUint32(b[40:], secs)
	binary.BigEndian.PutUint32(b[44:], frac)
	return b
}

// reply is a parsed server packet. The three timestamps are the ones the
// arithmetic needs; the rest is what the UI shows.
type reply struct {
	stratum int
	// kiss is the four character code a stratum 0 reply carries instead of a
	// time, with any padding trimmed.
	kiss string
	// originate is the transmit timestamp the client sent, echoed back. It is
	// checked against what was really sent so a stray packet cannot be mistaken
	// for the answer.
	originate time.Time
	receive   time.Time
	transmit  time.Time
}

// parseErr says why a packet was not an answer. The caller turns it into a
// sentence; nothing here reaches a user.
type parseErr string

func (e parseErr) Error() string { return string(e) }

const (
	errShort    parseErr = "short"
	errMode     parseErr = "mode"
	errNoTime   parseErr = "no time"
	errKiss     parseErr = "kiss"
	errMismatch parseErr = "mismatch"
)

// parseReply validates a server packet and pulls out the three timestamps.
// sent is what the client put in its transmit field, and a reply that does not
// echo it back is not the answer to this question.
func parseReply(b []byte, sent time.Time) (reply, error) {
	var r reply
	if len(b) < packetBytes {
		return r, errShort
	}
	mode := int(b[0] & 0x07)
	if mode != modeServer && mode != modeBroadcast {
		return r, errMode
	}
	r.stratum = int(b[1])
	if r.stratum == 0 {
		r.kiss = strings.TrimRight(string(b[12:16]), "\x00 ")
		return r, errKiss
	}

	originSecs, originFrac := binary.BigEndian.Uint32(b[24:]), binary.BigEndian.Uint32(b[28:])
	r.originate = fromNTP(originSecs, originFrac)
	r.receive = fromNTP(binary.BigEndian.Uint32(b[32:]), binary.BigEndian.Uint32(b[36:]))
	r.transmit = fromNTP(binary.BigEndian.Uint32(b[40:]), binary.BigEndian.Uint32(b[44:]))

	// A zero transmit timestamp is a server that answered without a time in it,
	// which is worse than no answer because the arithmetic would happily produce
	// a 126 year offset from it.
	if r.transmit.IsZero() || r.receive.IsZero() {
		return r, errNoTime
	}
	// Compared as encoded, not as decoded times: the 32 bit fraction cannot hold
	// every nanosecond, so a decoded originate is up to a nanosecond away from
	// the instant that produced it and would never compare equal.
	if wantSecs, wantFrac := toNTP(sent); originSecs != wantSecs || originFrac != wantFrac {
		return r, errMismatch
	}
	return r, nil
}

// toNTP splits an instant into the NTP era seconds and the 32 bit binary
// fraction. Integer arithmetic throughout, so a fraction that is an exact
// binary value survives a round trip unchanged.
func toNTP(t time.Time) (uint32, uint32) {
	if t.IsZero() {
		return 0, 0
	}
	secs := uint32(t.Unix() + eraSeconds)
	frac := uint32(uint64(t.Nanosecond()) << 32 / uint64(time.Second))
	return secs, frac
}

// fromNTP is the inverse. All zeroes is how NTP says "no time here", so it
// comes back as the zero time rather than as 1900.
func fromNTP(secs, frac uint32) time.Time {
	if secs == 0 && frac == 0 {
		return time.Time{}
	}
	ns := uint64(frac) * uint64(time.Second) >> 32
	return time.Unix(int64(secs)-eraSeconds, int64(ns)).UTC()
}

// offsetAndDelay is the RFC 4330 section 5 arithmetic.
//
// The sign is flipped from the RFC's: it defines the offset as how far the
// server is ahead of the client, and every sentence this tool writes is about
// how far this computer is ahead of the server, which is what a tech is asked
// about ("is my clock fast or slow?").
func offsetAndDelay(t1 time.Time, r reply, t4 time.Time) (offset, delay time.Duration) {
	serverAhead := ((r.receive.Sub(t1)) + (r.transmit.Sub(t4))) / 2
	return -serverAhead, (t4.Sub(t1)) - (r.transmit.Sub(r.receive))
}
