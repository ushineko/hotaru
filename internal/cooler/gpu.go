package cooler

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
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
GPUTemperature reads the graphics card, in degrees.

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
func GPUTemperature(ctx context.Context) (int, error) {
	for _, sensor := range GPUSensors {
		if t, err := sensor.Temperature(); err == nil {
			return t, nil
		}
	}
	return nvidiaSMI(ctx)
}

// smiTimeout bounds the fallback. A sensor read that hangs must not hold up a
// screen update, let alone the shutdown it would be blocking during.
const smiTimeout = 2 * time.Second

func nvidiaSMI(ctx context.Context) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, smiTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=temperature.gpu", "--format=csv,noheader").Output()
	if err != nil {
		return 0, fmt.Errorf("no graphics card temperature: %w", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0, errors.New("no graphics card temperature: nvidia-smi said nothing")
	}
	t, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, fmt.Errorf("no graphics card temperature: %w", err)
	}
	return t, nil
}
