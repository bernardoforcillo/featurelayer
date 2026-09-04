package featurelayer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrNilSnapshot is returned by Engine.Reload when the loader returns
// neither a snapshot nor an error.
var ErrNilSnapshot = errors.New("featurelayer: loader returned a nil snapshot")

// LoadJSON decodes one JSON Config from r and builds its Snapshot. It is
// the file format: the same shape `json.Marshal(Config{...})` writes.
//
// Decoding is strict — an unknown field anywhere in the document is an
// error, and so is anything after the first JSON value. A typo such as
// "entitlments" would otherwise be silently dropped and ship a plan
// with no entitlements, which is exactly the class of mistake the
// snapshot validation exists to catch. Validation errors come back as
// NewSnapshot returns them: an errors.Join of *ValidationError, one per
// problem, so a CI step can print all of them at once.
func LoadJSON(r io.Reader) (*Snapshot, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("featurelayer: decode config: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			err = errors.New("unexpected data after the config document")
		}
		return nil, fmt.Errorf("featurelayer: decode config: %w", err)
	}
	return NewSnapshot(cfg)
}

// LoadFile is LoadJSON over the file at path.
func LoadFile(path string) (*Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("featurelayer: %w", err)
	}
	defer func() { _ = f.Close() }() // read-only: a Close error carries no information
	return LoadJSON(f)
}

// Reload calls load and, on success, applies the snapshot it returns
// (firing the apply hooks exactly as Apply does). On failure the engine
// keeps serving the snapshot it already has and the error is returned:
// a broken config file never takes a running engine down, it just
// fails to update it.
//
//	if err := engine.Reload(func() (*featurelayer.Snapshot, error) {
//		return featurelayer.LoadFile("features.json")
//	}); err != nil {
//		log.Printf("config not applied: %v", err)
//	}
func (e *Engine) Reload(load func() (*Snapshot, error)) error {
	snap, err := load()
	if err != nil {
		return err
	}
	if snap == nil {
		return ErrNilSnapshot
	}
	e.Apply(snap)
	return nil
}
