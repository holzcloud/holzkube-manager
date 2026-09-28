package host

import (
	"errors"
	"io/fs"
)

var errNotYet = errors.New("not implemented")

func readModel(fs.FS) Reading[string]     { return Hidden[string](reasonFor("model", errNotYet)) }
func readOSRelease(fs.FS) Reading[string] { return Hidden[string](reasonFor("os-release", errNotYet)) }
func readCores(fs.FS) Reading[int]        { return Hidden[int](reasonFor("cpu", errNotYet)) }
func countCPUList(string) (int, error)    { return 0, errNotYet }
func detectContainer(fs.FS) bool          { return false }
