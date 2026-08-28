package auth

import (
	"context"
	"time"
)

// DefaultSudoWindow is how long one re-authentication authorises destructive
// actions (D-05). It is configurable; this is the value the operator gets when
// they do not choose one.
const DefaultSudoWindow = 5 * time.Minute

// sessionKeySudoAt is when the window was last opened or refreshed, in Unix
// nanoseconds.
//
// The state lives in the session record and nowhere else. There is no
// process-wide register of who has recently re-authenticated, because such a
// register would let a second, unrelated session ride on the first one's
// re-authentication -- which is precisely the theft the sudo window exists to
// contain (T-01-25).
const sessionKeySudoAt = "sudo_at"

// OpenSudoWindow starts the window after a successful re-authentication.
func (s *Service) OpenSudoWindow(ctx context.Context) {
	s.sm.Put(ctx, sessionKeySudoAt, s.now().UnixNano())
}

// TouchSudoWindow restarts the window after a destructive action has been
// performed (D-05), so that resetting three nodes in a row asks for the
// password once rather than three times.
//
// It only extends a window that is already open: the caller is the middleware,
// which runs it after the gate has already let the request through.
func (s *Service) TouchSudoWindow(ctx context.Context) {
	s.sm.Put(ctx, sessionKeySudoAt, s.now().UnixNano())
}

// IsSudoOpen reports whether this session re-authenticated within window.
//
// A non-positive window falls back to DefaultSudoWindow. Treating zero as
// "always closed" would make every destructive route permanently unreachable,
// and treating it as "always open" would disable the gate; falling back to the
// decided default is the only reading that is neither an outage nor a hole.
func (s *Service) IsSudoOpen(ctx context.Context, window time.Duration) bool {
	if window <= 0 {
		window = DefaultSudoWindow
	}
	stamp := s.sm.GetInt64(ctx, sessionKeySudoAt)
	if stamp == 0 {
		return false
	}
	return s.now().Before(time.Unix(0, stamp).Add(window))
}
