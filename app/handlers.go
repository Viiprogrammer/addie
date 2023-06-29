package app

import (
	"fmt"
	"net/url"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

func (m *App) fbHndApiUpstream(ctx *fiber.Ctx) error {
	fmt.Fprint(ctx, m.balancer.getUpstreamStats())
	ctx.Type(fiber.MIMETextHTMLCharsetUTF8)
	return ctx.SendStatus(fiber.StatusOK)
}

func (m *App) fbHndApiReset(ctx *fiber.Ctx) error {
	m.balancer.resetServersStats()
	ctx.Type(fiber.MIMETextHTMLCharsetUTF8)

	gLog.Debug().Msg("servers reset")
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

	gLog.Error().Msgf("[falsepositive]> new log level applied - %s", gLog.GetLevel().String())

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
