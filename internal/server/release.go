package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AlexSol/gachi-streak/internal/api"
)

// clientAsset is the release file the desktop client is shipped as.
const clientAsset = "gachi-client.exe"

// ReleaseChecker keeps the latest client release of Repo from GitHub,
// so clients learn about updates from their own server.
type ReleaseChecker struct {
	GH   *GitHubChecker // HTTP access to the GitHub API
	Repo string         // owner/name
	Min  string         // oldest client version still supported ("" = any)

	mu   sync.Mutex
	etag string
	cur  api.ClientVersion
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Digest string `json:"digest"` // "sha256:<hex>"
	} `json:"assets"`
}

// Run refreshes the release every interval until ctx is done.
func (c *ReleaseChecker) Run(ctx context.Context, interval time.Duration) {
	c.refreshLogged(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.refreshLogged(ctx)
		}
	}
}

func (c *ReleaseChecker) refreshLogged(ctx context.Context) {
	if err := c.Refresh(ctx); err != nil {
		c.GH.S.Log.Warn("release check", "repo", c.Repo, "err", err)
	}
}

// Refresh fetches the latest release; a 304 keeps the cached one.
func (c *ReleaseChecker) Refresh(ctx context.Context) error {
	c.mu.Lock()
	etag := c.etag
	c.mu.Unlock()

	resp, err := c.GH.get(ctx, fmt.Sprintf("/repos/%s/releases/latest", c.Repo), "application/vnd.github+json", etag)
	if errors.Is(err, errNotFound) {
		return nil // no releases yet
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return err
	}
	v := api.ClientVersion{Latest: strings.TrimPrefix(rel.TagName, "v"), URL: rel.HTMLURL}
	for _, a := range rel.Assets {
		if a.Name == clientAsset {
			v.Download = a.URL
			v.SHA256 = strings.TrimPrefix(a.Digest, "sha256:")
		}
	}
	c.mu.Lock()
	if v.Latest != c.cur.Latest {
		c.GH.S.Log.Info("client release", "version", v.Latest)
	}
	c.cur, c.etag = v, resp.Header.Get("ETag")
	c.mu.Unlock()
	return nil
}

// Current is the cached release plus the minimum supported version.
func (c *ReleaseChecker) Current() api.ClientVersion {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.cur
	v.Min = c.Min
	return v
}

func (s *Server) handleClientVersion(w http.ResponseWriter, _ *http.Request) {
	var v api.ClientVersion
	if s.Releases != nil {
		v = s.Releases.Current()
	}
	writeJSON(w, http.StatusOK, v)
}
