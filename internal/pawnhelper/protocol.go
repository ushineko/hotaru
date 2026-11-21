/*
Package pawnhelper is hotaru's PawnIO helper on Windows (spec 083): a
service, hotaru-pawnio, that owns the devices sanshoku reaches through
PawnIO -- an MSI board's RGB header on its Super I/O chip, and memory
lighting on the chipset's SMBus -- and draws on them what the service sends.

PawnIO's device opens for SYSTEM and administrators only, and the installed
service runs as NT SERVICE\hotaru with a user's rights. The helper runs as
LocalSystem so the service need not. It does one thing: it lists the
devices it found, shows a frame on one, or hands one back to its firmware.
It offers no bus, no register and no module function; a request names a
device it listed and lights that device's own keys.

Its pipe opens for NT SERVICE\hotaru and administrators, and refuses remote
clients. The service checks that the process serving the pipe is the one
the service manager started for hotaru-pawnio before it believes a word.

The helper is installed by packaging (fynstall.yaml), as the service is.
Off Windows sanshoku reaches these devices without PawnIO, and there is no
helper.
*/
package pawnhelper

import (
	"errors"

	"github.com/ushineko/sanshoku/lighting"
)

const (
	// Service is the helper's service name.
	Service = "hotaru-pawnio"
	// Pipe is where it listens.
	Pipe = `\\.\pipe\hotaru-pawnio`
	// Protocol is the version of its lines.
	Protocol = 1
	// Command is hotaru's hidden subcommand the service runs.
	Command = "pawnio-helper"
)

// Bounds on what crosses the pipe, each well above what a device here has:
// the board header is one light, a memory module a dozen.
const (
	maxLine    = 64 << 10
	maxPixels  = 1024
	maxDevices = 32
	maxText    = 256
)

// The requests.
const (
	opList    = "list"
	opFrame   = "frame"
	opRelease = "release"
)

// request is one line from the service.
type request struct {
	V  int    `json:"v"`
	Op string `json:"op"`
	// Device is a device's ID, as list gave it.
	Device string `json:"device,omitempty"`
	// Pixels are the frame's lights: ID, red, green, blue.
	Pixels [][4]uint8 `json:"pixels,omitempty"`
}

// response is one line back.
type response struct {
	V       int      `json:"v"`
	Error   string   `json:"error,omitempty"`
	Gone    bool     `json:"gone,omitempty"`
	Devices []device `json:"devices,omitempty"`
}

// device is a device the helper found, as the service sees it.
type device struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Vendor  uint16          `json:"vendor,omitempty"`
	Product uint16          `json:"product,omitempty"`
	Path    string          `json:"path,omitempty"`
	Keys    []lighting.Key  `json:"keys"`
	Zones   []lighting.Zone `json:"zones,omitempty"`
	FloorMS int64           `json:"floorMs"`
}

/*
modules are the PawnIO modules the helper lets sanshoku load, by SHA-256:
PawnIO.Modules 0.2.12's, as packaging puts them beside the program
(third_party/pawnio). PawnIO checks a module's signature and loads any its
author ever signed, older faulty ones included; this pins the three to the
release they were tested from.
*/
var modules = map[string]string{
	"LpcIO.bin":      "b3896a1cab0d808fca31fe2ebcae045d59dac690da87b17c858bb8da357eb45e",
	"SmbusPIIX4.bin": "91f9b4b1c39e3d399ce48477a89d8f6bd3e58a2241064a4daec3a1513dff56e5",
	"SmbusI801.bin":  "a0f7d066e7efda28c0e754c1f52dbd8dc280d388ba8575b514a80cb67490530c",
}

// ErrGone is a device the helper had and has not now. The service treats it
// as sanshoku.ErrGone.
var ErrGone = errors.New("the helper's device is gone")

func pixels(px []lighting.Pixel) [][4]uint8 {
	out := make([][4]uint8, len(px))
	for i, p := range px {
		out[i] = [4]uint8{p.ID, p.R, p.G, p.B}
	}
	return out
}

func unpixels(in [][4]uint8) []lighting.Pixel {
	out := make([]lighting.Pixel, len(in))
	for i, p := range in {
		out[i] = lighting.Pixel{ID: p[0], R: p[1], G: p[2], B: p[3]}
	}
	return out
}
