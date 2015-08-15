package honeybee

import (
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path"
	"strings"
)

// base64 encode a byte slice and remove the padding characters ("=")
func IdEncode(b []byte) string {
	encoded_len := base64.URLEncoding.EncodedLen(len(b))
	buf := make([]byte, encoded_len)
	base64.URLEncoding.Encode(buf, b)

	base_str := string(buf)
	padding_start := strings.Index(base_str, "=")
	if padding_start == -1 {
		padding_start = encoded_len - 1
	}
	return string(buf)[:padding_start]
}

// base64 encode strings and remove the padding characters ("=").
// variadic function
func IdEncodeStrings(parts ...string) string {
	h := sha1.New()
	for i, part := range parts {
		if i != 0 {
			io.WriteString(h, "|")
		}
		io.WriteString(h, part)
	}
	return IdEncode(h.Sum(nil))
}

// Expand the home directory in paths starting with "~/".
// Returns the path unmodified when no tilde was found.
func ExpandHome(p string) string {
	// TODO: handle errors and handle tilde in the middle of paths
	if p[:2] == "~/" {
		usr, err := user.Current()
		if err == nil {
			p = path.Join(usr.HomeDir, p[2:])
		}
	}
	return p
}

// ensure a directory exists, create it if it does not
func EnsureDirectoryExists(d string) (err error) {
	stat, err := os.Stat(d)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(d, 0744)
			if err != nil {
				return
			}
		} else {
			return
		}
	} else {
		if !stat.IsDir() {
			return errors.New(fmt.Sprintf("%v already exists, but is not a directory", d))
		}
	}
	return nil
}
