package app

import (
	"crypto/md5"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// getHlpExtra() simply is a secure_link implementation

// docs:
// https://nginx.org/ru/docs/http/ngx_http_secure_link_module.html#secure_link
//
// bash example:
//
//	echo -n '2147483647/s/link127.0.0.1 secret' | \
//		openssl md5 -binary | openssl base64 | tr +/ -_ | tr -d =
func (*App) getHlpExtra(uri, cip, sip, uid string) (expires, extra string) {

	localts := time.Now().Local().Add(gCli.Duration("link-expiration")).Unix()
	expires = strconv.Itoa(int(localts))

	// secret link skeleton:
	// expire:uri:client_ip:cache_ip secret
	gLog.Debug().Strs("extra_values", []string{expires, uri, cip, sip, uid, gCli.String("link-secret")}).
		Str("remote_addr", cip).Str("request_uri", uri).Msg("")

	// concat all values
	// ?? buf := expires + uri + cip + sip + uid + " " + gCli.String("link-secret")
	buf := expires + uri + sip + uid + " " + gCli.String("link-secret")

	// md5 sum
	md5sum := md5.Sum([]byte(buf))
	gLog.Trace().Bytes("computed_md5", md5sum[:]).
		Str("remote_addr", cip).Str("request_uri", uri).Msg("")

	// base64 encoding
	b64buf := base64.StdEncoding.EncodeToString(md5sum[:])
	gLog.Debug().Str("computed_base64", b64buf).Str("remote_addr", cip).Str("request_uri", uri).Msg("")

	// replace && trim string
	extra = strings.Trim(
		strings.ReplaceAll(
			strings.ReplaceAll(
				b64buf, "+", "-",
			),
			"/", "_",
		), "=")

	gLog.Debug().Str("computed_trim", extra).Str("remote_addr", cip).Str("request_uri", uri).Msg("")
	return
}

func (*App) getUidFromRequest(payload string) (uid string) {
	if uid = strings.TrimSpace(payload); uid != "" {
		return
	}

	return strings.TrimLeft(uid, "=")
}
