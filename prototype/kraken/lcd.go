package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"os"
)

const (
	lcdTotalMemory  = 24320
	bulkBufferSize  = 2 * 1024 * 1024
	maxReadAttempts = 12
	lcdSide         = 640
)

// hidWrite sends one 64-byte report.
func hidWrite(f *os.File, data ...byte) error {
	report := make([]byte, reportLen)
	copy(report, data)
	_, err := f.Write(report)
	return err
}

// hidWriteThenRead sends a report and returns the next one, as liquidctl does.
func hidWriteThenRead(f *os.File, data ...byte) ([]byte, error) {
	if err := hidWrite(f, data...); err != nil {
		return nil, err
	}
	buf := make([]byte, reportLen)
	if _, err := f.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// readUntil returns the first report whose two-byte prefix matches.
func readUntil(f *os.File, a, b byte) ([]byte, error) {
	buf := make([]byte, reportLen)
	for range maxReadAttempts {
		if _, err := f.Read(buf); err != nil {
			return nil, err
		}
		if buf[0] == a && buf[1] == b {
			out := make([]byte, reportLen)
			copy(out, buf)
			return out, nil
		}
	}
	return nil, fmt.Errorf("no %02x%02x report in %d reads", a, b, maxReadAttempts)
}

func queryBuckets(f *os.File) ([][]byte, error) {
	out := make([][]byte, 16)
	for i := range 16 {
		r, err := hidWriteThenRead(f, 0x30, 0x04, byte(i))
		if err != nil {
			return nil, fmt.Errorf("query bucket %d: %w", i, err)
		}
		out[i] = r
	}
	return out, nil
}

// A bucket is unoccupied when everything from byte 15 on is zero.
func findUnoccupied(buckets [][]byte) int {
	for i, b := range buckets {
		empty := true
		for _, v := range b[15:] {
			if v != 0 {
				empty = false
				break
			}
		}
		if empty {
			return i
		}
	}
	return -1
}

func deleteBucket(f *os.File, index int) (bool, error) {
	if err := hidWrite(f, 0x32, 0x02, byte(index)); err != nil {
		return false, err
	}
	msg, err := readUntil(f, 0x33, 0x02)
	if err != nil {
		return false, err
	}
	return msg[14] == 0x01, nil
}

// prepareBucket mirrors liquidctl: a delete that fails moves to the next
// bucket; one that had data is deleted twice.
func prepareBucket(f *os.File, index int, filled bool) (int, error) {
	if index >= 16 {
		return 0, fmt.Errorf("reached max bucket")
	}
	ok, err := deleteBucket(f, index)
	if err != nil {
		return 0, err
	}
	if !ok {
		return prepareBucket(f, index+1, true)
	}
	if filled {
		return prepareBucket(f, index, false)
	}
	return index, nil
}

func setupBucket(f *os.File, start, end int, addr, size [2]byte) (bool, []byte, error) {
	r, err := hidWriteThenRead(f, 0x32, 0x01, byte(start), byte(end),
		addr[0], addr[1], size[0], size[1], 0x01)
	if err != nil {
		return false, nil, err
	}
	return r[14] == 0x01, r, nil
}

func switchBucket(f *os.File, index int, mode byte) (bool, error) {
	r, err := hidWriteThenRead(f, 0x38, 0x01, mode, byte(index))
	if err != nil {
		return false, err
	}
	return r[14] == 0x01, nil
}

func u16(b []byte) int { return int(binary.LittleEndian.Uint16(b)) }

// bucketMemoryOffset is liquidctl's placement logic, which the device needs
// because bucket addresses move around between writes.
func bucketMemoryOffset(buckets [][]byte, index, dataSize int) (int, bool) {
	cur := buckets[index]
	curOffset := u16(cur[17:19])
	curSize := u16(cur[19:21])

	if dataSize <= curSize {
		return curOffset, true
	}

	minOccupied, maxOccupied, overlap := curOffset, 0, false
	for i, b := range buckets {
		start := u16(b[17:19])
		end := start + u16(b[19:21])
		if end > maxOccupied {
			maxOccupied = end
		}
		if start < minOccupied {
			minOccupied = start
		}
		if (start > curOffset && start < curOffset+dataSize) ||
			(start < curOffset && end > start) ||
			(start == curOffset && i != index) {
			overlap = true
		}
	}
	if !overlap {
		return curOffset, true
	}
	if maxOccupied+dataSize < lcdTotalMemory {
		return maxOccupied, true
	}
	if dataSize < minOccupied {
		return 0, true
	}
	return 0, false
}

// pushGIF runs the whole transfer: HID sets up a bucket, the image goes over
// the bulk endpoint, HID closes the transfer.
func pushGIF(hid *os.File, usb *usbDevice, data []byte) error {
	if _, err := hidWriteThenRead(hid, 0x36, 0x03); err != nil {
		return fmt.Errorf("pre-transfer: %w", err)
	}
	buckets, err := queryBuckets(hid)
	if err != nil {
		return err
	}

	index := findUnoccupied(buckets)
	filled := index == -1
	if filled {
		index = 0
	}
	index, err = prepareBucket(hid, index, filled)
	if err != nil {
		return fmt.Errorf("prepare bucket: %w", err)
	}

	header := []byte{0x12, 0xFA, 0x01, 0xE8, 0xAB, 0xCD, 0xEF, 0x98, 0x76, 0x54, 0x32, 0x10}
	info := make([]byte, 8)
	info[0] = 0x01 // gif
	binary.LittleEndian.PutUint32(info[4:], uint32(len(data)))
	header = append(header, info...)

	packets := (len(header) + len(data) + 1023) / 1024
	offset, ok := bucketMemoryOffset(buckets, index, packets)
	if !ok {
		return fmt.Errorf("no room in LCD memory; a reset would be needed")
	}

	var addr, size [2]byte
	binary.LittleEndian.PutUint16(addr[:], uint16(offset))
	binary.LittleEndian.PutUint16(size[:], uint16(packets))
	ok, resp, err := setupBucket(hid, index, index+1, addr, size)
	if err != nil {
		return fmt.Errorf("setup bucket: %w", err)
	}
	if !ok {
		return fmt.Errorf("device refused bucket %d addr %d packets %d: reply[12:18]=% x",
			index, offset, packets, resp[12:18])
	}

	if _, err := hidWriteThenRead(hid, 0x36, 0x01, byte(index)); err != nil {
		return fmt.Errorf("start transfer: %w", err)
	}
	if _, err := usb.bulkWrite(0x02, header, 5000); err != nil {
		return err
	}
	for i := 0; i < len(data); i += bulkBufferSize {
		end := min(i+bulkBufferSize, len(data))
		if _, err := usb.bulkWrite(0x02, data[i:end], 5000); err != nil {
			return err
		}
	}
	if _, err := hidWriteThenRead(hid, 0x36, 0x02); err != nil {
		return fmt.Errorf("end transfer: %w", err)
	}
	// Point the screen at the bucket just written. Without this the transfer
	// lands and nothing changes -- the device keeps showing whatever it was
	// showing, which looks exactly like a failed write.
	if ok, err := switchBucket(hid, index, 0x04); err != nil {
		return fmt.Errorf("show bucket: %w", err)
	} else if !ok {
		return fmt.Errorf("device refused to show bucket %d", index)
	}

	/*
		Free the bucket we were showing before this one.

		liquidctl allocates and never reclaims: every push takes the next
		address up, and the refusals begin when the climb reaches the end of
		24 KB. Measured on this cooler -- the addresses in the refusals were
		2620, 2625, 2637, 2643, rising with each push. Releasing the previous
		bucket once the new one is on screen keeps a dashboard to two.
	*/
	if recycle && lastShown >= 0 && lastShown != index {
		if _, err := deleteBucket(hid, lastShown); err != nil {
			return fmt.Errorf("release previous bucket: %w", err)
		}
	}
	lastShown = index
	return nil
}

// recycle frees the previously displayed bucket after switching away from it.
var recycle = false

// lastShown is the bucket currently on screen, or -1 if unknown.
var lastShown = -1

// testCard is an unmistakable 640x640 frame: four quadrants and a black cross.
func testCard() []byte {
	img := image.NewPaletted(image.Rect(0, 0, lcdSide, lcdSide), color.Palette{
		color.RGBA{0, 0, 0, 255},
		color.RGBA{220, 30, 30, 255},
		color.RGBA{30, 200, 60, 255},
		color.RGBA{40, 90, 240, 255},
		color.RGBA{240, 240, 240, 255},
	})
	half := lcdSide / 2
	for y := range lcdSide {
		for x := range lcdSide {
			var i uint8
			switch {
			case x < half && y < half:
				i = 1
			case x >= half && y < half:
				i = 2
			case x < half && y >= half:
				i = 3
			default:
				i = 4
			}
			if abs(x-half) < 12 || abs(y-half) < 12 {
				i = 0
			}
			img.SetColorIndex(x, y, i)
		}
	}
	var buf bytes.Buffer
	// Config makes the encoder write a *global* colour table. Without it Go
	// emits a local table per frame, which Pillow never does and which the
	// firmware's decoder appears not to handle.
	_ = gif.EncodeAll(&buf, &gif.GIF{
		Image:     []*image.Paletted{img},
		Delay:     []int{10},
		LoopCount: 0,
		Config: image.Config{
			ColorModel: img.Palette,
			Width:      lcdSide,
			Height:     lcdSide,
		},
	})
	return buf.Bytes()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

/*
pushPinned writes one fixed bucket at a fixed address, every time.

The placement logic exists to fit images of differing sizes alongside each
other in 24 KB of device memory. A dashboard is one frame, the same size,
rewritten forever -- so it can own bucket 0 at address 0 and never negotiate
with anything. That removes the bucket query (16 HID round trips, most of the
push's cost) and the arithmetic that appears to be what the device refuses.
*/
func pushPinned(hid *os.File, usb *usbDevice, data []byte, bucket int) error {
	if _, err := hidWriteThenRead(hid, 0x36, 0x03); err != nil {
		return fmt.Errorf("pre-transfer: %w", err)
	}
	// Twice: liquidctl's own comment says a bucket that held data needs two.
	if _, err := deleteBucket(hid, bucket); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	if _, err := deleteBucket(hid, bucket); err != nil {
		return fmt.Errorf("delete again: %w", err)
	}

	header := []byte{0x12, 0xFA, 0x01, 0xE8, 0xAB, 0xCD, 0xEF, 0x98, 0x76, 0x54, 0x32, 0x10}
	info := make([]byte, 8)
	info[0] = 0x01
	binary.LittleEndian.PutUint32(info[4:], uint32(len(data)))
	header = append(header, info...)

	packets := (len(header) + len(data) + 1023) / 1024
	var addr, size [2]byte
	binary.LittleEndian.PutUint16(size[:], uint16(packets))

	ok, resp, err := setupBucket(hid, bucket, bucket+1, addr, size)
	if err != nil {
		return fmt.Errorf("setup bucket: %w", err)
	}
	if !ok {
		return fmt.Errorf("device refused pinned bucket %d: reply[12:18]=% x", bucket, resp[12:18])
	}
	if _, err := hidWriteThenRead(hid, 0x36, 0x01, byte(bucket)); err != nil {
		return fmt.Errorf("start transfer: %w", err)
	}
	if _, err := usb.bulkWrite(0x02, header, 5000); err != nil {
		return err
	}
	for i := 0; i < len(data); i += bulkBufferSize {
		end := min(i+bulkBufferSize, len(data))
		if _, err := usb.bulkWrite(0x02, data[i:end], 5000); err != nil {
			return err
		}
	}
	if _, err := hidWriteThenRead(hid, 0x36, 0x02); err != nil {
		return fmt.Errorf("end transfer: %w", err)
	}
	if ok, err := switchBucket(hid, bucket, 0x04); err != nil {
		return fmt.Errorf("show bucket: %w", err)
	} else if !ok {
		return fmt.Errorf("device refused to show bucket %d", bucket)
	}
	return nil
}

/*
pushFramebuffer writes raw RGB565 pixels with no bucket involved at all.

liquidctl uses this only for one product on one firmware major, so it is never
attempted on this cooler -- but the shape of it explains how the vendor's own
software animates the panel. There is no asset to store, no memory to allocate
and nothing to delete: start the transfer, stream a frame, end it.
*/
func pushFramebuffer(hid *os.File, usb *usbDevice, rgb565 []byte, buf byte) error {
	if _, err := hidWriteThenRead(hid, 0x36, 0x01, buf, 0x01, 0x06); err != nil {
		return fmt.Errorf("start transfer: %w", err)
	}
	header := []byte{0x12, 0xFA, 0x01, 0xE8, 0xAB, 0xCD, 0xEF, 0x98, 0x76, 0x54, 0x32, 0x10}
	info := make([]byte, 8)
	info[0] = 0x06 // raw framebuffer
	binary.LittleEndian.PutUint32(info[4:], uint32(len(rgb565)))
	header = append(header, info...)

	if _, err := usb.bulkWrite(0x02, header, 5000); err != nil {
		return err
	}
	sent := 0
	for i := 0; i < len(rgb565); i += bulkBufferSize {
		end := min(i+bulkBufferSize, len(rgb565))
		n, err := usb.bulkWrite(0x02, rgb565[i:end], 5000)
		if err != nil {
			return err
		}
		sent += n
		if n != end-i {
			return fmt.Errorf("short write: asked %d, kernel took %d (total %d of %d)",
				end-i, n, sent, len(rgb565))
		}
	}
	if _, err := hidWriteThenRead(hid, 0x36, 0x02); err != nil {
		return fmt.Errorf("end transfer: %w", err)
	}
	return nil
}

// rgb565Frame draws directly into the panel's pixel format. No encoder, no
// palette: two bytes per pixel, 640x640.
func rgb565Frame(n int) []byte {
	out := make([]byte, lcdSide*lcdSide*2)
	barX := (n * 40) % lcdSide
	i := 0
	for y := range lcdSide {
		for x := range lcdSide {
			var r, g, b uint8
			switch {
			case y < 40:
				r, g, b = 10, 10, 10
			case x >= barX && x < barX+60:
				r, g, b = 250, 250, 250
			default:
				r = uint8(40 + (n*37)%200)
				g = uint8(30 + (n*61)%200)
				b = uint8(60 + (n*23)%180)
			}
			dr, dg, db := r>>3, g>>2, b>>3
			out[i] = (dr << 3) | (dg >> 3)
			out[i+1] = ((dg & 0x7) << 5) | db
			i += 2
		}
	}
	return out
}

/*
pushPingPong owns the memory map instead of negotiating for it.

liquidctl treats the device's 24 KB as a heap: find a free bucket, place the
image after everything else, never reclaim. A dashboard does not need a heap.
It needs two slots -- one on screen, one being written -- so this takes bucket
0 at address 0 and bucket 1 at the halfway mark, and alternates.

The bucket query stays. It looks redundant when the placement is fixed, but a
build that skipped it had every setup refused, so the device appears to want
the sequence.
*/
func pushPingPong(hid *os.File, usb *usbDevice, data []byte) error {
	const slotSize = lcdTotalMemory / 2

	index := 0
	if lastShown == 0 {
		index = 1
	}
	addrValue := index * slotSize

	if _, err := hidWriteThenRead(hid, 0x36, 0x03); err != nil {
		return fmt.Errorf("pre-transfer: %w", err)
	}
	if _, err := queryBuckets(hid); err != nil {
		return err
	}
	if _, err := deleteBucket(hid, index); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	if _, err := deleteBucket(hid, index); err != nil {
		return fmt.Errorf("delete again: %w", err)
	}

	header := []byte{0x12, 0xFA, 0x01, 0xE8, 0xAB, 0xCD, 0xEF, 0x98, 0x76, 0x54, 0x32, 0x10}
	info := make([]byte, 8)
	info[0] = 0x01
	binary.LittleEndian.PutUint32(info[4:], uint32(len(data)))
	header = append(header, info...)

	packets := (len(header) + len(data) + 1023) / 1024
	var addr, size [2]byte
	binary.LittleEndian.PutUint16(addr[:], uint16(addrValue))
	binary.LittleEndian.PutUint16(size[:], uint16(packets))

	ok, resp, err := setupBucket(hid, index, index+1, addr, size)
	if err != nil {
		return fmt.Errorf("setup bucket: %w", err)
	}
	if !ok {
		return fmt.Errorf("device refused bucket %d addr %d packets %d: reply[12:18]=% x",
			index, addrValue, packets, resp[12:18])
	}
	if _, err := hidWriteThenRead(hid, 0x36, 0x01, byte(index)); err != nil {
		return fmt.Errorf("start transfer: %w", err)
	}
	if _, err := usb.bulkWrite(0x02, header, 5000); err != nil {
		return err
	}
	for i := 0; i < len(data); i += bulkBufferSize {
		end := min(i+bulkBufferSize, len(data))
		if _, err := usb.bulkWrite(0x02, data[i:end], 5000); err != nil {
			return err
		}
	}
	if _, err := hidWriteThenRead(hid, 0x36, 0x02); err != nil {
		return fmt.Errorf("end transfer: %w", err)
	}
	if ok, err := switchBucket(hid, index, 0x04); err != nil {
		return fmt.Errorf("show bucket: %w", err)
	} else if !ok {
		return fmt.Errorf("device refused to show bucket %d", index)
	}
	lastShown = index
	return nil
}

// animatedCard is a real multi-frame GIF: a bar sweeping across a field, so a
// still screen and a playing one are impossible to confuse.
func animatedCard(frames int) []byte {
	pal := color.Palette{
		color.RGBA{15, 15, 25, 255},
		color.RGBA{60, 120, 220, 255},
		color.RGBA{250, 240, 120, 255},
	}
	imgs := make([]*image.Paletted, 0, frames)
	delays := make([]int, 0, frames)
	for n := range frames {
		img := image.NewPaletted(image.Rect(0, 0, lcdSide, lcdSide), pal)
		barX := n * lcdSide / frames
		for y := range lcdSide {
			for x := range lcdSide {
				i := uint8(1)
				if x >= barX && x < barX+80 {
					i = 2
				}
				if y < 60 || y > lcdSide-60 {
					i = 0
				}
				img.SetColorIndex(x, y, i)
			}
		}
		imgs = append(imgs, img)
		delays = append(delays, 8) // 80 ms per frame
	}
	var buf bytes.Buffer
	_ = gif.EncodeAll(&buf, &gif.GIF{
		Image:     imgs,
		Delay:     delays,
		LoopCount: 0,
		Config:    image.Config{ColorModel: pal, Width: lcdSide, Height: lcdSide},
	})
	return buf.Bytes()
}
