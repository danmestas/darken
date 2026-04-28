// Package main — `.scion/init-manifest.json` schema + I/O.
//
// runInit writes the manifest after scaffolds are written, recording
// each artifact's path + SHA-256 of the bytes written. runUninstallInit
// reads it to classify each artifact as PRISTINE / CUSTOMIZED without
// re-rendering templated bodies (which can drift across binary versions).
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/danmestas/darken/internal/substrate"
)

// initManifest is the on-disk representation at <target>/.scion/init-manifest.json.
type initManifest struct {
	SchemaVersion int                `json:"schema_version"`
	DarkenVersion string             `json:"darken_version"`
	SubstrateHash string             `json:"substrate_hash"`
	Artifacts     []manifestArtifact `json:"artifacts"`
}

// manifestArtifact records one artifact's identity at init time.
type manifestArtifact struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256"`
}

// writeInitManifest computes each artifact's SHA-256 (using its Body())
// and writes the manifest atomically (temp + rename). The .scion dir is
// created if missing.
func writeInitManifest(target string, arts []artifact) error {
	man := initManifest{
		SchemaVersion: 1,
		DarkenVersion: resolvedVersion(),
		SubstrateHash: substrate.EmbeddedHash(),
	}
	for _, art := range arts {
		body, err := art.Body()
		if err != nil {
			return fmt.Errorf("manifest: %s body: %w", art.RelPath, err)
		}
		h := sha256.Sum256(body)
		man.Artifacts = append(man.Artifacts, manifestArtifact{
			Path:   art.RelPath,
			Kind:   art.Kind,
			SHA256: hex.EncodeToString(h[:]),
		})
	}

	scionDir := filepath.Join(target, ".scion")
	if err := os.MkdirAll(scionDir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(scionDir, "init-manifest.json")
	tmp := dst + ".tmp"

	body, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// readInitManifest reads <target>/.scion/init-manifest.json. Returns
// (nil, nil) if the file is missing — older inits or operator deletion.
// Returns (nil, err) on parse failure.
func readInitManifest(target string) (*initManifest, error) {
	mp := filepath.Join(target, ".scion", "init-manifest.json")
	body, err := os.ReadFile(mp)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var man initManifest
	if err := json.Unmarshal(body, &man); err != nil {
		return nil, fmt.Errorf("parse init-manifest.json: %w", err)
	}
	return &man, nil
}
