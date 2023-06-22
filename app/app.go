package app

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/MindHunter86/anilibria-hlp-service/utils"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
	"github.com/valyala/fasthttp"
)

var (
	gCli *cli.Context
	gLog *zerolog.Logger

	gCtx   context.Context
	gAbort context.CancelFunc

	gConsul *consulClient

	gAniApi *ApiClient
)

var (
	errHlpBadIp    = errors.New("got a problem in parsing X-Forwarded-For request")
	errHlpBadInput = errors.New("there are empty headers in the request")
	errHlpBadUid   = errors.New("got a problem in uid parsing")
	errHlpBanIp    = errors.New("your ip address has reached download limits; try again later")
)

var (
	gQualityLevel  = titleQualityFHD
	gLotteryChance = 0
)

type App struct {
	cache    *CachedTitlesBucket
	banlist  *blocklist
	balancer *balancer

	chunkRegexp *regexp.Regexp
}

type runtimeConfig struct {
	lotteryChance []byte
	qualityLevel  []byte
}

func NewApp(c *cli.Context, l *zerolog.Logger) *App {
	gCli, gLog = c, l
	gLotteryChance = gCli.Int("consul-ab-split")
	return &App{}
}

const (
	chunkPath = iota + 1
	chunkTitleId
	chunkEpisodeId
	chunkQualityLevel
	chunkName
)

func (m *App) Bootstrap() (e error) {
	var wg sync.WaitGroup
	var echan = make(chan error, 32)
	var cfgchan = make(chan *runtimeConfig, 1)

	gCtx, gAbort = context.WithCancel(context.Background())
	gCtx = context.WithValue(gCtx, utils.ContextKeyLogger, gLog)
	gCtx = context.WithValue(gCtx, utils.ContextKeyCliContext, gCli)
	gCtx = context.WithValue(gCtx, utils.ContextKeyAbortFunc, gAbort)
	gCtx = context.WithValue(gCtx, utils.ContextKeyCfgChan, cfgchan)

	// defer m.checkErrorsBeforeClosing(echan)
	// defer wg.Wait() // !!
	defer gLog.Debug().Msg("waiting for opened goroutines")
	defer gAbort()

	// BOOTSTRAP SECTION:
	// common
	const chunksplit = `^(\/[^\/]+\/[^\/]+\/[^\/]+\/)([^\/]+)\/([^\/]+)\/([^\/]+)\/([^.\/]+)\.ts`
	m.chunkRegexp = regexp.MustCompile(chunksplit)

	// anilibria API
	if gAniApi, e = NewApiClient(gCli, gLog); e != nil {
		return
	}

	// ban subsystem
	wg.Add(1)
	go func(adone func()) {
		m.banlist = newBlocklist(!ccx.Bool("ban-ip-disable"))
		m.banlist.run(adone)
	}(wg.Done)

	// cache
	m.cache = NewCachedTitlesBucket()

	// !!!
	// TODO
	// fix this shit
	// main http server
	go fasthttp.ListenAndServe(":8089", m.hlpHandler)

	// balancer
	m.balancer = newBalancer()

	// consul
	if gConsul, e = newConsulClient(m.balancer); e != nil {
		return
	}

	wg.Add(1)
	go func(adone func()) {
		gConsul.bootstrap()
		adone()
	}(wg.Done)

	// another subsystems
	// ...

	// main event loop
	wg.Add(1)
	go m.loop(echan, wg.Done)

	wg.Wait()
	return
}

func (m *App) loop(_ chan error, done func()) {
	defer done()

	kernSignal := make(chan os.Signal, 1)
	signal.Notify(kernSignal, syscall.SIGINT, syscall.SIGTERM, syscall.SIGTERM, syscall.SIGQUIT)

	gLog.Debug().Msg("initiate main event loop")
	defer gLog.Debug().Msg("main event loop has been closed")

	cfgchan := gCtx.Value(utils.ContextKeyCfgChan).(chan *runtimeConfig)

LOOP:
	for {
		select {
		case cfg := <-cfgchan:
			gLog.Info().Msg("new configuration detected, applying...")
			m.applyRuntimeConfig(cfg)
		case <-kernSignal:
			gLog.Info().Msg("kernel signal has been caught; initiate application closing...")
			gAbort()
			break LOOP
		// case err := <-errs:
		// 	gLog.Error().Err(err).Msg("there are internal errors from one of application submodule")
		// 	gLog.Info().Msg("calling abort()...")
		// 	gAbort()
		case <-gCtx.Done():
			gLog.Info().Msg("internal abort() has been caught; initiate application closing...")
			break LOOP
		}
	}
}

func (m *App) applyRuntimeConfig(cfg *runtimeConfig) (e error) {
	if len(cfg.lotteryChance) != 0 {
		if e = m.applyLotteryChance(cfg.lotteryChance); e != nil {
			gLog.Error().Err(e).Msg("could not apply runtime configuration (lottery chance)")
		}
	}

	if len(cfg.qualityLevel) != 0 {
		if e = m.applyQualityLevel(cfg.qualityLevel); e != nil {
			gLog.Error().Err(e).Msg("could not apply runtime configuration (quality level)")
		}
	}

	return
}

func (*App) applyLotteryChance(input []byte) (e error) {
	var chance int
	if chance, e = strconv.Atoi(string(input)); e != nil {
		return
	}

	if chance < 0 || chance > 100 {
		gLog.Warn().Int("chance", chance).Msg("chance could not be less than 0 and more than 100")
		return
	}

	gLog.Info().Msgf("runtime config - applied lottery chance %s", string(input))
	gLotteryChance = chance
	return
}

func (*App) applyQualityLevel(input []byte) (e error) {
	log.Info().Msg("quality settings change requested")

	switch string(input) {
	case "480":
		gQualityLevel = titleQualitySD
	case "720":
		gQualityLevel = titleQualityHD
	case "1080":
		gQualityLevel = titleQualityFHD
	default:
		gLog.Warn().Str("input", string(input)).Msg("qulity level can be 480 720 or 1080 only")
		return
	}

	gLog.Info().Msgf("runtime config - applied quality %s", string(input))
	return
}

// func (*App) checkErrorsBeforeClosing(errs chan error) {
// 	gLog.Debug().Msg("pre-exit error chan checking for errors...")
// 	if len(errs) == 0 {
// 		gLog.Debug().Msg("error chan is empty, cool")
// 		return
// 	}

// 	close(errs)
// 	for err := range errs {
// 		gLog.Warn().Err(err).Msg("an error has been detected while application trying close the submodules")
// 	}
// }

// func (*App) defaultHandler(ctx *fasthttp.RequestCtx) {
// 	fmt.Fprintf(ctx, "Hello, world!\n\n")

// 	fmt.Fprintf(ctx, "Request method is %q\n", ctx.Method())
// 	fmt.Fprintf(ctx, "RequestURI is %q\n", ctx.RequestURI())
// 	fmt.Fprintf(ctx, "Requested path is %q\n", ctx.Path())
// 	fmt.Fprintf(ctx, "Host is %q\n", ctx.Host())
// 	fmt.Fprintf(ctx, "Query string is %q\n", ctx.QueryArgs())
// 	fmt.Fprintf(ctx, "User-Agent is %q\n", ctx.UserAgent())
// 	fmt.Fprintf(ctx, "Connection has been established at %s\n", ctx.ConnTime())
// 	fmt.Fprintf(ctx, "Request has been started at %s\n", ctx.Time())
// 	fmt.Fprintf(ctx, "Serial request number for the current connection is %d\n", ctx.ConnRequestNum())
// 	fmt.Fprintf(ctx, "Your ip is %q\n\n", ctx.RemoteIP())

// 	fmt.Fprintf(ctx, "Raw request is:\n---CUT---\n%s\n---CUT---", &ctx.Request)

// 	ctx.Response.Header.Add("X-Location", "google.com")
// 	ctx.Response.SetStatusCode(fasthttp.StatusOK)
// }

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

	// debug methods
	if string(ctx.Request.RequestURI()) == "/debug/upstream" {
		fmt.Fprint(ctx, m.balancer.getUpstreamStats())
		ctx.SetContentType("text/plain; charset=utf8")
		ctx.Response.SetStatusCode(fasthttp.StatusOK)
		return
	}

	// client IP parsing
	cip := string(ctx.Request.Header.Peek(fasthttp.HeaderXForwardedFor))
	if cip == "" || cip == "127.0.0.1" {
		gLog.Debug().Str("remote_addr", ctx.RemoteIP().String()).Str("x_forwarded_for", cip).Msg("")
		m.hlpRespondError(&ctx.Response, errHlpBadIp)
		return
	}

	// blocklist
	if !bytes.Equal(ctx.Request.Header.Peek("X-ReqLimit-Status"), []byte("PASSED")) && len(ctx.Request.Header.Peek("X-ReqLimit-Status")) != 0 {
		log.Info().Str("reqlimit_status", string(ctx.Request.Header.Peek("X-ReqLimit-Status"))).Str("remote_addr", cip).
			Msg("bad x-reqlimit-status detected, given ip addr will be banned immediately")

		if !m.banlist.push(ctx.Request.Header.Peek(fasthttp.HeaderXForwardedFor)) {
			log.Warn().Str("remote_addr", cip).Msg("there is an unknown error in blocklist.push method")
		}

		m.hlpRespondError(&ctx.Response, errHlpBanIp, fasthttp.StatusForbidden)
		return
	}

	if m.banlist.isExists(ctx.Request.Header.Peek(fasthttp.HeaderXForwardedFor)) {
		log.Debug().Str("remote_addr", cip).Msg("given remote addr has been banned")

		m.hlpRespondError(&ctx.Response, errHlpBanIp, fasthttp.StatusForbidden)
		return
	}

	// another values parsing
	uri := string(ctx.Request.Header.Peek("X-Client-URI"))
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

	// quality cooler:
	uri = m.getUriWithFakeQuality(uri, gQualityLevel)

	// consul managing
	if gCli.Bool("consul-managed") {
		// lottery
		if gLotteryChance >= rand.Intn(99)+1 {
			ip, s := m.balancer.getServerByChunkName(
				string(
					m.chunkRegexp.FindSubmatch(
						ctx.Request.Header.Peek("X-Client-URI"),
					)[chunkName],
				),
			)

			if ip != "" {
				srv = strings.ReplaceAll(s.name, "-node", "") + "." + gCli.String("consul-entries-domain")
				gLog.Trace().Msgf("test new consul balancing %s %s", ip, srv)
			} else {
				gLog.Debug().Msg("consul has no servers for balancing, fallback to old method")
			}
		} else {
			gLog.Debug().Msg("consul lottery looser, fallback to old method")
		}
	}

	// request signer:
	expires, extra := m.getHlpExtra(uri, cip, srv, uid)

	// furl := fasthttp.AcquireURI()
	// furl.Parse(nil, ctx.Request.Header.Peek("X-Client-URI"))
	// furl.Q

	rrl, e := url.Parse(srv + uri)
	if e != nil {
		gLog.Debug().Str("url_parse", srv+uri).Str("remote_addr", ctx.RemoteIP().String()).
			Str("x_forwarded_for", cip).Msg("")
		m.hlpRespondError(&ctx.Response, e)
		return
	}

	var rgs = &url.Values{}
	rgs.Add("expires", expires)
	rgs.Add("extra", extra)
	rrl.RawQuery = rgs.Encode()

	rrl.Scheme = "https"

	gLog.Debug().Str("computed_request", rrl.String()).Str("remote_addr", ctx.RemoteIP().String()).
		Str("x_forwarded_for", cip).Msg("")
	ctx.Response.Header.Set("X-Location", rrl.String())
	ctx.Response.SetStatusCode(fasthttp.StatusOK)
}

func (m *App) getUriWithFakeQuality(uri string, quality titleQuality) string {
	log.Debug().Msg("called fake quality")
	tsr := NewTitleSerieRequest(uri)

	if !tsr.isValid() {
		return uri
	}

	log.Debug().Uint16("tsr", uint16(tsr.getTitleQuality())).Uint16("coded", uint16(quality)).Msg("quality check")
	if tsr.getTitleQuality() <= quality {
		return uri
	}

	log.Debug().Msg("format check")
	if tsr.isOldFormat() && !tsr.isM3U8() {
		log.Info().Str("old", "/"+tsr.getTitleQualityString()+"/").Str("new", "/"+quality.string()+"/").Str("uri", uri).Msg("format is old")
		return strings.ReplaceAll(uri, "/"+tsr.getTitleQualityString()+"/", "/"+quality.string()+"/")
	}

	log.Debug().Msg("trying to complete tsr")
	title, e := m.doTitleSerieRequest(tsr)
	if e != nil {
		log.Error().Err(e).Msg("could not rewrite quality for the request")
		return uri
	}

	log.Debug().Msg("trying to get hash")
	hash, ok := tsr.getTitleHash()
	if !ok {
		return uri
	}

	log.Debug().Str("old_hash", hash).Str("new_hash", title.QualityHashes[quality]).Str("uri", uri).Msg("")
	return strings.ReplaceAll(
		strings.ReplaceAll(uri, "/"+tsr.getTitleQualityString()+"/", "/"+quality.string()+"/"),
		hash, title.QualityHashes[quality],
	)
}

// getHlpExtra() simply is a secure_link implementation
//
// docs:
// https://nginx.org/ru/docs/http/ngx_http_secure_link_module.html#secure_link
//
// unix example:
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
	gLog.Debug().Bytes("computed_md5", md5sum[:]).
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
