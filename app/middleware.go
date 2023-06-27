package app

import (
	"errors"
	"math/rand"
	"strings"

	"github.com/gofiber/fiber/v2"
)

var (
	errApiPreBadHeaders = errors.New("could not parse required headers")
)

const (
	apiHeaderUri      = "X-Client-URI"
	apiHeaderId       = "X-Client-ID"
	apiHeaderServer   = "X-Cache-Server"
	apiHeaderLocation = "X-Location"
)

const (
	errMidAppPreHeaderUri = 1 << iota
	errMidAppPreHeaderId
	errMidAppPreHeaderServer
	errMidAppPreUidFromReq
	errMidAppPreUriRegexp
)

// API precondition check
func (m *App) fbMidAppPreCond(ctx *fiber.Ctx) (skip bool) {
	var errs int

	switch h := ctx.GetReqHeaders(); {
	case h[apiHeaderUri] == "":
		ctx.Locals("errors", errs|errMidAppPreHeaderUri)
		return
	case h[apiHeaderId] == "":
		ctx.Locals("errors", errs|errMidAppPreHeaderId)
		return
	case h[apiHeaderServer] == "":
		ctx.Locals("errors", errs|errMidAppPreHeaderServer)
		return
	}

	// parse title uid from given request
	uid := m.getUidFromRequest(ctx.Get(apiHeaderUri))
	if uid == "" {
		ctx.Locals("errs", errs|errMidAppPreUidFromReq)
		return false
	}

	ctx.Locals("uid", uid)

	// match uri
	if !m.chunkRegexp.Match([]byte(ctx.Get(apiHeaderUri))) {
		ctx.Locals("errs", errs|errMidAppPreUriRegexp)
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
	if gLotteryChance < rand.Intn(99)+1 {
		gLog.Trace().Msg("consul lottery looser, fallback to old method")
		return ctx.Next()
	}

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
