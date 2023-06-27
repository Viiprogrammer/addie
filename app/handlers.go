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

	ctx.SendString("OK")
	return ctx.SendStatus(fiber.StatusOK)
}

func (m *App) fbHndAppRequestSign(ctx *fiber.Ctx) error {
	var srv string
	if srv = ctx.Locals("srv").(string); srv == "" {
		srv = ctx.Get(apiHeaderServer)
	}

	expires, extra := m.getHlpExtra(
		ctx.Locals("uri").(string),
		ctx.Context().RemoteIP().String(),
		srv,
		ctx.Locals("uid").(string),
	)

	srv = srv + ctx.Locals("uri").(string)
	rrl, e := url.Parse(srv)
	if e != nil {
		gLog.Debug().Str("url_parse", srv).Str("remote_addr", ctx.Context().RemoteIP().String()).
			Msg("could not sign request; url.Parse error")
		return ctx.SendStatus(fiber.StatusInternalServerError)
	}

	var rgs = &url.Values{}
	rgs.Add("expires", expires)
	rgs.Add("extra", extra)
	rrl.RawQuery = rgs.Encode()
	rrl.Scheme = "https"

	gLog.Debug().Str("computed_request", rrl.String()).
		Str("remote_addr", ctx.Context().RemoteIP().String()).Msg("request signing completed")
	ctx.Set(apiHeaderLocation, rrl.String())
	return ctx.SendStatus(fiber.StatusOK)
}
