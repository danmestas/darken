// Package staging materializes role skills into a per-harness staging
// directory the manifest mounts as a read-only volume.
package staging

import (
	"encoding/json"
	"os"
)

type manifest struct {
	Name   string   `json:"name"`
	Skills []string `json:"skills"`
}

// ParseSkillsFromFile reads a manifest JSON file (the output of
// `scion templates show <name> --local --format json`) and returns
// the declared skill refs.
func ParseSkillsFromFile(path string) ([]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	return m.Skills, nil
}
