package app

import (
	"fmt"
	"net/url"

	"github.com/gofiber/fiber/v2"
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

// func (*App) hlpRespondError(r *fasthttp.Response, err error, status ...int) {
// 	status = append(status, fasthttp.StatusInternalServerError)

// 	r.Header.Set("X-Error", err.Error())
// 	r.SetStatusCode(status[0])

// 	gLog.Error().Err(err).Msg("")
// }

func (*App) fbHndApiPreCondErr(ctx *fiber.Ctx) error {
	switch ctx.Locals("errors").(appMidError) {
	case errMidAppPreHeaderUri:
		gLog.Warn().Msg(errApiPreBadUri.Error())
		ctx.SendString(errApiPreBadUri.Error())
	case errMidAppPreHeaderId:
		gLog.Warn().Msg(errApiPreBadId.Error())
		ctx.SendString(errApiPreBadId.Error())
	case errMidAppPreHeaderServer:
		gLog.Warn().Msg(errApiPreBadServer.Error())
		ctx.SendString(errApiPreBadServer.Error())
	case errMidAppPreUidFromReq:
		gLog.Warn().Msg(errApiPreUidParse.Error())
		ctx.SendString(errApiPreUidParse.Error())
	case errMidAppPreUriRegexp:
		gLog.Warn().Msg(errApiPreUriRegexp.Error())
		ctx.SendString(errApiPreUriRegexp.Error())
	default:
		gLog.Warn().Msg("unknown error")
	}

	return ctx.SendStatus(fiber.StatusPreconditionFailed)
}

func (m *App) fbHndAppRequestSign(ctx *fiber.Ctx) error {
	gLog.Debug().Msg("new `sign request` request")

	srv := ctx.Get(apiHeaderServer)
	expires, extra := m.getHlpExtra(
		ctx.Locals("uri").(string),
		ctx.Context().RemoteIP().String(),
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
