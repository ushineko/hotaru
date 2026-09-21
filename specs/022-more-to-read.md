# Spec 022: more to read

**Issue**: [#51](https://github.com/ushineko/hotaru/issues/51)

## Status: COMPLETE

## Context

The dashboard shows four numbers because the Python showed four numbers, and
`dashboard.Reading` is four pairs of fields to match. Two of them say how hot
the machine is and none says how hard it is working.

That is about to matter. The dashboard editor lets somebody choose what goes
in each slot, and a menu of four -- two of which are temperatures of the same
kind -- is not a choice worth offering. So the readings come first, and this
spec is only about what the machine can say.

### A reading is named, not a field

`Reading{CPU int; CPUOK bool; ...}` cannot answer "what is in slot 2" without
a switch statement that has to be kept in step with the struct. A reading
becomes a value looked up by name, and a slot holds a name.

The `OK` half stays, as the absence of an entry: every field can be missing,
and a missing one draws a placeholder rather than a zero. A sensor that has
gone away must not stop the panel, because the screen is decorative and the
other numbers are still true.

### Where the new numbers come from

**CPU %** is `/proc/stat`: the busy and idle jiffies since boot, differenced
against the last sample. Utilisation is a rate, so the first reading after
start-up has nothing to difference against and is absent -- the same absence
a missing sensor produces, and the panel already knows what to do with it.

**GPU %** is `gpu_busy_percent` under the card's sysfs node on AMD, and
`nvidia-smi` otherwise -- the same call that already fetches the temperature,
asked for two numbers instead of one. That is the whole cost: this project
refuses to spawn a process per frame on the write path, and this is one spawn
per dashboard tick either way.

**Pump %, fan RPM and fan %** are already in `cooler.Status` and have never
been shown.

### Why a command for it

`hotaru readings` prints what the machine will say, once. It is how somebody
finds out whether their graphics card reports utilisation before they put it
on a panel and wonder why it draws `--`, and it is how this spec was verified
on hardware.

## Requirements

**R1. A reading is looked up by name.** `readings.Source` names each number;
`readings.Set` records one; an unrecorded source is absent.

**R2. Nine sources.** Coolant, CPU °C, CPU %, GPU °C, GPU %, pump RPM, pump %,
fan RPM, fan %.

**R3. Utilisation is a rate, and says so when it cannot be one.** The first
sample after start-up is absent rather than zero or a since-boot average.

**R4. One process spawn, not two.** Where `nvidia-smi` answers, temperature
and utilisation come from the same invocation.

**R5. A sensor that is not there costs its own number and nothing else.**
Unchanged from spec 013, and now true per source rather than per struct field.

**R6. Both shells.** `hotaru readings` and a route, so the parity rule holds.

**R7. The panel draws what it drew.** This spec adds no pixels: the same four
numbers in the same places, read through the new lookup.

## Acceptance Criteria

- [x] AC1. A `Reading` returns a value for a source that was set and reports
      absence for one that was not.
- [x] AC2. CPU utilisation is computed from two `/proc/stat` samples against a
      fixture, with a known answer.
- [x] AC3. The first CPU utilisation sample is absent.
- [x] AC4. GPU utilisation is read from `gpu_busy_percent` where it exists,
      against a fixture directory.
- [x] AC5. Where it does not, temperature and utilisation are parsed from one
      `nvidia-smi` answer, against a recorded line of its output.
- [x] AC6. A machine with no graphics card reports both as absent and nothing
      else changes.
- [x] AC7. Pump %, fan RPM and fan % are present in a reading taken from a
      cooler status.
- [x] AC8. `hotaru readings` lists every source with its value or `--`, and
      the parity test covers its route.
- [x] AC9. The rendered dashboard is unchanged: every existing render test
      still passes and still asserts the same thing, with only the way a
      reading is *constructed* changed. The content hashes are what prove it:
      the gate that stops hotaru writing a frame that says nothing would fail
      first if a pixel had moved.
- [x] AC10. Verified on the development machine: `hotaru readings` shows a CPU
      percentage that moves under load and a GPU percentage that matches
      `nvidia-smi`.

## Verified on hardware

Development machine, the service restarted:

	SOURCE    NAME     VALUE
	coolant   Coolant  38.5 °C
	cpu_c     CPU      50 °C
	cpu_pct   CPU      3 %
	gpu_c     GPU      39 °C
	gpu_pct   GPU      1 %
	pump_rpm  Pump     2658 RPM
	pump_pct  Pump     84 %
	fan_rpm   Fan      1298 RPM
	fan_pct   Fan      54 %

Four busy loops took the processor from 3% to 19% -- about what four threads
of this machine's count comes to -- and it fell back to 6% when they stopped.
The graphics card read 2% against `nvidia-smi`'s own 2% at the same moment,
which is the check that matters: the number comes from parsing somebody else's
output format, and the only proof it is the right column is asking them both.

The panel drew exactly what it drew before, which is the point of R7: this
spec is about what the machine can say, not about what the screen does with
it.

## Risks & Assumptions

- **`/proc/stat` is Linux.** So is everything else in this program; the reader
  returns absence rather than failing on a machine without it.
- **`nvidia-smi` output is a format, not an API.** It is already parsed for
  the temperature; asking for a second column widens an existing exposure
  rather than creating one. A line that does not parse is absence.
- **Utilisation between two dashboard ticks is an average over two seconds**,
  not an instant. That is what a panel across a room wants.
- **Rollback** is a revert; nothing is stored and no file format changes.

## Alternatives Considered

Considered NVML through cgo for the NVIDIA numbers; rejected for the reason
spec 013 gave -- cgo for one integer -- which holds for two.

Considered keeping the struct and adding five fields; rejected because the
editor needs to address a reading by name, and the switch statement that would
bridge the two is the thing that goes out of step.
