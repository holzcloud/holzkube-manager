package auth

// The token-authenticated actor, carried on the request context.
//
// A person's identity lives in the session, which is where CurrentUser has
// always read it from. A service account has no session: it presents a token on
// every request and is resolved per request. Rather than teach every caller
// that there are now two places an identity can come from, there is one place
// -- CurrentUser -- and it asks the context first.

import (
	"context"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// actorKey is the context key for a token-authenticated account. It is an
// unexported type so nothing outside this package can put one there.
type actorKey struct{}

// WithTokenActor marks a request as authenticated by a service-account token.
//
// It is the only way an identity enters a request other than through the
// session, and it is called from exactly one place: the middleware that
// validated the token. A handler cannot call it, because nothing outside this
// package can construct the key.
func WithTokenActor(ctx context.Context, u model.User) context.Context {
	return context.WithValue(ctx, actorKey{}, u)
}

// TokenActor returns the service account this request presented a token for.
func TokenActor(ctx context.Context) (model.User, bool) {
	u, ok := ctx.Value(actorKey{}).(model.User)
	return u, ok
}
