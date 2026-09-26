package talos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
)

// Temperatures and fans come from Linux's hwmon tree, read file by file
// through the node's own List and Read RPCs.
//
// There is no other way to get them. Talos keeps no resource for a sensor and
// has no RPC that returns one, and the machine API's file reads are what
// `talosctl read /sys/class/hwmon/...` already uses -- they need os:admin,
// which is the role this product's client certificate carries anyway.
//
// The tree is discovered once and then only its *_input files are read. A
// discovery lists every chip and reads every label and threshold, which is
// dozens of round trips; the inputs are one read per sensor. The layout is
// handed back to the caller to keep, because a ClusterClient lives for one
// request and the layout is worth keeping for minutes.

const (
	hwmonClass   = "/sys/class/hwmon"
	thermalClass = "/sys/class/thermal"

	// maxProbedTemps and maxProbedFans bound the fallback that guesses file
	// names when a chip's directory cannot be listed. hwmon numbers inputs
	// from 1 without gaps in practice, and the busiest board chips in use
	// (nct6798, it8686) stop well below these.
	maxProbedTemps = 16
	maxProbedFans  = 8

	// sensorReadConcurrency is how many file reads are in flight at once on
	// the one connection. Each is its own gRPC stream; a desktop board with
	// three chips is thirty reads at discovery, and opening them all at once
	// would ask a node's apid for thirty concurrent streams to answer a
	// question it answers just as fast eight at a time.
	sensorReadConcurrency = 8

	// maxAttributeBytes caps one file read. A sysfs attribute is a page at
	// most; anything longer is not the file this code thinks it is reading,
	// and is treated as absent rather than buffered.
	maxAttributeBytes = 4096
)

// SensorKind is what a chip measures, as far as its driver's name says.
type SensorKind string

// The kinds, in the words the API uses.
const (
	SensorCPU   SensorKind = "cpu"
	SensorBoard SensorKind = "board"
	SensorDisk  SensorKind = "disk"
	SensorGPU   SensorKind = "gpu"
	SensorOther SensorKind = "other"
)

// ClassifyChip names what an hwmon chip measures from its driver's name.
//
// The name is the only evidence there is: hwmon carries no "this is the CPU"
// attribute, and a chip's temperatures are labelled by whatever its driver's
// author chose. The table is the drivers a homelab board actually carries.
// Everything else is "other", which is shown rather than dropped -- an
// unrecognised chip is still a real reading, and hiding it would be this
// product deciding which of a node's sensors matter.
func ClassifyChip(name string) SensorKind {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case n == "coretemp", n == "k10temp", n == "zenpower",
		// A thermal zone's hwmon twin takes the zone's type with '-'
		// turned into '_', which is how a Raspberry Pi's CPU arrives.
		n == "cpu_thermal", n == "cpu-thermal", n == "x86_pkg_temp":
		return SensorCPU
	case n == "nvme", n == "drivetemp":
		return SensorDisk
	case strings.HasPrefix(n, "nct"),
		// it87's chips name themselves by model -- it8686, it8728 -- so the
		// prefix is the family, not the driver.
		strings.HasPrefix(n, "it8"),
		strings.HasPrefix(n, "w83"),
		strings.HasPrefix(n, "asus"),
		strings.HasPrefix(n, "pch_"),
		n == "acpitz", n == "gigabyte_wmi":
		return SensorBoard
	case n == "amdgpu", n == "nouveau", n == "radeon":
		return SensorGPU
	default:
		return SensorOther
	}
}

// SensorLayout is a node's discovered sensor tree: which files to read, and
// what each one means.
type SensorLayout struct {
	Chips []SensorChip

	// Boot is the boot this layout was discovered in. It is a layout for that
	// boot only: see HardwareSample for why a reboot invalidates it even when
	// every file in it still exists.
	Boot time.Time
}

// SensorChip is one hwmon chip, or one thermal zone standing in for a chip
// that has none.
type SensorChip struct {
	// Name is the driver's name for the chip, e.g. "coretemp", "nvme".
	Name string
	Kind SensorKind

	// DeviceModel and DeviceSerial are what the chip's parent device reports,
	// read for disk chips only: they are how a drive temperature is matched to
	// a drive. Either may be empty -- drivetemp's SCSI parent has no serial
	// attribute.
	DeviceModel  string
	DeviceSerial string

	Temps []TempInput
	Fans  []FanInput
}

// TempInput is one temperature sensor: the file to read, and what was learned
// about it once at discovery.
type TempInput struct {
	Input string
	Label string

	// High and Critical are the chip's own thresholds in degrees Celsius, nil
	// when the chip reports none. They are read once: a threshold is a
	// property of the chip, not a reading.
	High     *float64
	Critical *float64
}

// FanInput is one fan tachometer.
type FanInput struct {
	Input string
	Label string
}

// ChipReading is one chip's sensors as read just now.
type ChipReading struct {
	Name         string
	Kind         SensorKind
	DeviceModel  string
	DeviceSerial string

	Temps []TempReading
	Fans  []FanReading
}

// TempReading is one temperature, in degrees Celsius.
type TempReading struct {
	Label    string
	Celsius  float64
	High     *float64
	Critical *float64
}

// FanReading is one fan's speed. Zero is a reading, not an absence: it is a
// stopped fan, or a header with nothing plugged into it, and both are worth
// seeing.
type FanReading struct {
	Label string
	RPM   int
}

// dirEntry is one name in a listed directory.
type dirEntry struct {
	name string
	link string
}

// listDir lists one directory on the node, without descending.
//
// The node answers "no such directory" as a refusal, and that is returned as
// ok=false rather than as an error: a node with no hwmon tree has no sensors,
// which is an answer. Only a node that did not answer is an error.
func (c *ClusterClient) listDir(ctx context.Context, root string) (entries []dirEntry, ok bool, err error) {
	// Released on every return, including the early ones, because a stream
	// abandoned before io.EOF holds its context until somebody cancels it --
	// the same obligation LogStream.Close exists for.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := c.conn.c.MachineClient.List(ctx, &machine.ListRequest{Root: root})
	if err != nil {
		return nil, false, absentOr(err)
	}

	clean := path.Clean(root)
	for {
		fi, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return entries, true, nil
		}
		if err != nil {
			return nil, false, absentOr(err)
		}

		// An entry the node could not stat is skipped rather than fatal: one
		// unreadable name says nothing about its siblings.
		if fi.GetError() != "" {
			continue
		}

		// The walk reports the root itself first, as ".". A root that is a
		// symlink the node did not follow comes back as the link alone, named
		// after itself; either way it is not a child.
		name := fi.GetRelativeName()
		if name == "" {
			name = path.Base(fi.GetName())
		}
		if name == "." || path.Clean(fi.GetName()) == clean || strings.Contains(name, "/") {
			continue
		}
		entries = append(entries, dirEntry{name: name, link: fi.GetLink()})
	}
}

// readFile reads one small file on the node. ok is false when the node
// answered that it is not there.
func (c *ClusterClient) readFile(ctx context.Context, p string) (content string, ok bool, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := c.conn.c.MachineClient.Read(ctx, &machine.ReadRequest{Path: p})
	if err != nil {
		return "", false, absentOr(err)
	}

	var b strings.Builder
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return b.String(), true, nil
		}
		if err != nil {
			return "", false, absentOr(err)
		}
		if b.Len()+len(chunk.GetBytes()) > maxAttributeBytes {
			return "", false, nil
		}
		b.Write(chunk.GetBytes())
	}
}

// absentOr turns "that file is not there" into an absence and leaves every
// other failure an error.
//
// The distinction is the one errors.go exists for. A node that answered "no
// such file" is a node without that sensor; a node that did not answer is a
// node the whole view cannot be drawn for, and folding that into "no sensors"
// would tell an operator their kernel lacks a driver when their network
// dropped a packet. Talos hands the os error of a missing path to gRPC as it
// is, so it arrives as a refusal with codes.Unknown.
//
// A refusal to answer at all is not an absence either. A certificate whose
// role may not read files, or a node that does not serve the RPC, would
// otherwise turn into the same sentence about kernel drivers -- wrong, and
// wrong in the direction that sends the operator to the wrong machine.
func absentOr(err error) error {
	var te *Error
	if !errors.As(err, &te) || te.Kind != KindRejected {
		return err
	}
	switch te.Status {
	case codes.PermissionDenied, codes.Unauthenticated, codes.Unimplemented:
		return err
	default:
		return nil
	}
}

// readFiles reads many small files concurrently and returns the ones that
// exist, keyed by path.
func (c *ClusterClient) readFiles(ctx context.Context, paths []string) (map[string]string, error) {
	out := make(map[string]string, len(paths))
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(sensorReadConcurrency)

	seen := make(map[string]bool, len(paths))
	for _, p := range paths {
		if seen[p] {
			continue
		}
		seen[p] = true

		g.Go(func() error {
			content, ok, err := c.readFile(gctx, p)
			if err != nil {
				return err
			}
			if ok {
				mu.Lock()
				out[p] = content
				mu.Unlock()
			}
			return nil
		})
	}

	return out, g.Wait()
}

// chipDir is one hwmon chip while it is being discovered.
type chipDir struct {
	index int
	dir   string

	// files is what listing the directory found. probe is set when the
	// listing found nothing to go on, and the file names are then guessed.
	files map[string]bool
	probe bool

	name  string
	kind  SensorKind
	temps []int
	fans  []int
}

func (d *chipDir) has(file string) bool { return d.probe || d.files[file] }

// discoverSensors walks the node's hwmon tree and works out which files to
// read on every poll.
func (c *ClusterClient) discoverSensors(ctx context.Context) (*SensorLayout, error) {
	entries, _, err := c.listDir(ctx, hwmonClass)
	if err != nil {
		return nil, err
	}

	var chips []*chipDir
	for _, e := range entries {
		index, ok := numbered(e.name, "hwmon")
		if !ok {
			continue
		}
		chips = append(chips, &chipDir{index: index, dir: linkTarget(hwmonClass, e)})
	}
	// Numerically, so that hwmon10 follows hwmon9 rather than hwmon1, which
	// keeps the order a person reads the same as the order the kernel uses.
	sort.Slice(chips, func(i, j int) bool { return chips[i].index < chips[j].index })

	// Each chip's directory. /sys/class/hwmon holds symlinks, and the listing
	// goes to where the link points rather than to the link: whether the
	// node's walk follows a symlinked root is its implementation's business,
	// and the resolved directory is a directory either way.
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(sensorReadConcurrency)
	for _, chip := range chips {
		g.Go(func() error {
			listed, _, err := c.listDir(gctx, chip.dir+"/")
			if err != nil {
				return err
			}
			chip.files = make(map[string]bool, len(listed))
			for _, e := range listed {
				chip.files[e.name] = true
			}
			// A chip directory always carries its name. A listing without one
			// is not this chip's directory -- it is the node declining to
			// follow the link, answered with the link itself -- and the file
			// names are then probed rather than trusted.
			chip.probe = !chip.files["name"]
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// First round: every chip's name, and for a chip that could not be
	// listed, every input it might have.
	var first []string
	for _, chip := range chips {
		first = append(first, chip.dir+"/name")
		if chip.probe {
			for n := 1; n <= maxProbedTemps; n++ {
				first = append(first, fmt.Sprintf("%s/temp%d_input", chip.dir, n))
			}
			for n := 1; n <= maxProbedFans; n++ {
				first = append(first, fmt.Sprintf("%s/fan%d_input", chip.dir, n))
			}
		}
	}
	got, err := c.readFiles(ctx, first)
	if err != nil {
		return nil, err
	}

	// Second round: what the inputs are called, where their limits are, and
	// which drive a disk chip belongs to. Only for what exists.
	var second []string
	for _, chip := range chips {
		chip.name = strings.TrimSpace(got[chip.dir+"/name"])
		if chip.name == "" {
			chip.name = fmt.Sprintf("hwmon%d", chip.index)
		}
		chip.kind = ClassifyChip(chip.name)

		if chip.probe {
			for n := 1; n <= maxProbedTemps; n++ {
				if _, ok := got[fmt.Sprintf("%s/temp%d_input", chip.dir, n)]; ok {
					chip.temps = append(chip.temps, n)
				}
			}
			for n := 1; n <= maxProbedFans; n++ {
				if _, ok := got[fmt.Sprintf("%s/fan%d_input", chip.dir, n)]; ok {
					chip.fans = append(chip.fans, n)
				}
			}
		} else {
			chip.temps = inputsNamed(chip.files, "temp")
			chip.fans = inputsNamed(chip.files, "fan")
		}

		for _, n := range chip.temps {
			for _, suffix := range []string{"label", "max", "crit"} {
				file := fmt.Sprintf("temp%d_%s", n, suffix)
				if chip.has(file) {
					second = append(second, chip.dir+"/"+file)
				}
			}
		}
		for _, n := range chip.fans {
			if file := fmt.Sprintf("fan%d_label", n); chip.has(file) {
				second = append(second, chip.dir+"/"+file)
			}
		}
		// The parent device names the drive: an NVMe controller, or the SCSI
		// device drivetemp hangs off. Read through the device link, which the
		// node resolves on a read as it does for any path.
		if chip.kind == SensorDisk && chip.has("device") {
			second = append(second, chip.dir+"/device/model", chip.dir+"/device/serial")
		}
	}
	labels, err := c.readFiles(ctx, second)
	if err != nil {
		return nil, err
	}

	layout := &SensorLayout{}
	cpuTemps := false
	for _, chip := range chips {
		sc := SensorChip{Name: chip.name, Kind: chip.kind}
		if chip.kind == SensorDisk {
			sc.DeviceModel = strings.TrimSpace(labels[chip.dir+"/device/model"])
			sc.DeviceSerial = strings.TrimSpace(labels[chip.dir+"/device/serial"])
		}
		for _, n := range chip.temps {
			prefix := fmt.Sprintf("%s/temp%d_", chip.dir, n)
			label := strings.TrimSpace(labels[prefix+"label"])
			if label == "" {
				label = fmt.Sprintf("temp%d", n)
			}
			sc.Temps = append(sc.Temps, TempInput{
				Input:    prefix + "input",
				Label:    label,
				High:     millidegrees(labels, prefix+"max"),
				Critical: millidegrees(labels, prefix+"crit"),
			})
		}
		for _, n := range chip.fans {
			label := strings.TrimSpace(labels[fmt.Sprintf("%s/fan%d_label", chip.dir, n)])
			if label == "" {
				label = fmt.Sprintf("fan%d", n)
			}
			sc.Fans = append(sc.Fans, FanInput{Input: fmt.Sprintf("%s/fan%d_input", chip.dir, n), Label: label})
		}

		// A chip with nothing to read -- a voltage monitor, a battery, a
		// wireless card's power sensor -- is left out rather than listed
		// empty.
		if len(sc.Temps) == 0 && len(sc.Fans) == 0 {
			continue
		}
		if sc.Kind == SensorCPU && len(sc.Temps) > 0 {
			cpuTemps = true
		}
		layout.Chips = append(layout.Chips, sc)
	}

	if !cpuTemps {
		zones, err := c.thermalZones(ctx, layout.Chips)
		if err != nil {
			return nil, err
		}
		layout.Chips = append(layout.Chips, zones...)
	}

	return layout, nil
}

// thermalZones is the fallback for a node whose CPU has no hwmon chip.
//
// The thermal framework registers an hwmon twin for each zone when the kernel
// is built with CONFIG_THERMAL_HWMON, so on most machines this finds nothing
// new. Where it is not, a zone is the only place the CPU's temperature is --
// which is what an ARM board like a Raspberry Pi looks like. A zone whose hwmon
// twin already reported is skipped, so nothing is listed twice.
func (c *ClusterClient) thermalZones(ctx context.Context, known []SensorChip) ([]SensorChip, error) {
	entries, _, err := c.listDir(ctx, thermalClass)
	if err != nil {
		return nil, err
	}

	type zone struct {
		index int
		name  string
		dir   string
	}
	var zones []zone
	var paths []string
	for _, e := range entries {
		index, ok := numbered(e.name, "thermal_zone")
		if !ok {
			continue
		}
		z := zone{index: index, name: e.name, dir: linkTarget(thermalClass, e)}
		zones = append(zones, z)
		paths = append(paths, z.dir+"/type", z.dir+"/temp")
	}
	sort.Slice(zones, func(i, j int) bool { return zones[i].index < zones[j].index })

	got, err := c.readFiles(ctx, paths)
	if err != nil {
		return nil, err
	}

	have := map[string]bool{}
	for _, chip := range known {
		have[chip.Name] = true
	}

	var out []SensorChip
	for _, z := range zones {
		typ := strings.TrimSpace(got[z.dir+"/type"])
		if typ == "" {
			continue
		}
		if _, ok := got[z.dir+"/temp"]; !ok {
			continue
		}
		if have[strings.ReplaceAll(typ, "-", "_")] {
			continue
		}
		out = append(out, SensorChip{
			Name:  typ,
			Kind:  ClassifyChip(typ),
			Temps: []TempInput{{Input: z.dir + "/temp", Label: z.name}},
		})
	}
	return out, nil
}

// readSensors reads every input of a layout.
//
// stale reports that an input the layout names is no longer there, which means
// the layout no longer describes the node -- a driver was unloaded, a disk was
// pulled -- and should be discovered again. An input that is there and does
// not parse is only skipped: some drivers answer a read of a sleeping device
// with nothing, and one silent sensor is not a reason to walk the tree again.
func (c *ClusterClient) readSensors(ctx context.Context, layout *SensorLayout) (readings []ChipReading, stale bool, err error) {
	var paths []string
	for _, chip := range layout.Chips {
		for _, t := range chip.Temps {
			paths = append(paths, t.Input)
		}
		for _, f := range chip.Fans {
			paths = append(paths, f.Input)
		}
	}

	got, err := c.readFiles(ctx, paths)
	if err != nil {
		return nil, false, err
	}

	for _, chip := range layout.Chips {
		r := ChipReading{
			Name:         chip.Name,
			Kind:         chip.Kind,
			DeviceModel:  chip.DeviceModel,
			DeviceSerial: chip.DeviceSerial,
		}
		for _, t := range chip.Temps {
			raw, ok := got[t.Input]
			if !ok {
				stale = true
				continue
			}
			milli, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
			if err != nil {
				continue
			}
			r.Temps = append(r.Temps, TempReading{
				Label:    t.Label,
				Celsius:  float64(milli) / 1000,
				High:     t.High,
				Critical: t.Critical,
			})
		}
		for _, f := range chip.Fans {
			raw, ok := got[f.Input]
			if !ok {
				stale = true
				continue
			}
			rpm, err := strconv.Atoi(strings.TrimSpace(raw))
			if err != nil {
				continue
			}
			r.Fans = append(r.Fans, FanReading{Label: f.Label, RPM: rpm})
		}
		readings = append(readings, r)
	}

	return readings, stale, nil
}

// inputsNamed returns the N of every <kind>N_input in a listing, in order.
func inputsNamed(files map[string]bool, kind string) []int {
	var out []int
	for name := range files {
		rest, ok := strings.CutPrefix(name, kind)
		if !ok {
			continue
		}
		digits, ok := strings.CutSuffix(rest, "_input")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(digits)
		if err != nil || n <= 0 {
			continue
		}
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// numbered parses "hwmon12" as 12.
func numbered(name, prefix string) (int, bool) {
	digits, ok := strings.CutPrefix(name, prefix)
	if !ok || digits == "" {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// linkTarget is the directory a class entry points at. Entries under
// /sys/class are symlinks into /sys/devices, and the link the node reported is
// relative to the class directory; an entry that is not a link is used as it
// is.
func linkTarget(class string, e dirEntry) string {
	switch {
	case e.link == "":
		return path.Join(class, e.name)
	case path.IsAbs(e.link):
		return path.Clean(e.link)
	default:
		return path.Join(class, e.link)
	}
}

// millidegrees reads a threshold file as degrees Celsius, or nil when there is
// none. A threshold at or below zero is a chip that does not set one -- some
// report 0, some a sentinel -- and a red line at 0 °C would be read as a
// machine in trouble.
func millidegrees(files map[string]string, p string) *float64 {
	raw, ok := files[p]
	if !ok {
		return nil
	}
	milli, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || milli <= 0 {
		return nil
	}
	c := float64(milli) / 1000
	return &c
}
