package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/ushineko/hotaru/internal/images"
)

/*
The image library, as the service offers it.

Here rather than in the window because the conversion is the interesting part
and both shells want it: a CLI that could not add an image would be a parity
rule broken the first time it was tested, and a directory two shells both write
into is the one-writer question with a worse answer.
*/

// ImageLibrary is where converted pictures live.
type ImageLibrary interface {
	Add(name string, source []byte) (images.Image, error)
	AddSlideshow(name string, sources [][]byte) (images.Image, error)
	All() ([]images.Image, error)
	Remove(name string) error
	Read(name string) ([]byte, error)
}

// SetImages gives the service somewhere to keep pictures.
func (s *Service) SetImages(library ImageLibrary) {
	s.mu.Lock()
	s.images, s.imagesWhy = library, nil
	s.mu.Unlock()
}

/*
NoImages says the library could not be opened, and why.

The reason is kept rather than logged and dropped. "The image library is
unavailable on this machine" is true and useless: the first machine to hit it
had a service sandboxed out of the directory it was trying to create, and the
sentence that solves that is "read-only file system" -- which was in the
service's log at startup and nowhere a person adding a picture would look.
*/
func (s *Service) NoImages(why error) {
	s.mu.Lock()
	s.images, s.imagesWhy = nil, why
	s.mu.Unlock()
}

// ErrNoImages is a machine whose image directory could not be opened.
var ErrNoImages = errors.New("the image library is unavailable on this machine")

func (s *Service) library() (ImageLibrary, error) {
	s.mu.RLock()
	library, why := s.images, s.imagesWhy
	s.mu.RUnlock()

	switch {
	case library != nil:
		return library, nil
	case why != nil:
		return nil, fmt.Errorf("%w: %w", ErrNoImages, why)
	}
	return nil, ErrNoImages
}

// AddImage converts a picture and stores it under a name.
func (s *Service) AddImage(name string, source []byte) (images.Image, error) {
	library, err := s.library()
	if err != nil {
		return images.Image{}, err
	}
	return library.Add(name, source)
}

/*
PreviewImage converts a picture and does not keep it.

What "preview" has to mean here: the useful thing to look at is the conversion
-- cropped square, scaled to the panel, reduced to 256 colours chosen from the
picture -- and looking at it before deciding is the difference between a
library of wallpapers and a library of wallpapers that did not survive being
cropped.

Storing it and offering to remove it afterwards would be the same picture and a
worse promise.
*/
func (s *Service) PreviewImage(source []byte) ([]byte, int, error) {
	if _, err := s.library(); err != nil {
		return nil, 0, err
	}
	return images.Convert(source)
}

/*
AddSlideshow turns several pictures into one animation and stores it.

The other answer to a stack of files: eight wallpapers, or one reel that holds
each and crossfades between them.
*/
func (s *Service) AddSlideshow(name string, sources [][]byte) (images.Image, error) {
	library, err := s.library()
	if err != nil {
		return images.Image{}, err
	}
	return library.AddSlideshow(name, sources)
}

// Images is what is stored.
func (s *Service) Images() ([]images.Image, error) {
	library, err := s.library()
	if err != nil {
		return nil, err
	}
	return library.All()
}

// RemoveImage forgets one.
func (s *Service) RemoveImage(name string) error {
	library, err := s.library()
	if err != nil {
		return err
	}
	return library.Remove(name)
}

/*
ShowImage puts a stored picture on the panel.

Through the same door as every other thing that draws on it, so the dashboard
stands down and a scene applied afterwards takes the screen back.
*/
func (s *Service) ShowImage(ctx context.Context, name string) error {
	library, err := s.library()
	if err != nil {
		return err
	}
	body, err := library.Read(name)
	if err != nil {
		return fmt.Errorf("show %s: %w", name, err)
	}
	return s.Draw(ctx, Screen{Image: body})
}
