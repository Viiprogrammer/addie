package app

import (
	"errors"
	"math/rand"
	"strings"

	"github.com/gofiber/fiber/v2"
)

var (
	errApiPreBadHeaders = errors.New("could not parse required headers")
	errApiPreBadUri     = errors.New("invalid uri")
	errApiPreBadId      = errors.New("invalid id")
	errApiPreBadServer  = errors.New("invalid server")
	errApiPreUidParse   = errors.New("got a problem in uid parsing")
	errApiPreUriRegexp  = errors.New("regexp matching failure")
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
	errMidAppPreUidFromReq
	errMidAppPreUriRegexp
)

// API precondition check
func (m *App) fbMidAppPreCond(ctx *fiber.Ctx) (skip bool) {
	var errs appMidError

	gLog.Trace().Interface("hdrs", ctx.GetReqHeaders()).Msg("debug")
	switch h := ctx.GetReqHeaders(); {
	case h[apiHeaderUri] == "":
		errs = errs | errMidAppPreHeaderUri
		ctx.Locals("errors", errs)
		return
	case h[apiHeaderId] == "":
		errs = errs | errMidAppPreHeaderId
		ctx.Locals("errors", errs)
		return
	case h[apiHeaderServer] == "":
		errs = errs | errMidAppPreHeaderServer
		ctx.Locals("errors", errs)
		return
	}

	// parse title uid from given request
	uid := m.getUidFromRequest(ctx.Get(apiHeaderUri))
	if uid == "" {
		ctx.Locals("errs", errs|errMidAppPreUidFromReq)
		return
	}

	ctx.Locals("uid", uid)
	ctx.Locals("srv", ctx.Get(apiHeaderServer))

	// match uri
	if !m.chunkRegexp.Match([]byte(ctx.Get(apiHeaderUri))) {
		ctx.Locals("errs", errs|errMidAppPreUriRegexp)
		return
	}

	return true
}

// fake quality check
func (m *App) fbMidAppFakeQuality(ctx *fiber.Ctx) error {
	gLog.Trace().Msg("fake quality check")

	uri := ctx.Get(apiHeaderUri)
	tsr := NewTitleSerieRequest(uri)

	if !tsr.isValid() {
		ctx.Locals("uri", uri)
		return ctx.Next()
	}

	gQualityLock.RLock()
	defer gQualityLock.RUnlock()

	log.Debug().Uint16("tsr", uint16(tsr.getTitleQuality())).Uint16("coded", uint16(gQualityLevel)).
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
	gLog.Trace().Msg("consul lottery")

	gLotteryLock.RLock()
	if gLotteryChance < rand.Intn(99)+1 {
		gLog.Trace().Msg("consul lottery looser, fallback to old method")
		gLotteryLock.RUnlock()
		return ctx.Next()
	}
	gLotteryLock.RUnlock()

	ip, s := m.balancer.getServerByChunkName(
		string(m.chunkRegexp.FindSubmatch(
			[]byte(ctx.Locals("uri").(string)),
		)[chunkName]),
	)

	if ip == "" {
		gLog.Debug().Msg("consul has no servers for balancing, fallback to old method")
		return ctx.Next()
	}

	srv := strings.ReplaceAll(s.name, "-node", "") + "." + gCli.String("consul-entries-domain")
	ctx.Locals("srv", srv)

	return ctx.Next()
}
