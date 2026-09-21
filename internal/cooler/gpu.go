package cooler

import (
	"context"
	"os/exec"
	"time"

	"github.com/ushineko/hotaru/internal/readings"
)

/*
GPUSensors are the kernel's ways of knowing a graphics card's temperature, in
the order they are tried.

AMD labels its sensors; nouveau exposes one unlabelled temperature. NVIDIA's
proprietary driver registers no hwmon at all, which is why there is a fallback
below rather than a longer list here.
*/
var GPUSensors = []Sensor{
	{Chip: "amdgpu", Label: "edge"},
	{Chip: "amdgpu"},
	{Chip: "nouveau"},
}

/*
Graphics reads the card's temperature in degrees and its utilisation as a
percentage, by whichever route the machine has.

The kernel first. Where the kernel has nothing -- which is every machine
running NVIDIA's own driver, including the one this was written on -- it asks
nvidia-smi, and that is a process spawn on a path this project otherwise
refuses to spawn processes on.

The refusal was about `liquidctl` on the *write* path: 105 ms of process
startup blocking every frame. This is a sensor read at the dashboard's cadence,
measured at 18 ms wall and about 2 ms of CPU, once every two seconds. The
alternative is NVML, which is cgo, for one integer.

A machine with neither answers "unknown", and the dashboard draws a placeholder
rather than a zero. See spec 013.
*/
func Graphics(ctx context.Context) Card {
	var card Card
	for _, sensor := range GPUSensors {
		if t, err := sensor.Temperature(); err == nil {
			card.Temperature, card.TemperatureOK = float64(t), true
			break
		}
	}
	if load, ok := readings.Busy(); ok {
		card.Load, card.LoadOK = load, true
	}
	if card.TemperatureOK && card.LoadOK {
		return card
	}

	/*
		One spawn for both numbers.

		The refusal to spawn a process per frame was about the write path
		(spec 012). This is one process per dashboard tick either way, so
		asking it for the second number is free -- and asking twice would
		not be.
	*/
	temperature, load, gotTemp, gotLoad := nvidiaSMI(ctx)
	if !card.TemperatureOK && gotTemp {
		card.Temperature, card.TemperatureOK = temperature, true
	}
	if !card.LoadOK && gotLoad {
		card.Load, card.LoadOK = load, true
	}
	return card
}

// Card is what the graphics card had to say. Either number can be missing,
// and a machine with no card at all reports neither.
type Card struct {
	Temperature   float64
	TemperatureOK bool
	Load          float64
	LoadOK        bool
}

// smiTimeout bounds the fallback. A sensor read that hangs must not hold up a
// screen update, let alone the shutdown it would be blocking during.
const smiTimeout = 2 * time.Second

func nvidiaSMI(ctx context.Context) (temperature, load float64, gotTemp, gotLoad bool) {
	ctx, cancel := context.WithTimeout(ctx, smiTimeout)
	defer cancel()

	//nolint:gosec // both arguments are constants; nothing here is input
	out, err := exec.CommandContext(ctx, "nvidia-smi",
		readings.SMIFlag, "--format=csv,noheader").Output()
	if err != nil {
		return 0, 0, false, false
	}
	return readings.ParseSMI(string(out))
}
