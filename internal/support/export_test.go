package support

// ConfigForBundle is configForBundle, for the test of what leaves the
// installation when a node's configuration does not parse. The simulator
// cannot serve such a configuration -- it parses its own with the same
// machinery -- so the decision is tested where it is made.
var ConfigForBundle = configForBundle
