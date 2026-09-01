// Package catalog models the feature catalog: which features exist,
// their lifecycle, and their prerequisites.
package catalog

import (
	"fmt"
	"regexp"
)

// Key identifies a feature. Syntax: ^[a-z0-9][a-z0-9._-]{0,127}$.
type Key string

var keyRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

// ValidateKey reports whether k is a well-formed feature key.
func ValidateKey(k Key) error {
	if !keyRE.MatchString(string(k)) {
		return fmt.Errorf("catalog: invalid key %q", k)
	}
	return nil
}

// Lifecycle is a feature's release stage.
type Lifecycle string

const (
	Draft      Lifecycle = "draft"
	Beta       Lifecycle = "beta"
	GA         Lifecycle = "ga"
	Deprecated Lifecycle = "deprecated"
	Retired    Lifecycle = "retired"
)

// Valid reports whether l is one of the defined lifecycles.
func (l Lifecycle) Valid() bool {
	switch l {
	case Draft, Beta, GA, Deprecated, Retired:
		return true
	}
	return false
}

// Feature is one entry of the catalog.
type Feature struct {
	Key         Key       `json:"key"`
	Name        string    `json:"name,omitempty"`
	Description string    `json:"description,omitempty"`
	Lifecycle   Lifecycle `json:"lifecycle"`
	Owner       string    `json:"owner,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Free        bool      `json:"free,omitempty"`
	DependsOn   []Key     `json:"dependsOn,omitempty"`
}
