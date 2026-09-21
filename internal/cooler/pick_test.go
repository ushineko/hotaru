package cooler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTheNodeThatAnswersIsTheCooler(t *testing.T) {
	/*
		One device commonly exposes several hidraw nodes -- on the development
		machine a Logitech receiver has three, a keyboard two, and this cooler
		had two earlier the same day. Sysfs cannot tell them apart, and the
		glob is lexical, so hidraw10 sorts before hidraw7: choosing the first
		match works on the machine it was written on and stops working when
		something else is plugged in.

		So each candidate is asked, and the one that replies is the cooler.
	*/
	silent := NewFake()
	silent.Silent = true
	answering := NewFake()

	dialled := []string{}
	dial := func(path string) (transport, error) {
		dialled = append(dialled, path)
		if path == "/dev/hidraw7" {
			return answering, nil
		}
		return silent, nil
	}

	c, err := pick(context.Background(), []Device{
		{Name: "NZXT Kraken Elite V2", HID: "/dev/hidraw10"},
		{Name: "NZXT Kraken Elite V2", HID: "/dev/hidraw7"},
	}, dial)
	require.NoError(t, err)
	require.Equal(t, "/dev/hidraw7", c.Device().HID)
	require.Equal(t, []string{"/dev/hidraw10", "/dev/hidraw7"}, dialled,
		"the silent node was not tried first, so the test proves nothing")
}

func TestASilentNodeIsClosedRatherThanLeaking(t *testing.T) {
	// Probing opens things. A candidate that turns out to be the wrong node
	// must not be left open for the life of the service.
	silent := NewFake()
	silent.Silent = true
	answering := NewFake()

	_, err := pick(context.Background(), []Device{
		{HID: "a"}, {HID: "b"},
	}, func(path string) (transport, error) {
		if path == "b" {
			return answering, nil
		}
		return silent, nil
	})
	require.NoError(t, err)
	require.True(t, silent.closed, "a node that did not answer was left open")
}

func TestNoNodeAnsweringSaysWhichWereTried(t *testing.T) {
	/*
		"no supported liquid cooler" is wrong when one was found and would not
		talk: the useful sentence names the node, because the next question is
		whether something else is holding it.
	*/
	silent := NewFake()
	silent.Silent = true

	_, err := pick(context.Background(), []Device{
		{Name: "NZXT Kraken Elite V2", HID: "/dev/hidraw7"},
	}, func(string) (transport, error) { return silent, nil })

	require.ErrorContains(t, err, "/dev/hidraw7")
	require.ErrorContains(t, err, "did not answer")
}

func TestANodeThatWillNotOpenIsNotTheEndOfTheSearch(t *testing.T) {
	// Permission on one node says nothing about the next one, and a machine
	// where the first candidate is busy still has a working cooler.
	answering := NewFake()
	c, err := pick(context.Background(), []Device{
		{HID: "busy"}, {HID: "free"},
	}, func(path string) (transport, error) {
		if path == "busy" {
			return nil, context.DeadlineExceeded
		}
		return answering, nil
	})
	require.NoError(t, err)
	require.Equal(t, "free", c.Device().HID)
}
