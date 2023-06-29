package app

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/favicon"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/pprof"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/skip"
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
			AllowHeaders: strings.Join([]string{
				fiber.HeaderContentType,
			}, ","),
			AllowMethods: strings.Join([]string{
				fiber.MethodPost,
			}, ","),
		}))
	}

	// Routes

	// group api - /api
	api := m.fb.Group("/api")
	api.Get("/upstream", m.fbHndApiUpstream)
	api.Get("/reset", m.fbHndApiReset)
	api.Post("logger/level", m.fbHndApiLoggerLevel)

	// group blocklist - /api/blocklist
	blist := api.Group("/blocklist")
	blist.Post("/add", m.fbHndApiBlockIp)
	blist.Post("/remove", m.fbHndApiUnblockIp)
	blist.Post("/switch", m.fbHndApiBListSwitch)
	blist.Post("/reset", m.fbHndApiBlockReset)

	// group media - /videos/media/ts
	media := m.fb.Group("/videos/media/ts", skip.New(m.fbHndApiPreCondErr, m.fbMidAppPreCond))

	// group media - blocklist
	media.Use(m.fbMidAppBlocklist)

	// group media - limiter
	media.Use(limiter.New(limiter.Config{
		Next: func(c *fiber.Ctx) bool {
			// add emergency stop for limiter
			return c.IP() == "127.0.0.1" || gCli.App.Version == "devel"
		},

		Max:        gCli.Int("limiter-max-req"),
		Expiration: gCli.Duration("limiter-records-duration"),

		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},

		Storage: m.fbstor,
	}))

	// group media - middlewares
	media.Use(
		m.fbMidAppFakeQuality,
		m.fbMidAppConsulLottery)

	// group media - sign handler
	media.Use(m.fbHndAppRequestSign)
}
