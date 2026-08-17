package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Fetcher struct {
	client      *http.Client
	XferCommand string
}

func New() *Fetcher {
	return &Fetcher{
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func ExpandRepoURL(server, repo, arch, filename string) string {
	s := strings.ReplaceAll(server, "$repo", repo)
	s = strings.ReplaceAll(s, "$arch", arch)
	s = strings.TrimRight(s, "/")
	if filename == "" {
		return s
	}
	return s + "/" + filename
}

func (f *Fetcher) Fetch(ctx context.Context, rawURL, dest string) error {
	if f.XferCommand != "" {
		return f.fetchViaXfer(ctx, rawURL, dest)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse url %q: %w", rawURL, err)
	}

	switch u.Scheme {
		case "file":
			src := u.Path
			if u.Host != "" && u.Host != "localhost" {
				src = filepath.Join("/", u.Host, u.Path)
			}
			in, err := os.Open(src)
			if err != nil {
				return fmt.Errorf("open %s: %w", src, err)
			}
			defer in.Close()
			return atomicWrite(dest, in)

		case "http", "https":
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
			if err != nil {
				return err
			}
			resp, err := f.client.Do(req)
			if err != nil {
				return fmt.Errorf("GET %s: %w", rawURL, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("GET %s: %s", rawURL, resp.Status)
			}
			return atomicWrite(dest, resp.Body)

		default:
			return fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
}

func (f *Fetcher) fetchViaXfer(ctx context.Context, rawURL, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	cmdLine := strings.ReplaceAll(f.XferCommand, "%u", rawURL)
	cmdLine = strings.ReplaceAll(cmdLine, "%o", dest)

	cmd := exec.CommandContext(ctx, "sh", "-c", cmdLine)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("xfer command failed: %w", err)
	}

	if _, err := os.Stat(dest); err != nil {
		return fmt.Errorf("xfer command did not create %s: %w", dest, err)
	}

	return nil
}

func atomicWrite(dest string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".forge-dl-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, dest)
}
