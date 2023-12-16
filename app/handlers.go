package app

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/MindHunter86/addie/balancer"
	"github.com/MindHunter86/addie/utils"
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
	ctx.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)

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
	ctx.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)
	rlog(ctx).Debug().Msg("servers stats reset")

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

	return ctx.SendStatus(fiber.StatusNoContent)
}

func (m *App) fbHndApiBalancerReset(ctx *fiber.Ctx) (e error) {
	ctx.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)
	rlog(ctx).Debug().Msg("upstream reset")

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

	return ctx.SendStatus(fiber.StatusNoContent)
}

func (m *App) fbHndApiBlockIp(ctx *fiber.Ctx) error {
	ctx.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)

	ip := ctx.Query("ip")
	if ip == "" {
		return fiber.NewError(fiber.StatusBadRequest, "given ip is empty")
	}

	if e := gConsul.addIpToBlocklist(ip); e != nil {
		return fiber.NewError(fiber.StatusInternalServerError, e.Error())
	}

	fmt.Fprintln(ctx, ip+" has been banned")
	return ctx.SendStatus(fiber.StatusOK)
}

func (m *App) fbHndApiUnblockIp(ctx *fiber.Ctx) error {
	ctx.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)

	ip := ctx.Query("ip")
	if ip == "" {
		return fiber.NewError(fiber.StatusBadRequest, "given ip is empty")
	}

	if e := gConsul.removeIpFromBlocklist(ip); e != nil {
		return fiber.NewError(fiber.StatusInternalServerError, e.Error())
	}

	fmt.Fprintln(ctx, ip+" has been unbanned")
	return ctx.SendStatus(fiber.StatusOK)
}

func (m *App) fbHndApiBlockReset(ctx *fiber.Ctx) error {
	ctx.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)

	if e := gConsul.resetIpsInBlocklist(); e != nil {
		return fiber.NewError(fiber.StatusInternalServerError, e.Error())
	}

	return ctx.SendStatus(fiber.StatusNoContent)
}

func (m *App) fbHndApiBListSwitch(ctx *fiber.Ctx) error {
	ctx.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)

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

	return ctx.SendStatus(fiber.StatusNoContent)
}

func (m *App) fbHndApiLimiterSwitch(ctx *fiber.Ctx) error {
	ctx.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)

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

	return ctx.SendStatus(fiber.StatusNoContent)
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

	rlog(ctx).Error().Msgf("[falsepositive]> new log level applied - %s", lvl)

	fmt.Fprintln(ctx, lvl+" logger level has been applied")
	return ctx.SendStatus(fiber.StatusOK)
}

func (*App) fbHndApiPreCondErr(ctx *fiber.Ctx) error {
	switch ctx.Locals("errors").(appMidError) {
	case errMidAppPreHeaderUri:
		rlog(ctx).Warn().Msg(errApiPreBadUri.Error())
		ctx.Set("X-Error", errApiPreBadUri.Error())
		ctx.SendString(errApiPreBadUri.Error())
	case errMidAppPreHeaderId:
		rlog(ctx).Warn().Msg(errApiPreBadId.Error())
		ctx.Set("X-Error", errApiPreBadId.Error())
		ctx.SendString(errApiPreBadId.Error())
	case errMidAppPreHeaderServer:
		rlog(ctx).Warn().Msg(errApiPreBadServer.Error())
		ctx.Set("X-Error", errApiPreBadServer.Error())
		ctx.SendString(errApiPreBadServer.Error())
	case errMidAppPreUriRegexp:
		rlog(ctx).Warn().Msg(errApiPreUriRegexp.Error())
		ctx.Set("X-Error", errApiPreUriRegexp.Error())
		ctx.SendString(errApiPreUriRegexp.Error())
	default:
		rlog(ctx).Warn().Msg("unknown error")
	}

	return ctx.SendStatus(fiber.StatusPreconditionFailed)
}

func (m *App) fbHndAppRequestSign(ctx *fiber.Ctx) (e error) {
	m.lapRequestTimer(ctx, utils.FbReqTmrReqSign)
	rlog(ctx).Trace().Msg("new 'sign request' request")

	srv, uri := ctx.Locals("srv").(string), ctx.Locals("uri").(string)
	expires, extra := m.getHlpExtra(
		uri,
		srv,
		ctx.Locals("uid").(string),
	)

	var rrl *url.URL
	if rrl, e = url.Parse(srv + uri); e != nil {
		rlog(ctx).Debug().Str("url_parse", srv+uri).Str("remote_addr", ctx.IP()).
			Msg("could not sign request; url.Parse error")
		return fiber.NewError(fiber.StatusInternalServerError, e.Error())
	}

	var rgs = &url.Values{}
	rgs.Add("expires", expires)
	rgs.Add("extra", extra)
	rrl.RawQuery, rrl.Scheme = rgs.Encode(), "https"

	rlog(ctx).Debug().Str("computed_request", rrl.String()).Str("remote_addr", ctx.IP()).
		Msg("request signing completed")
	ctx.Set(apiHeaderLocation, rrl.String())
	return ctx.SendStatus(fiber.StatusNoContent)
}

func (m *App) fbHndBlcNodesBalance(ctx *fiber.Ctx) error {
	ctx.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)

	rlog(ctx).Trace().Msg("im here!")

	err := m.balanceFiberRequest(ctx, []balancer.Balancer{m.bareBalancer})

	var ferr *fiber.Error
	if errors.As(err, &ferr) {
		rlog(ctx).Trace().Msgf("im here! %d %s", ferr.Code, ferr.Message)
		// ! error here
		return err
	} else if err != nil {
		// ! undefined error here
		panic("undefined error in BM balancer")
	}

	ctx.Set("X-Location", ctx.Locals("srv").(string))
	return ctx.SendStatus(fiber.StatusNoContent)
}
