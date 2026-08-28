package talossim

import "errors"

var (
	errNoMachine = errors.New("target has no machine ID; the address is a hint and never identity")
	errNoTLS     = errors.New("talossim: credentials carry no TLS configuration")
)
