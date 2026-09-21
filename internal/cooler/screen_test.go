package cooler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// slot builds a bucket-query reply: occupied at an address, of a size.
func slot(index, address, size int) []byte {
	reply := make([]byte, reportLen)
	reply[0], reply[1] = 0x31, 0x04
	reply[14] = byte(index)
	reply[15] = byte(index + 1) // the asset index, which is what marks it used
	reply[16] = 0x02
	reply[17], reply[18] = byte(address&0xFF), byte(address>>8)
	reply[19], reply[20] = byte(size&0xFF), byte(size>>8)
	reply[21], reply[22] = 0x01, 0x01
	return reply
}

// blank is an unoccupied slot: everything from byte 15 is zero.
func blank() []byte {
	reply := make([]byte, reportLen)
	reply[0], reply[1] = 0x31, 0x04
	return reply
}

func TestAnEmptySlotIsPreferred(t *testing.T) {
	slots := [][]byte{slot(0, 0, 7), slot(1, 22, 22), blank(), slot(3, 66, 22)}
	require.Equal(t, 2, free(slots, -1))
}

func TestTheSlotOnScreenIsNeverFree(t *testing.T) {
	/*
		The transfer takes about a second and the panel keeps showing its
		current slot throughout. Write into that slot and the screen is blank
		for the duration -- which is what a dashboard updating every two
		seconds looks like when placement walks into the image it is
		replacing.
	*/
	slots := [][]byte{slot(0, 0, 7), blank(), blank()}
	require.Equal(t, 2, free(slots, 1))
}

func TestAFullScreenReportsNoFreeSlot(t *testing.T) {
	// -1 rather than 0: "all taken" leads to clearing one, and a caller that
	// cannot tell the two apart writes over an image nobody asked it to.
	slots := [][]byte{slot(0, 0, 7), slot(1, 22, 22)}
	require.Equal(t, -1, free(slots, -1))
}

func TestAnImageThatStillFitsKeepsItsAddress(t *testing.T) {
	/*
		The device owns the memory map and refuses an address it did not
		arrive at itself -- that refusal is 0x04, and it looks exactly like
		every other thing that goes wrong here. Reusing a slot's own space is
		the first and cheapest answer.
	*/
	slots := [][]byte{slot(0, 40, 20), slot(1, 100, 10)}
	address, ok := place(slots, 0, 15)
	require.True(t, ok)
	require.Equal(t, 40, address, "an image that fits where it is was moved")
}

func TestAGrowingImageGoesAfterEverythingElse(t *testing.T) {
	// It no longer fits in place and would overlap its neighbour, so it goes
	// beyond the highest thing on the device.
	slots := [][]byte{slot(0, 0, 10), slot(1, 10, 30)}
	address, ok := place(slots, 0, 25)
	require.True(t, ok)
	require.Equal(t, 40, address)
}

func TestRoomAtTheStartIsUsedBeforeGivingUp(t *testing.T) {
	// Everything sits high up and there is space below it. Wrapping to the
	// beginning is the last placement before declaring the memory unusable.
	slots := [][]byte{slot(0, capacity-20, 20)}
	address, ok := place(slots, 0, 15)
	require.True(t, ok)
	require.Equal(t, capacity-20, address, "it fits where it is and should not have moved")

	slots = [][]byte{slot(0, 300, 5), slot(1, 305, capacity-310)}
	address, ok = place(slots, 0, 100)
	require.True(t, ok)
	require.Equal(t, 0, address)
}

func TestMemoryTooFullToPlaceAnythingSaysSo(t *testing.T) {
	/*
		The caller clears the screen and starts again rather than sending an
		address the device will refuse.

		Note what does *not* qualify: an image that fills the slot it already
		occupies still fits there, however large. It has to be bigger than its
		own slot, overlap its neighbours, and find room neither above nor
		below.
	*/
	slots := [][]byte{slot(0, 0, 10), slot(1, 10, capacity-10)}
	_, ok := place(slots, 0, capacity)
	require.False(t, ok)
}

func TestASlotIsOnlyVacantWhenEverythingAfterByteFifteenIsZero(t *testing.T) {
	require.True(t, vacant(blank()))
	require.False(t, vacant(slot(0, 0, 7)))
}
