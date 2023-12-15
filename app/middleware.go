package app

import (
	"bytes"
	"errors"
	"math/rand"
	"strings"

	"github.com/MindHunter86/addie/balancer"
	"github.com/MindHunter86/addie/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
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
// 		rlog(ctx).Debug().Str("remote_addr", ctx.RemoteIP().String()).Str("x_forwarded_for", cip).Msg("")
// 		m.hlpRespondError(&ctx.Response, errHlpBadIp)
// 		return
// 	}
// }

// API precondition check
func (m *App) fbMidAppPreCond(ctx *fiber.Ctx) (skip bool) {
	m.lapRequestTimer(ctx, utils.FbReqTmrPreCond)
	var errs appMidError

	rlog(ctx).Trace().Interface("hdrs", ctx.GetReqHeaders()).Msg("debug")
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
	m.lapRequestTimer(ctx, utils.FbReqTmrFakeQuality)
	rlog(ctx).Trace().Msg("fake quality check")

	uri := ctx.Get(apiHeaderUri)
	tsr := NewTitleSerieRequest(uri)

	if !tsr.isValid() {
		ctx.Locals("uri", uri)
		return ctx.Next()
	}

	quality, ok := m.runtime.GetQualityLevel()
	if !ok {
		rlog(ctx).Warn().Msg("could not get lock for reading quality level; skipping fake quality chain")
		return ctx.Next()
	}

	rlog(ctx).Debug().Uint16("tsr", uint16(tsr.getTitleQuality())).Uint16("coded", uint16(quality)).
		Msg("quality check")
	if tsr.getTitleQuality() <= quality {
		ctx.Locals("uri", uri)
		return ctx.Next()
	}

	// precondition finished; quality cool down
	ctx.Locals("uri", m.getUriWithFakeQuality(tsr, uri, quality))
	return ctx.Next()
}

// if return value == true - Balance() will be skipped
func (m *App) fbMidAppBalancerLottery(ctx *fiber.Ctx) bool {
	lottery, ok := m.runtime.GetLotteryChance()
	if !ok {
		rlog(ctx).Warn().Msg("could not get lock for reading lottery chance; fallback to old method")
		return !ok
	}

	return lottery < rand.Intn(99)+1
}

func (m *App) fbMidAppBalance(ctx *fiber.Ctx) (e error) {
	m.lapRequestTimer(ctx, utils.FbReqTmrConsulLottery)
	rlog(ctx).Trace().Msg("consul lottery winner, rewriting destination server...")

	var server *balancer.BalancerServer
	uri := []byte(ctx.Locals("uri").(string))

	prefixbuf := bytes.NewBuffer(m.chunkRegexp.FindSubmatch(uri)[utils.ChunkTitleId])
	prefixbuf.Write(m.chunkRegexp.FindSubmatch(uri)[utils.ChunkEpisodeId])
	prefixbuf.Write(m.chunkRegexp.FindSubmatch(uri)[utils.ChunkQualityLevel])

	try, retries := uint8(balancer.MaxTries), uint8(0)

	// TODO
	// ? do we need the failover with RandomBalancing

	// simplify the upstream switching (using) with constant slice
	// minifies code and removes copy-paste blocks
	for _, cluster := range []balancer.Balancer{m.cloudBalancer, m.bareBalancer} {

		// common && backup balancing with one upstream ("balancer")
		// loop used for retries after errors and for fallback to a backup server
		// err count has limited by `balancer.BalancerMaxRetries`
		for ; try != 0; try-- {

			// trying to balance with giver cluster
			// `try` used as a collision for slice with available servers
			// works like (`X` * maxTries) * -1
			_, server, e = cluster.BalanceByChunkname(
				prefixbuf.String(),
				string(m.chunkRegexp.FindSubmatch(uri)[utils.ChunkName]),
				try,
			)

			var berr *balancer.BalancerError
			if e == nil {
				// if all ok (if no errors) - save destination and go to the next fiber handler:
				ctx.Locals("srv",
					strings.ReplaceAll(server.Name, "-node", "")+"."+gCli.String("consul-entries-domain"))

				return ctx.Next()
			} else if !errors.As(e, &berr) {
				panic("balancer internal error - undefined error")
			}

			if zerolog.GlobalLevel() <= zerolog.DebugLevel && berr.HasError() {
				gLog.Debug().Err(berr).Msg("error")
			}

			// here we accept only 3 retry for one server in current "balance session"
			// if TryLock in balancer always fails, balancer session in sum has 6 tries for request
			// (if MaxTries == 3)
			if berr.Has(balancer.IsRetriable) {
				rlog(ctx).Trace().Uint8("try", try).Msg("undefined error")
				if retries += 1; retries < balancer.MaxTries {
					try++
				}
				continue
			} else if berr.Has(balancer.IsBackupable) {
				rlog(ctx).Trace().Uint8("try", try).Msg("IsNextServerRouted")
				// ! use backup server
				continue
			} else if berr.Has(balancer.IsReroutable) {
				rlog(ctx).Trace().Uint8("try", try).Msg("IsNextClusterRouted")
				// ! use next cluster
				break
			}

			panic("balancer internal error - undefined error flag")
		}
	}

	// if we here - no alive balancers, so return error
	return fiber.NewError(fiber.StatusInternalServerError, e.Error())
}

// blocklist
func (m *App) fbMidAppBlocklist(ctx *fiber.Ctx) error {
	m.lapRequestTimer(ctx, utils.FbReqTmrBlocklist)

	if !m.blocklist.IsEnabled() {
		return ctx.Next()
	}

	if m.blocklist.IsExists(ctx.IP()) {
		rlog(ctx).Debug().Str("cip", ctx.IP()).Msg("client has been banned, forbid request")
		return fiber.NewError(fiber.StatusForbidden)
	}

	return ctx.Next()
}

// balancer api
func (m *App) fbMidBlcPreCond(ctx *fiber.Ctx) bool {
	m.lapRequestTimer(ctx, utils.FbReqTmrBlcPreCond)
	rlog(ctx).Trace().Interface("hdrs", ctx.GetReqHeaders()).Msg("cache-node-internal balancer")

	var errs appMidError

	if huri := strings.TrimSpace(ctx.Get(apiHeaderUri)); huri == "" {
		errs = errs | errMidAppPreHeaderUri
	} else if !m.chunkRegexp.Match([]byte(huri)) {
		errs = errs | errMidAppPreUriRegexp
	} else {
		ctx.Locals("uri", &huri)
	}

	ctx.Locals("errors", errs)
	return errs == 0
}
