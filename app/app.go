package app

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/MindHunter86/anilibria-hlp-service/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/favicon"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/pprof"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/skip"
	bolt "github.com/gofiber/storage/bbolt"
	"github.com/gofiber/storage/memory"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
)

var (
	gCli *cli.Context
	gLog *zerolog.Logger

	gCtx   context.Context
	gAbort context.CancelFunc

	gConsul *consulClient

	gAniApi *ApiClient
)

// var (
// 	errHlpBadUid   = errors.New("got a problem in uid parsing")
// 	errHlpBanIp    = errors.New("your ip address has reached download limits; try again later")
// )

var (
	gQualityLock  sync.RWMutex
	gQualityLevel = titleQualityFHD

	gLotteryLock   sync.RWMutex
	gLotteryChance = 0
)

type App struct {
	fb     *fiber.App
	fbstor fiber.Storage

	cache    *CachedTitlesBucket
	banlist  *blocklist
	balancer *balancer

	chunkRegexp *regexp.Regexp
}

type runtimeConfig struct {
	lotteryChance []byte
	qualityLevel  []byte
}

func NewApp(c *cli.Context, l *zerolog.Logger) (app *App) {
	gCli, gLog = c, l
	gLotteryChance = gCli.Int("consul-ab-split")

	app = &App{}
	app.fb = fiber.New(fiber.Config{
		EnableTrustedProxyCheck: len(gCli.String("http-trusted-proxies")) > 0,
		TrustedProxies:          strings.Split(gCli.String("http-trusted-proxies"), ","),
		ProxyHeader:             fiber.HeaderXForwardedFor,

		AppName:               gCli.App.Name,
		ServerHeader:          gCli.App.Name,
		DisableStartupMessage: true,

		StrictRouting:             true,
		DisableDefaultContentType: true,
		DisableDefaultDate:        true,

		Prefork:      gCli.Bool("http-prefork"),
		IdleTimeout:  300 * time.Second,
		ReadTimeout:  1000 * time.Millisecond,
		WriteTimeout: 200 * time.Millisecond,

		GETOnly: true,
		RequestMethods: []string{
			fiber.MethodHead,
			fiber.MethodGet,
		},

		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return ctx.SendStatus(fiber.StatusInternalServerError)
		},
	})

	// storage setup for fiber's limiter
	if gCli.Bool("http-limiter-bbolt") {
		var prefix string
		if prefix := gCli.String("database-prefix"); prefix == "" {
			prefix = "."
		}

		app.fbstor = bolt.New(bolt.Config{
			Database: fmt.Sprintf("%s/%s.db", prefix, gCli.App.Name),
			Bucket:   "application-limiter",
			Reset:    false,
		})
	} else {
		app.fbstor = memory.New(memory.Config{
			GCInterval: 1 * time.Minute,
		})
	}

	// router configuration
	app.fiberConfigure()
	return app
}

const (
	chunkPath = iota + 1
	chunkTitleId
	chunkEpisodeId
	chunkQualityLevel
	chunkName
)

func (m *App) fiberConfigure() {
	// panic recover for all handlers
	m.fb.Use(recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, e interface{}) {
			gLog.Error().Str("stack", c.Request().String()).Msg("PANIC! panic has been caught")
			_, _ = os.Stderr.WriteString(fmt.Sprintf("panic: %v\n%s\n", e, debug.Stack())) //nolint:errcheck // This will never fail
		},
	}))

	// debug
	if gCli.Bool("http-pprof-enable") {
		m.fb.Use(pprof.New())
	}

	// favicon disable
	m.fb.Use(favicon.New(favicon.ConfigDefault))

	// compress support
	m.fb.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed,
	}))

	// CORS serving
	if gCli.Bool("http-cors") {
		m.fb.Use(cors.New(cors.Config{
			AllowOrigins: "*",
		}))
	}

	// Routes
	// controll api
	api := m.fb.Group("/api")
	api.Get("/upstream", m.fbHndApiUpstream)
	api.Get("/reset", m.fbHndApiReset)

	// app
	media := m.fb.Group("/videos/media/ts", skip.New(m.fbHndApiPreCondErr, m.fbMidAppPreCond))

	// app limiter
	media.Use(limiter.New(limiter.Config{
		Next: func(c *fiber.Ctx) bool {
			return c.IP() == "127.0.0.1" || gCli.App.Version == "devel"
		},

		Max:        gCli.Int("limiter-max-req"),
		Expiration: gCli.Duration("limiter-records-duration"),

		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.SendStatus(fiber.StatusTooManyRequests)
		},

		Storage: m.fbstor,
	}))

	// app middlewares
	media.Use(
		m.fbMidAppFakeQuality,
		m.fbMidAppConsulLottery)

	// app sign handler
	media.Use(m.fbHndAppRequestSign)
}

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
	const chunksplit = `^(\/[^\/]+\/[^\/]+\/[^\/]+\/)([^\/]+)\/([^\/]+)\/([^\/]+)\/([^.\/]+)\.ts$`
	m.chunkRegexp = regexp.MustCompile(chunksplit)

	// anilibria API
	gLog.Info().Msg("starting anilibria api client...")
	if gAniApi, e = NewApiClient(gCli, gLog); e != nil {
		return
	}

	// fake quality cooler cache
	gLog.Info().Msg("starting fake quality cache buckets...")
	m.cache = NewCachedTitlesBucket()

	// balancer
	gLog.Info().Msg("starting balancer...")
	m.balancer = newBalancer()

	// consul
	gLog.Info().Msg("starting consul client...")
	if gConsul, e = newConsulClient(m.balancer); e != nil {
		return
	}

	// consul bootstrap
	gLog.Info().Msg("bootstrap consul subsystems...")
	wg.Add(1)
	go func(adone func()) {
		gConsul.bootstrap()
		adone()
	}(wg.Done)

	// ban subsystem
	if !gCli.Bool("ip-ban-disable") {
		gLog.Info().Msg("bootstrap ban subsystem...")
		wg.Add(1)
		go func(adone func()) {
			m.banlist = newBlocklist(!ccx.Bool("ban-ip-disable"))
			m.banlist.run(adone)
		}(wg.Done)
	}

	// http
	wg.Add(1)
	go func(adone func()) {
		defer adone()

		gLog.Debug().Msg("starting fiber http server...")
		defer gLog.Debug().Msg("fiber http server has been stopped")

		if e = m.fb.Listen(gCli.String("http-listen-addr")); e != nil {
			gLog.Error().Err(e).Msg("fiber internal error")
		}
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

	gLog.Debug().Msg("initiate main event loop...")
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

	// http destruct (wtf fiber?)
	// ShutdownWithContext() may be called only after fiber.Listen is running (O_o)
	if e := m.fb.ShutdownWithContext(gCtx); e != nil {
		gLog.Error().Err(e).Msg("fiber Shutdown() error")
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

	gLotteryLock.Lock()
	gLotteryChance = chance
	gLotteryLock.Unlock()

	return
}

func (*App) applyQualityLevel(input []byte) (e error) {
	log.Info().Msg("quality settings change requested")

	gQualityLock.Lock()
	defer gQualityLock.Unlock()

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

func (m *App) getUriWithFakeQuality(tsr *TitleSerieRequest, uri string, quality titleQuality) string {
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

func (*App) getUidFromRequest(payload string) (uid string) {
	if uid = strings.TrimSpace(payload); uid != "" {
		return
	}

	return strings.TrimLeft(uid, "=")
}

// func (m *App) hlpHandler(ctx *fasthttp.RequestCtx) {
// 	// client IP parsing
// 	cip := string(ctx.Request.Header.Peek(fasthttp.HeaderXForwardedFor))
// 	if cip == "" || cip == "127.0.0.1" {
// 		gLog.Debug().Str("remote_addr", ctx.RemoteIP().String()).Str("x_forwarded_for", cip).Msg("")
// 		m.hlpRespondError(&ctx.Response, errHlpBadIp)
// 		return
// 	}

// 	// blocklist
// 	if !bytes.Equal(ctx.Request.Header.Peek("X-ReqLimit-Status"), []byte("PASSED")) && len(ctx.Request.Header.Peek("X-ReqLimit-Status")) != 0 {
// 		log.Info().Str("reqlimit_status", string(ctx.Request.Header.Peek("X-ReqLimit-Status"))).Str("remote_addr", cip).
// 			Msg("bad x-reqlimit-status detected, given ip addr will be banned immediately")

// 		if !m.banlist.push(ctx.Request.Header.Peek(fasthttp.HeaderXForwardedFor)) {
// 			log.Warn().Str("remote_addr", cip).Msg("there is an unknown error in blocklist.push method")
// 		}

// 		m.hlpRespondError(&ctx.Response, errHlpBanIp, fasthttp.StatusForbidden)
// 		return
// 	}

// 	if m.banlist.isExists(ctx.Request.Header.Peek(fasthttp.HeaderXForwardedFor)) {
// 		log.Debug().Str("remote_addr", cip).Msg("given remote addr has been banned")

// 		m.hlpRespondError(&ctx.Response, errHlpBanIp, fasthttp.StatusForbidden)
// 		return
// 	}
// }
