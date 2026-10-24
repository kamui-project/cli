// Package clientinfo carries release metadata on existing API requests.
package clientinfo

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
)

const (
	VersionHeader  = "X-Kamui-Client-Version"
	MinimumHeader  = "X-Kamui-CLI-Min-Version"
	LatestHeader   = "X-Kamui-CLI-Latest-Version"
	RequiredHeader = "X-Kamui-CLI-Update-Required"
	UpdateURL      = "https://github.com/kamui-project/cli/releases/latest"
)

// Version is initialized from the existing release ldflag before CLI or daemon startup.
var Version = "dev"
var warned sync.Once
var ErrUpdateRequired = errors.New("this Kamui CLI version is no longer supported; update with brew upgrade kamui or " + UpdateURL)

func SetVersionHeader(req *http.Request) { req.Header.Set(VersionHeader, Version) }

func parseVersion(value string) *semver.Version {
	if len(value) == 0 || len(value) > 128 {
		return nil
	}
	version, err := semver.StrictNewVersion(strings.TrimPrefix(value, "v"))
	if err != nil {
		return nil
	}
	return version
}

// ObserveResponse uses metadata already present in the normal API response.
// It performs no requests, downloads, or local token changes.
func ObserveResponse(resp *http.Response) error {
	if resp.Request != nil && resp.Request.Method == http.MethodPost && resp.Request.URL.Path == "/oauth/revoke" {
		return nil
	}
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get(RequiredHeader) == "true" {
		return ErrUpdateRequired
	}
	current := parseVersion(Version)
	if minimum := resp.Header.Get(MinimumHeader); minimum != "" {
		supported := parseVersion(minimum)
		if supported == nil {
			return errors.New("invalid minimum CLI version in API response")
		}
		if current == nil || current.LessThan(supported) {
			return ErrUpdateRequired
		}
	}
	latest := parseVersion(resp.Header.Get(LatestHeader))
	if current != nil && latest != nil && current.LessThan(latest) {
		warned.Do(func() {
			fmt.Fprintf(os.Stderr, "A newer Kamui CLI (%s) is available. Run brew upgrade kamui or visit %s\n", latest.String(), UpdateURL)
		})
	}
	return nil
}
