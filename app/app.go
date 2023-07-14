package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/MindHunter86/anilibria-hlp-service/balancer"
	"github.com/MindHunter86/anilibria-hlp-service/blocklist"
	"github.com/MindHunter86/anilibria-hlp-service/runtime"
	"github.com/MindHunter86/anilibria-hlp-service/utils"
	"github.com/gofiber/fiber/v2"
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

var (
	gQualityLock  sync.RWMutex
	gQualityLevel = utils.TitleQualityFHD

	gLotteryLock   sync.RWMutex
	gLotteryChance = 0

	gBListLock        sync.RWMutex
	gBlocklistEnabled = 0

	gLimiterLock    sync.RWMutex
	gLimiterEnabled = 1
)

type App struct {
	fb     *fiber.App
	fbstor fiber.Storage

	cache     *CachedTitlesBucket
	blocklist *blocklist.Blocklist
	runtime   *runtime.Runtime

	cloudBalancer balancer.Balancer
	bareBalancer  balancer.Balancer

	chunkRegexp *regexp.Regexp
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

		RequestMethods: []string{
			fiber.MethodHead,
			fiber.MethodGet,
			fiber.MethodOptions,
			fiber.MethodPost,
		},

		// ErrorHandler: func(ctx *fiber.Ctx, err error) error {
		// 	return ctx.SendStatus(fiber.StatusInternalServerError)
		// },
	})

	// storage setup for fiber's limiter
	if gCli.Bool("limiter-use-bbolt") {
		var prefix string
		if prefix = gCli.String("database-prefix"); prefix == "" {
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

func (m *App) Bootstrap() (e error) {
	var wg sync.WaitGroup
	var echan = make(chan error, 32)
	var rpatcher = make(chan *runtime.RuntimePatch, 1)

	// goroutine helper
	gofunc := func(waitgroup *sync.WaitGroup, payload func()) {
		waitgroup.Add(1)
		defer waitgroup.Done()

		payload()
	}

	gCtx, gAbort = context.WithCancel(context.Background())
	gCtx = context.WithValue(gCtx, utils.ContextKeyLogger, gLog)
	gCtx = context.WithValue(gCtx, utils.ContextKeyCliContext, gCli)
	gCtx = context.WithValue(gCtx, utils.ContextKeyAbortFunc, gAbort)
	gCtx = context.WithValue(gCtx, utils.ContextKeyRPatcher, rpatcher)

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
	if gAniApi, e = NewApiClient(); e != nil {
		return
	}

	// fake quality cooler cache
	gLog.Info().Msg("starting fake quality cache buckets...")
	m.cache = NewCachedTitlesBucket()

	// blocklist
	m.blocklist = blocklist.NewBlocklist()

	// runtime
	m.runtime = runtime.NewRuntime(m.blocklist)

	// balancer
	gLog.Info().Msg("starting balancer...")

	// balancer V2
	gLog.Info().Msg("bootstrap balancer_v2 subsystems...")
	gofunc(&wg, func() {
		balancer.Init(gCtx)
	})

	m.bareBalancer = balancer.NewClusterBalancer(gCtx)
	m.cloudBalancer = balancer.NewClusterBalancer(gCtx)

	// consul
	gLog.Info().Msg("starting consul client...")
	if gConsul, e = newConsulClient(m.cloudBalancer); e != nil {
		return
	}

	// consul bootstrap
	gLog.Info().Msg("bootstrap consul subsystems...")
	gofunc(&wg, gConsul.bootstrap)

	// http
	gofunc(&wg, func() {
		gLog.Debug().Msg("starting fiber http server...")
		defer gLog.Debug().Msg("fiber http server has been stopped")

		if e = m.fb.Listen(gCli.String("http-listen-addr")); e != nil {
			gLog.Error().Err(e).Msg("fiber internal error")
		}
	})

	// another subsystems
	// ...

	// main event loop
	wg.Add(1)
	go m.loop(echan, wg.Done)

	gLog.Info().Msg("ready...")
	wg.Wait()
	return
}

func (m *App) loop(_ chan error, done func()) {
	defer done()

	kernSignal := make(chan os.Signal, 1)
	signal.Notify(kernSignal, syscall.SIGINT, syscall.SIGTERM, syscall.SIGTERM, syscall.SIGQUIT)

	gLog.Debug().Msg("initiate main event loop...")
	defer gLog.Debug().Msg("main event loop has been closed")

	rpatcher := gCtx.Value(utils.ContextKeyRPatcher).(chan *runtime.RuntimePatch)

LOOP:
	for {
		select {
		case patch := <-rpatcher:
			gLog.Debug().Msg("new configuration detected, applying...")
			m.runtime.ApplyPath(patch)
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
