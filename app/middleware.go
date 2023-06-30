package app

import (
	"errors"
	"math/rand"
	"strings"

	"github.com/MindHunter86/anilibria-hlp-service/utils"
	"github.com/gofiber/fiber/v2"
)

var (
	// errApiPreBadHeaders = errors.New("could not parse required headers")
	errApiPreBadUri    = errors.New("invalid uri")
	errApiPreBadId     = errors.New("invalid id")
	errApiPreBadServer = errors.New("invalid server")
	errApiPreUriRegexp = errors.New("regexp matching failure")
)

const (
	apiHeaderUri      = "X-Client-Uri"
	apiHeaderId       = "X-Client-Id"
	apiHeaderServer   = "X-Cache-Server"
	apiHeaderLocation = "X-Location"
)

type appMidError uint8

const (
	errMidAppPreHeaderUri appMidError = 1 << iota
	errMidAppPreHeaderId
	errMidAppPreHeaderServer
	errMidAppPreUriRegexp
)

// func (m *App) hlpHandler(ctx *fasthttp.RequestCtx) {
// 	// client IP parsing
// 	cip := string(ctx.Request.Header.Peek(fasthttp.HeaderXForwardedFor))
// 	if cip == "" || cip == "127.0.0.1" {
// 		gLog.Debug().Str("remote_addr", ctx.RemoteIP().String()).Str("x_forwarded_for", cip).Msg("")
// 		m.hlpRespondError(&ctx.Response, errHlpBadIp)
// 		return
// 	}
// }

// API precondition check
func (m *App) fbMidAppPreCond(ctx *fiber.Ctx) (skip bool) {
	m.lapRequestTimer(ctx, utils.FbCtxReqPreCond)
	var errs appMidError

	gLog.Trace().Interface("hdrs", ctx.GetReqHeaders()).Msg("debug")
	switch h := ctx.GetReqHeaders(); {
	case strings.TrimSpace(h[apiHeaderUri]) == "":
		errs = errs | errMidAppPreHeaderUri
		ctx.Locals("errors", errs)
		return
	case strings.TrimSpace(h[apiHeaderId]) == "":
		errs = errs | errMidAppPreHeaderId
		ctx.Locals("errors", errs)
		return
	case strings.TrimSpace(h[apiHeaderServer]) == "":
		errs = errs | errMidAppPreHeaderServer
		ctx.Locals("errors", errs)
		return
	}

	ctx.Locals("uid", strings.TrimSpace(ctx.Get(apiHeaderId)))
	ctx.Locals("srv", strings.TrimSpace(ctx.Get(apiHeaderServer)))

	// match uri
	if !m.chunkRegexp.Match([]byte(ctx.Get(apiHeaderUri))) {
		ctx.Locals("errors", errs|errMidAppPreUriRegexp)
		return
	}

	return true
}

// fake quality check
func (m *App) fbMidAppFakeQuality(ctx *fiber.Ctx) error {
	m.lapRequestTimer(ctx, utils.FbCtxReqFakeQuality)
	gLog.Trace().Msg("fake quality check")

	uri := ctx.Get(apiHeaderUri)
	tsr := NewTitleSerieRequest(uri)

	if !tsr.isValid() {
		ctx.Locals("uri", uri)
		return ctx.Next()
	}

	if !gQualityLock.TryRLock() {
		gLog.Warn().Msg("could not get lock for reading quality level; skipping fake quality chain")
		return ctx.Next()
	}
	defer gQualityLock.RUnlock()

	gLog.Debug().Uint16("tsr", uint16(tsr.getTitleQuality())).Uint16("coded", uint16(gQualityLevel)).
		Msg("quality check")
	if tsr.getTitleQuality() <= gQualityLevel {
		ctx.Locals("uri", uri)
		return ctx.Next()
	}

	// precondition finished; quality cool down
	ctx.Locals("uri", m.getUriWithFakeQuality(tsr, uri, gQualityLevel))
	return ctx.Next()
}

// consul lottery
func (m *App) fbMidAppConsulLottery(ctx *fiber.Ctx) error {
	m.lapRequestTimer(ctx, utils.FbCtxReqConsulLottery)
	gLog.Trace().Msg("consul lottery")

	if !gLotteryLock.TryRLock() {
		gLog.Warn().Msg("could not get lock for reading lottery chance; fallback to old method")
		return ctx.Next()
	}

	if gLotteryChance < rand.Intn(99)+1 {
		gLog.Trace().Msg("consul lottery looser, fallback to old method")
		gLotteryLock.RUnlock()
		return ctx.Next()
	}
	gLotteryLock.RUnlock()

	ip, s := m.balancer.getServerByChunkName(
		string(m.chunkRegexp.FindSubmatch(
			[]byte(ctx.Locals("uri").(string)),
		)[utils.ChunkName]),
	)

	if ip == "" {
		gLog.Debug().Msg("consul has no servers for balancing, fallback to old method")
		return ctx.Next()
	}

	srv := strings.ReplaceAll(s.name, "-node", "") + "." + gCli.String("consul-entries-domain")
	ctx.Locals("srv", srv)

	return ctx.Next()
}

// blocklist
func (m *App) fbMidAppBlocklist(ctx *fiber.Ctx) error {
	m.lapRequestTimer(ctx, utils.FbCtxReqBlocklist)

	if !m.isBlocklistEnabled() {
		return ctx.Next()
	}

	if m.blocklist.isExists(ctx.IP()) {
		gLog.Debug().Str("cip", ctx.IP()).Msg("client has been banned, forbid request")
		return fiber.NewError(fiber.StatusForbidden)
	}

	return ctx.Next()
}
