package app

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/MindHunter86/anilibria-hlp-service/balancer"
	"github.com/MindHunter86/anilibria-hlp-service/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

func (*App) getBalancersClusterArg(ctx *fiber.Ctx) (cluster balancer.BalancerCluster, e error) {
	var buf string
	if buf = ctx.Query("cluster"); buf == "" {
		e = fiber.NewError(fiber.StatusBadRequest, "cluster could not be empty")
		return
	}

	switch buf = strings.TrimSpace(buf); buf {
	case "cache-nodes":
		cluster = balancer.BalancerClusterNodes
	case "cache-cloud":
		cluster = balancer.BalancerClusterCloud
	default:
		e = fiber.NewError(fiber.StatusBadRequest, "invalid cluster name")
	}

	return
}

func (m *App) fbHndApiBalancerStats(ctx *fiber.Ctx) (e error) {
	ctx.Type(fiber.MIMETextPlainCharsetUTF8)

	var cluster balancer.BalancerCluster
	if cluster, e = m.getBalancersClusterArg(ctx); e != nil {
		return
	}

	switch cluster {
	case balancer.BalancerClusterNodes:
		fmt.Fprint(ctx, m.bareBalancer.GetStats())
	case balancer.BalancerClusterCloud:
		fmt.Fprint(ctx, m.cloudBalancer.GetStats())
	}

	return ctx.SendStatus(fiber.StatusOK)
}

func (m *App) fbHndApiStatsReset(ctx *fiber.Ctx) (e error) {
	ctx.Type(fiber.MIMETextPlainCharsetUTF8)
	gLog.Debug().Msg("servers stats reset")

	var cluster balancer.BalancerCluster
	if cluster, e = m.getBalancersClusterArg(ctx); e != nil {
		return
	}

	switch cluster {
	case balancer.BalancerClusterNodes:
		m.bareBalancer.ResetStats()
	case balancer.BalancerClusterCloud:
		m.cloudBalancer.ResetStats()
	}

	ctx.SendString("OK")
	return ctx.SendStatus(fiber.StatusOK)
}

func (m *App) fbHndApiBalancerReset(ctx *fiber.Ctx) (e error) {
	ctx.Type(fiber.MIMETextPlainCharsetUTF8)
	gLog.Debug().Msg("upstream reset")

	var cluster balancer.BalancerCluster
	if cluster, e = m.getBalancersClusterArg(ctx); e != nil {
		return
	}

	switch cluster {
	case balancer.BalancerClusterNodes:
		m.bareBalancer.ResetUpstream()
	case balancer.BalancerClusterCloud:
		m.cloudBalancer.ResetUpstream()
	}

	ctx.SendString("OK")
	return ctx.SendStatus(fiber.StatusOK)
}

func (m *App) fbHndApiBlockIp(ctx *fiber.Ctx) error {
	ctx.Type(fiber.MIMETextHTMLCharsetUTF8)

	ip := ctx.Query("ip")
	if ip == "" {
		return fiber.NewError(fiber.StatusBadRequest, "given ip is empty")
	}

	if e := gConsul.addIpToBlocklist(ip); e != nil {
		return fiber.NewError(fiber.StatusInternalServerError, e.Error())
	}

	return ctx.SendStatus(fiber.StatusOK)
}

func (m *App) fbHndApiUnblockIp(ctx *fiber.Ctx) error {
	ctx.Type(fiber.MIMETextHTMLCharsetUTF8)

	ip := ctx.Query("ip")
	if ip == "" {
		return fiber.NewError(fiber.StatusBadRequest, "given ip is empty")
	}

	if e := gConsul.removeIpFromBlocklist(ip); e != nil {
		return fiber.NewError(fiber.StatusInternalServerError, e.Error())
	}

	return ctx.SendStatus(fiber.StatusOK)
}

func (m *App) fbHndApiBlockReset(ctx *fiber.Ctx) error {
	ctx.Type(fiber.MIMETextHTMLCharsetUTF8)

	if e := gConsul.resetIpsInBlocklist(); e != nil {
		return fiber.NewError(fiber.StatusInternalServerError, e.Error())
	}

	return ctx.SendStatus(fiber.StatusOK)
}

func (m *App) fbHndApiBListSwitch(ctx *fiber.Ctx) error {
	ctx.Type(fiber.MIMETextHTMLCharsetUTF8)

	enabled := ctx.Query("enabled")
	switch enabled {
	case "0":
		fallthrough
	case "1":
		if e := gConsul.updateBlocklistSwitcher(enabled); e != nil {
			return fiber.NewError(fiber.StatusInternalServerError, e.Error())
		}
	default:
		return fiber.NewError(fiber.StatusBadRequest, "enabled query can be only 0 or 1")
	}

	return ctx.SendStatus(fiber.StatusOK)
}

func (m *App) fbHndApiLimiterSwitch(ctx *fiber.Ctx) error {
	ctx.Type(fiber.MIMETextHTMLCharsetUTF8)

	enabled := ctx.Query("enabled")
	switch enabled {
	case "0":
		fallthrough
	case "1":
		if e := gConsul.updateLimiterSwitcher(enabled); e != nil {
			return fiber.NewError(fiber.StatusInternalServerError, e.Error())
		}
	default:
		return fiber.NewError(fiber.StatusBadRequest, "enabled query can be only 0 or 1")
	}

	return ctx.SendStatus(fiber.StatusOK)
}

func (m *App) fbHndApiLoggerLevel(ctx *fiber.Ctx) error {
	lvl := ctx.Query("level")

	switch lvl {
	case "trace":
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		return fiber.NewError(fiber.StatusBadRequest, "unknown level sent")
	}

	gLog.Error().Msgf("[falsepositive]> new log level applied - %s", lvl)

	return ctx.SendStatus(fiber.StatusOK)
}

func (*App) fbHndApiPreCondErr(ctx *fiber.Ctx) error {
	switch ctx.Locals("errors").(appMidError) {
	case errMidAppPreHeaderUri:
		gLog.Warn().Msg(errApiPreBadUri.Error())
		ctx.Set("X-Error", errApiPreBadUri.Error())
		ctx.SendString(errApiPreBadUri.Error())
	case errMidAppPreHeaderId:
		gLog.Warn().Msg(errApiPreBadId.Error())
		ctx.Set("X-Error", errApiPreBadId.Error())
		ctx.SendString(errApiPreBadId.Error())
	case errMidAppPreHeaderServer:
		gLog.Warn().Msg(errApiPreBadServer.Error())
		ctx.Set("X-Error", errApiPreBadServer.Error())
		ctx.SendString(errApiPreBadServer.Error())
	case errMidAppPreUriRegexp:
		gLog.Warn().Msg(errApiPreUriRegexp.Error())
		ctx.Set("X-Error", errApiPreUriRegexp.Error())
		ctx.SendString(errApiPreUriRegexp.Error())
	default:
		gLog.Warn().Msg("unknown error")
	}

	return ctx.SendStatus(fiber.StatusPreconditionFailed)
}

func (m *App) fbHndAppRequestSign(ctx *fiber.Ctx) error {
	m.lapRequestTimer(ctx, utils.FbReqTmrReqSign)
	gLog.Trace().Msg("new `sign request` request")

	srv := ctx.Locals("srv").(string)
	expires, extra := m.getHlpExtra(
		ctx.Locals("uri").(string),
		srv,
		ctx.Locals("uid").(string),
	)

	srv = srv + ctx.Locals("uri").(string)
	rrl, e := url.Parse(srv)
	if e != nil {
		gLog.Debug().Str("url_parse", srv).Str("remote_addr", ctx.IP()).
			Msg("could not sign request; url.Parse error")
		return ctx.SendStatus(fiber.StatusInternalServerError)
	}

	var rgs = &url.Values{}
	rgs.Add("expires", expires)
	rgs.Add("extra", extra)
	rrl.RawQuery = rgs.Encode()
	rrl.Scheme = "https"

	gLog.Debug().Str("computed_request", rrl.String()).Str("remote_addr", ctx.IP()).
		Msg("request signing completed")
	ctx.Set(apiHeaderLocation, rrl.String())
	return ctx.SendStatus(fiber.StatusOK)
}

func (m *App) fbHndBlcNodesBalance(ctx *fiber.Ctx) error {

	uri := ctx.Locals("uri").(*string)
	sub := m.chunkRegexp.FindSubmatch([]byte(*uri))

	buf := bytes.NewBuffer(sub[utils.ChunkTitleId])
	buf.Write(sub[utils.ChunkQualityLevel])

	_, server, e := m.bareBalancer.BalanceByChunk(buf.String(), string(sub[utils.ChunkName]))
	if errors.Is(e, balancer.ErrServerUnavailable) {
		gLog.Warn().Err(e).Msg("balancer error; fallback to old method")
		return ctx.Next()
	}

	srv := strings.ReplaceAll(server.Name, "-node", "") + "." + gCli.String("consul-entries-domain")
	ctx.Set("X-Location", srv)

	ctx.Type(fiber.MIMETextPlainCharsetUTF8)
	return ctx.SendStatus(fiber.StatusOK)
}
