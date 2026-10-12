package investigator

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"
)

const scifactURL = "https://scifact.s3-us-west-2.amazonaws.com/release/latest/data.tar.gz"

// FetchSciFact downloads the official archive using Go stdlib, extracting only
// the two required JSONL files; it rejects symlinks and path traversal by
// never trusting original archive paths as filesystem paths.
func FetchSciFact(ctx context.Context, dir string) error {
	if dir == "" {
		return errors.New("target directory required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scifactURL, nil)
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 3 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("dataset download HTTP %d", resp.StatusCode)
	}
	// Keep limits bounded to avoid filling a small VPS with unexpected archive data.
	reader := io.LimitReader(resp.Body, 120<<20)
	gz, err := gzip.NewReader(reader)
	if err != nil {
		return err
	}
	defer gz.Close()
	tarReader := tar.NewReader(gz)
	expected := map[string]bool{"corpus.jsonl": false, "claims_dev.jsonl": false}
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := path.Base(header.Name)
		if _, ok := expected[name]; !ok {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return fmt.Errorf("expected regular dataset file: %s", name)
		}
		if header.Size <= 0 || header.Size > 120<<20 {
			return errors.New("dataset member exceeds limits")
		}
		target := filepath.Join(dir, name)
		f, err := os.CreateTemp(dir, ".download-*")
		if err != nil {
			return err
		}
		written, err := io.CopyN(f, tarReader, header.Size)
		if err == nil && written != header.Size {
			err = io.ErrUnexpectedEOF
		}
		if err == nil {
			err = f.Chmod(0600)
		}
		if err == nil {
			err = f.Sync()
		}
		if e := f.Close(); err == nil {
			err = e
		}
		if err != nil {
			os.Remove(f.Name())
			return err
		}
		if err := os.Rename(f.Name(), target); err != nil {
			os.Remove(f.Name())
			return err
		}
		expected[name] = true
	}
	for n, ok := range expected {
		if !ok {
			return fmt.Errorf("missing %s in SciFact dataset", n)
		}
	}
	return nil
}
