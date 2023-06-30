package app

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/MindHunter86/anilibria-hlp-service/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/favicon"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/pprof"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/gofiber/fiber/v2/middleware/skip"
)

func (m *App) fiberConfigure() {
	// time collector
	m.fb.Use(func(c *fiber.Ctx) (e error) {

		c.SetUserContext(context.WithValue(
			c.UserContext(),
			utils.FbCtxRequestTimer,
			make(map[utils.ContextKey]time.Time),
		))

		start, e := time.Now(), c.Next()
		stop := time.Now()

		total := stop.Sub(start)
		setup := stop.Sub(m.getRequestTimerSegment(c, utils.FbCtxReqBeforeRoute)).Round(time.Microsecond)
		routing := stop.Sub(m.getRequestTimerSegment(c, utils.FbCtxReqPreCond)).Round(time.Microsecond)
		precond := stop.Sub(m.getRequestTimerSegment(c, utils.FbCtxReqBlocklist)).Round(time.Microsecond)
		blist := stop.Sub(m.getRequestTimerSegment(c, utils.FbCtxReqFakeQuality)).Round(time.Microsecond)
		fquality := stop.Sub(m.getRequestTimerSegment(c, utils.FbCtxReqConsulLottery)).Round(time.Microsecond)
		clottery := stop.Sub(m.getRequestTimerSegment(c, utils.FbCtxReqReqSign)).Round(time.Microsecond)
		reqsign := stop.Sub(stop).Round(time.Microsecond)

		reqsign = clottery - reqsign
		clottery = fquality - clottery
		fquality = blist - fquality
		blist = precond - blist
		precond = routing - precond
		routing = setup - routing
		setup = total - setup

		gLog.Debug().Msgf(
			"Setup %s; Routing %s; PreCond %s; Blocklist %s; FQuality %s; CLottery %s; ReqSign %s;",
			setup, routing, precond, blist, fquality, clottery, reqsign)
		gLog.Debug().Msgf("Total %s", stop.Sub(start).Round(time.Microsecond))
		gLog.Debug().Msgf("Time Collector %s", time.Now().Sub(stop).Round(time.Microsecond))

		return
	})

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

	// request id
	m.fb.Use(requestid.New())

	// http logger
	m.fb.Use(logger.New(logger.Config{
		TimeFormat: time.RFC3339,
		Format:     "[${time}] ${requestid} ${status} - ${latency} ${method} ${path}\n",
		Output:     gLog,
	}))

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

	// time collector - Before routing
	m.fb.Use(func(c *fiber.Ctx) error {
		m.lapRequestTimer(c, utils.FbCtxReqBeforeRoute)
		return c.Next()
	})

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
			// !!!
			// add emergency stop for limiter
			// if gLimiterStop == true --> return true
			// !!!
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

func (*App) lapRequestTimer(c *fiber.Ctx, k utils.ContextKey) {
	c.UserContext().
		Value(utils.FbCtxRequestTimer).(map[utils.ContextKey]time.Time)[k] = time.Now()
}

func (*App) getRequestTimerSegment(c *fiber.Ctx, k utils.ContextKey) time.Time {
	return c.UserContext().
		Value(utils.FbCtxRequestTimer).(map[utils.ContextKey]time.Time)[k]
}
