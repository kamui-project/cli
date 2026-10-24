//go:build !windows

package localtls

import (
	"fmt"
	"os"
)

func privatePermissions(path string, info os.FileInfo) error {
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("local TLS path %s must be accessible only to its owner (directory chmod 700, file chmod 600)", path)
	}
	return nil
}
