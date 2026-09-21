package service_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

func TestAMissingLibrarySaysWhyItIsMissing(t *testing.T) {
	/*
		"The image library is unavailable on this machine" is true and
		useless. The first machine to hit it had a service sandboxed out of
		the directory it was trying to create, and the sentence that solves
		that -- "read-only file system" -- was in the startup log and nowhere
		that somebody adding a picture would look.
	*/
	svc := service.New(nil, openrgb.NewFake(), "")
	svc.NoImages(errors.New("mkdir /home/x/.local/share/hotaru: read-only file system"))

	_, err := svc.AddImage("anything", []byte("not a picture"))
	require.ErrorIs(t, err, service.ErrNoImages, "it stopped being the same kind of answer")
	require.Contains(t, err.Error(), "read-only file system", "it did not say why")

	// And a service that was never given one says the plain thing, because
	// there is nothing more to say.
	plain := service.New(nil, openrgb.NewFake(), "")
	_, err = plain.AddImage("anything", []byte("not a picture"))
	require.EqualError(t, err, service.ErrNoImages.Error())
}
