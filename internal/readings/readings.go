/*
Package readings is what the machine will say about itself.

One number per source, each taken independently and each allowed to be
missing: a sensor that has gone away costs its own reading and nothing else,
because the panel is decorative and the rest of the numbers are still true.

Looked up by name rather than held in fields, because a dashboard slot holds a
name. A struct of pairs cannot answer "what is in slot 2" without a switch
somebody has to keep in step with it.
*/
package readings

import "fmt"

// Source names a number this machine can report.
type Source string

// The sources. Their strings are written into saved dashboards, so they are
// part of the file format and do not change.
const (
	Coolant  Source = "coolant"
	CPUTemp  Source = "cpu_c"
	CPULoad  Source = "cpu_pct"
	GPUTemp  Source = "gpu_c"
	GPULoad  Source = "gpu_pct"
	PumpRPM  Source = "pump_rpm"
	PumpDuty Source = "pump_pct"
	FanRPM   Source = "fan_rpm"
	FanDuty  Source = "fan_pct"
	MemUsed  Source = "mem_pct"
	MemBytes Source = "mem_gb"
)

// All is every source, in the order a listing shows them: the coolant first,
// because that is the number the screen was built around.
var All = []Source{
	Coolant, CPUTemp, CPULoad, GPUTemp, GPULoad, MemUsed, MemBytes,
	PumpRPM, PumpDuty, FanRPM, FanDuty,
}

/*
Describe is what a source is called and what it is measured in.

Here rather than in the window: the CLI prints the same words, and a label
that differed between the two would be two names for one number.
*/
func Describe(s Source) (label, unit string) {
	switch s {
	case Coolant:
		return "Coolant", "°C"
	case CPUTemp:
		return "CPU", "°C"
	case CPULoad:
		return "CPU", "%"
	case GPUTemp:
		return "GPU", "°C"
	case GPULoad:
		return "GPU", "%"
	case MemUsed:
		return "Memory", "%"
	case MemBytes:
		return "Memory", "GB"
	case PumpRPM:
		return "Pump", "RPM"
	case PumpDuty:
		return "Pump", "%"
	case FanRPM:
		return "Fan", "RPM"
	case FanDuty:
		return "Fan", "%"
	}
	return string(s), ""
}

/*
Width is how many characters to reserve for a source's reading.

The panel redraws every couple of seconds, and a value drawn to the width of
whatever it happens to say moves every time the character count changes -- `9`
to `10`, `99` to `100`. Reserving the width it *can* take holds it still.

The widest plausible reading rather than the widest possible one. A field is a
minimum and not a limit, so being wrong here costs one shift at the extreme
instead of a layout that cannot hold what it is given: a coolant at 100 °C is
a machine with a bigger problem than a number that moved.

Here rather than in the renderer because the format that decides it is here.
Text puts one decimal on a coolant and none on anything else, and a width
worked out somewhere else would be a second opinion about the same string.
*/
func Width(s Source) int {
	switch s {
	case Coolant:
		return 4 // 37.5
	case PumpRPM, FanRPM:
		return 4 // 2608
	case CPUTemp, GPUTemp, CPULoad, GPULoad, PumpDuty, FanDuty, MemUsed, MemBytes:
		return 3 // 100
	}
	return 3
}

// Reading is what the machine said this time round.
type Reading struct {
	values map[Source]float64
}

// Set records one number.
func (r *Reading) Set(s Source, v float64) {
	if r.values == nil {
		r.values = map[Source]float64{}
	}
	r.values[s] = v
}

// Value is a number and whether the machine had it to give.
func (r Reading) Value(s Source) (float64, bool) {
	v, ok := r.values[s]
	return v, ok
}

/*
Placeholder is what the panel draws for a number the machine did not have.

It is the point of the whole shape. A screen that drew zero for a pump it
could not read would be reporting the most alarming number this machine can
show, on no evidence.
*/
const Placeholder = "--"

// Text is a reading as the panel draws it: one decimal for a coolant
// temperature, whole numbers for everything else, and Placeholder for what is
// not there.
func (r Reading) Text(s Source) string {
	v, ok := r.Value(s)
	if !ok {
		return Placeholder
	}
	if s == Coolant {
		return fmt.Sprintf("%.1f", v)
	}
	return fmt.Sprintf("%.0f", v)
}
