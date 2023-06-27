package main

import (
	"fmt"
	_ "net/http/pprof"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"

	application "github.com/MindHunter86/anilibria-hlp-service/app"
)

var version = "devel" // -ldflags="-X 'main.version=X.X.X'"

func main() {
	// logger
	log := zerolog.New(zerolog.ConsoleWriter{
		Out: os.Stderr,
	}).With().Timestamp().Logger().Hook(SeverityHook{})
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	// application
	app := cli.NewApp()
	cli.VersionFlag = &cli.BoolFlag{Name: "version", Aliases: []string{"V"}}

	app.Name = "anilibria-hlp-service"
	app.Version = version
	app.Compiled = time.Now()
	app.Authors = []*cli.Author{
		&cli.Author{
			Name:  "MindHunter86",
			Email: "mindhunter86@vkom.cc",
		},
	}
	app.Copyright = "(c) 2022-2023 mindhunter86\nwith love for Anilibria project"
	app.Usage = "Hotlink Protection Service for Anilibria project"

	app.Flags = []cli.Flag{
		// common flags
		&cli.StringFlag{
			Name:    "log-level",
			Aliases: []string{"l"},
			Value:   "debug",
			Usage:   "levels: trace, debug, info, warn, err, panic, disabled",
			EnvVars: []string{"LOG_LEVEL"},
		},
		&cli.BoolFlag{
			Name:    "quite",
			Aliases: []string{"q"},
			Usage:   "Flag is equivalent to --log-level=quite",
		},
		// http client settings
		&cli.BoolFlag{
			Name:  "http-client-insecure",
			Usage: "Flag for TLS certificate verification disabling",
		},
		&cli.DurationFlag{
			Name:  "http-client-timeout",
			Usage: "Internal HTTP client connection `TIMEOUT` (format: 1000ms, 1s)",
			Value: 3 * time.Second,
		},
		&cli.DurationFlag{
			Name:  "http-tcp-timeout",
			Usage: "",
			Value: 1 * time.Second,
		},
		&cli.DurationFlag{
			Name:  "http-tls-handshake-timeout",
			Usage: "",
			Value: 1 * time.Second,
		},
		&cli.DurationFlag{
			Name:  "http-idle-timeout",
			Usage: "",
			Value: 300 * time.Second,
		},
		&cli.DurationFlag{
			Name:  "http-keepalive-timeout",
			Usage: "",
			Value: 300 * time.Second,
		},
		&cli.IntFlag{
			Name:  "http-max-idle-conns",
			Usage: "",
			Value: 100,
		},
		&cli.BoolFlag{
			Name:  "http-debug",
			Usage: "",
		},

		// fiber settings
		&cli.StringFlag{
			Name:  "http-listen-addr",
			Usage: "Ex: 127.0.0.1:8080, :8080",
			Value: "127.0.0.1:8080",
		},
		&cli.StringFlag{
			Name:  "http-trusted-proxies",
			Usage: "Ex: 10.0.0.0/8; Separated by comma",
		},
		&cli.BoolFlag{
			Name: "http-prefork",
			Usage: `Enables use of the SO_REUSEPORT socket option;
			if enabled, the application will need to be ran
			through a shell because prefork mode sets environment variables`,
		},
		&cli.BoolFlag{
			Name:  "http-cors",
			Usage: "enable cors requests serving",
			Value: true,
		},
		&cli.BoolFlag{
			Name:  "http-pprof-enable",
			Usage: "enable golang http-pprof methods",
		},

		// anilibria settings
		&cli.StringFlag{
			Name:  "anilibria-baseurl",
			Usage: "",
			Value: "https://www.anilibria.tv",
		},
		&cli.StringFlag{
			Name:  "anilibria-api-baseurl",
			Usage: "",
			Value: "https://api.anilibria.tv/v2",
		},

		// ip ban settigns
		&cli.BoolFlag{
			Name:  "ip-ban-disable",
			Usage: "",
			Value: true,
		},
		&cli.DurationFlag{
			Name:  "ip-ban-time",
			Usage: "",
			Value: 60 * time.Minute,
		},

		// ...
		&cli.DurationFlag{
			Name:  "link-expiration",
			Usage: "",
			Value: 10 * time.Second,
		},
		&cli.StringFlag{
			Name:        "link-secret",
			Usage:       "",
			Value:       "TZj3Ts1LsvkX",
			EnvVars:     []string{"SIGN_SECRET"},
			DefaultText: "CHANGE DEFAULT SECRET",
		},

		// consul settings
		&cli.BoolFlag{
			Name: "consul-managed",
		},
		&cli.BoolFlag{
			Name: "consul-ignore-errors",
		},
		&cli.StringFlag{
			Name:    "consul-address",
			Usage:   "consul API uri",
			Value:   "http://127.0.0.1:8500",
			EnvVars: []string{"CONSUL_ADDRESS"},
		},
		&cli.StringFlag{
			Name:  "consul-service-name",
			Usage: "service name (id) used for balancing",
		},
		&cli.StringFlag{
			Name:  "consul-entries-domain",
			Usage: "add domain for all service entries",
			Value: "libria.fun",
		},
		&cli.IntFlag{
			Name:  "consul-ab-split",
			Usage: "percent",
			Value: 0,
		},
		&cli.StringFlag{
			Name:  "consul-kv-prefix",
			Value: fmt.Sprintf("anilibria/%s", app.Name),
		},
	}

	app.Action = func(c *cli.Context) (e error) {
		var lvl zerolog.Level
		if lvl, e = zerolog.ParseLevel(c.String("log-level")); e != nil {
			log.Fatal().Err(e)
		}

		zerolog.SetGlobalLevel(lvl)
		if c.Bool("quite") {
			zerolog.SetGlobalLevel(zerolog.Disabled)
		}

		if !fiber.IsChild() {
			log.Info().Msg("ready...")
			log.Info().Msgf("system cpu count %d", runtime.NumCPU())
			log.Info().Strs("args", os.Args).Msg("")
		} else {
			log.Info().Msgf("system cpu count %d", runtime.NumCPU())
			log.Info().Msgf("old cpu count %d", runtime.GOMAXPROCS(1))
			log.Info().Msgf("new cpu count %d", runtime.GOMAXPROCS(1))
		}

		return application.NewApp(c, &log).Bootstrap()
	}

	// TODO sort.Sort of Flags uses too much allocs; temporary disabled
	// sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	if e := app.Run(os.Args); e != nil {
		log.Fatal().Err(e).Msg("")
	}
}

type SeverityHook struct{}

func (SeverityHook) Run(e *zerolog.Event, level zerolog.Level, _ string) {
	if level > zerolog.DebugLevel || version != "devel" {
		return
	}

	rfn := "unknown"
	pcs := make([]uintptr, 1)

	if runtime.Callers(4, pcs) != 0 {
		if fun := runtime.FuncForPC(pcs[0] - 1); fun != nil {
			rfn = fun.Name()
		}
	}

	fn := strings.Split(rfn, "/")
	e.Str("func", fn[len(fn)-1:][0])
}
