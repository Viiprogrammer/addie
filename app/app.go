package app

import (
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
	"github.com/valyala/fasthttp"
)

var (
	gCli *cli.Context
	gLog *zerolog.Logger
)

var (
	errHlpBadIp    = errors.New("got a problem in parsing X-Forwarded-For request")
	errHlpBadInput = errors.New("there are empty headers in the request")
	errHlpBadUid   = errors.New("got a problem in uid parsing")
)

type App struct{}

func NewApp(c *cli.Context, l *zerolog.Logger) *App {
	gCli, gLog = c, l
	return &App{}
}

func (m *App) Bootstrap() (e error) {
	return fasthttp.ListenAndServe(":8080", m.defaultHandler)
}

func (*App) defaultHandler(ctx *fasthttp.RequestCtx) {
	fmt.Fprintf(ctx, "Hello, world!\n\n")

	fmt.Fprintf(ctx, "Request method is %q\n", ctx.Method())
	fmt.Fprintf(ctx, "RequestURI is %q\n", ctx.RequestURI())
	fmt.Fprintf(ctx, "Requested path is %q\n", ctx.Path())
	fmt.Fprintf(ctx, "Host is %q\n", ctx.Host())
	fmt.Fprintf(ctx, "Query string is %q\n", ctx.QueryArgs())
	fmt.Fprintf(ctx, "User-Agent is %q\n", ctx.UserAgent())
	fmt.Fprintf(ctx, "Connection has been established at %s\n", ctx.ConnTime())
	fmt.Fprintf(ctx, "Request has been started at %s\n", ctx.Time())
	fmt.Fprintf(ctx, "Serial request number for the current connection is %d\n", ctx.ConnRequestNum())
	fmt.Fprintf(ctx, "Your ip is %q\n\n", ctx.RemoteIP())

	fmt.Fprintf(ctx, "Raw request is:\n---CUT---\n%s\n---CUT---", &ctx.Request)

	ctx.Response.Header.Add("X-Location", "google.com")
	ctx.Response.SetStatusCode(fasthttp.StatusOK)
}

// 115     proxy_set_header X-Client-ID $client_id;
// 116     proxy_set_header X-Client-URI $request_uri;
// 117     proxy_set_header X-Cache-Server $http_x_cache_server;

// root@cache-lb1 conf.d #                                                                                                                                                                                                                                                                                                      root@cache-lb1 conf.d # GET /gethtlextra HTTP/1.1                                                                                                                                                                                                                                                                            Host: cache.libria.fun                                                                                                                                                                                                                                                                                                       X-Real-IP: 138.201.93.209                                                                                                                                                                                                                                                                                                    X-Forwarded-Host: cache.libria.fun
// X-Forwarded-Server: cache.libria.fun
// X-Forwarded-For: 138.201.93.209
// X-Forwarded-Proto: https
// X-Client-ID: uid=9FCCEBA77AD66E63BE05A3A502040303
// X-Client-URI: /lalalal/1.m3u8
// X-Cache-Server: 95.216.116.38
// Connection: close

func (*App) hlpRespondError(r *fasthttp.Response, err error, status ...int) {
	status = append(status, fasthttp.StatusInternalServerError)

	r.Header.Set("X-Error", err.Error())
	r.SetStatusCode(status[0])

	gLog.Error().Err(err).Msg("")
}

func (m *App) hlpHandler(ctx *fasthttp.RequestCtx) {

	cip := string(ctx.Request.Header.Peek("X-Forwarded-For"))
	if cip == "" || cip == "127.0.0.1" {
		gLog.Debug().Str("remote_addr", ctx.RemoteIP().String()).Str("x_forwarded_for", cip).Msg("")
		m.hlpRespondError(&ctx.Response, errHlpBadIp)
		return
	}

	uri := string(ctx.RequestURI())
	uid := string(ctx.Request.Header.Peek("X-Client-ID"))
	srv := string(ctx.Request.Header.Peek("X-Cache-Server"))

	if uri == "" || uid == "" || srv == "" {
		gLog.Debug().Strs("headers", []string{uri, uid, srv}).Str("remote_addr", ctx.RemoteIP().String()).
			Str("x_forwarded_for", cip).Msg("")
		m.hlpRespondError(&ctx.Response, errHlpBadInput, fasthttp.StatusBadRequest)
		return
	}

	if uid = m.getUidFromRequest(uid); uid == "" {
		gLog.Debug().Str("uid", uid).Str("remote_addr", ctx.RemoteIP().String()).
			Str("x_forwarded_for", cip).Msg("")
		m.hlpRespondError(&ctx.Response, errHlpBadUid, fasthttp.StatusBadRequest)
		return
	}

	//
	//
	//

	expires, extra := m.getHlpExtra(uri, cip, srv)

	rrl, e := url.Parse(srv + uri)
	if e != nil {
		gLog.Debug().Str("url_parse", srv+uri).Str("remote_addr", ctx.RemoteIP().String()).
			Str("x_forwarded_for", cip).Msg("")
		m.hlpRespondError(&ctx.Response, e)
		return
	}

	var rgs *url.Values
	rgs.Add("expires", expires)
	rgs.Add("extra", extra)
	rrl.RawQuery = rgs.Encode()

	rrl.Scheme = "https"

	gLog.Debug().Str("computed_request", rrl.String()).Str("remote_addr", ctx.RemoteIP().String()).
		Str("x_forwarded_for", cip).Msg("")
	ctx.Response.Header.Set("X-Location", rrl.String())
	ctx.Response.SetStatusCode(fasthttp.StatusOK)
}

func (*App) getHlpExtra(uri, cip, sip string) (expires, extra string) {
	// https://nginx.org/ru/docs/http/ngx_http_secure_link_module.html#secure_link

	expires = time.Now().Add(gCli.Duration("link-expiration")).String()

	// secret link skeleton:
	// expire:uri:client_ip:cache_ip secret
	gLog.Debug().Strs("extra_values", []string{expires, uri, cip, sip, gCli.String("link-secret")}).
		Str("remote_addr", cip).Str("reuqest_uri", uri).Msg("")

	// concat all values
	buf := expires + uri + cip + sip + " " + gCli.String("link-secret")

	// md5 sum
	md5sum := md5.Sum([]byte(buf))
	gLog.Debug().Bytes("computed_md5", md5sum[:]).
		Str("remote_addr", cip).Str("reuqest_uri", uri).Msg("")

	// base64 encoding
	b64buf := base64.StdEncoding.EncodeToString(md5sum[:])
	gLog.Debug().Str("computed_base64", b64buf).Str("remote_addr", cip).Str("reuqest_uri", uri).Msg("")

	// replace && trim string
	extra = strings.Trim(
		strings.ReplaceAll(
			strings.ReplaceAll(
				b64buf, "+", "-",
			),
			"/", "_",
		), "=")

	gLog.Debug().Str("computed_trim", extra).Str("remote_addr", cip).Str("reuqest_uri", uri).Msg("")
	return
}

func (*App) getUidFromRequest(payload string) (uid string) {
	if uid = strings.TrimSpace(payload); uid != "" {
		return
	}

	return strings.TrimLeft(uid, "=")
}
