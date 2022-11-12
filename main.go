package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"

	application "github.com/MindHunter86/anilibria-hlp-service/app"
)

var version = "devel" // -ldflags="-X 'main.version=X.X.X'"

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6063", nil))
	}()

	// logger
	log := zerolog.New(zerolog.ConsoleWriter{
		Out: os.Stderr,
	}).With().Timestamp().Logger().Hook(SeverityHook{})
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	// application
	app := cli.NewApp()
	cli.VersionFlag = &cli.BoolFlag{Name: "print-version", Aliases: []string{"V"}}

	app.Name = "anilibria-hlp-service"
	app.Version = version
	app.Compiled = time.Now()
	app.Authors = []*cli.Author{
		&cli.Author{
			Name:  "MindHunter86",
			Email: "mindhunter86@vkom.cc",
		},
	}
	app.Copyright = "(c) 2022 mindhunter86"
	app.Usage = "N\\A"

	app.Flags = []cli.Flag{
		// common flags
		&cli.IntFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Value:   5,
			Usage:   "Verbose `LEVEL` (value from 5(debug) to 0(panic) and -1 for log disabling(quite mode))",
		},
		&cli.BoolFlag{
			Name:    "quite",
			Aliases: []string{"q"},
			Usage:   "Flag is equivalent to verbose -1",
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

		// ...
		&cli.DurationFlag{
			Name:  "link-expiration",
			Usage: "",
			Value: 30 * time.Second,
		},
		&cli.StringFlag{
			Name:        "link-secret",
			Usage:       "",
			Value:       "TZj3Ts1LsvkX",
			DefaultText: "CHANGE DEFAULT SECRET",
		},
	}

	app.Action = func(c *cli.Context) error {
		// log.Debug().Msg("ready...")
		// log.Debug().Strs("args", os.Args).Msg("")

		// TODO
		// if c.Int("verbose") < -1 || c.Int("verbose") > 5 {
		// 	log.Fatal().Msg("There is invalid data in verbose option. Option supports values for -1 to 5")
		// }

		// zerolog.SetGlobalLevel(zerolog.Level(int8((c.Int("verbose") - 5) * -1)))
		// if c.Int("verbose") == -1 || c.Bool("quite") {
		// 	zerolog.SetGlobalLevel(zerolog.Disabled)
		// }

		return os.ErrInvalid
	}

	app.Commands = []*cli.Command{
		&cli.Command{
			Name:  "test",
			Usage: "",
			Action: func(c *cli.Context) (e error) {
				return application.NewApp(c, &log).Bootstrap()
			},
		},
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
	if level != zerolog.DebugLevel && version != "devel" {
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