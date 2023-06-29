//go:build !windows

package utils

import (
	"log/syslog"
	"os"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
)

func SetUpSyslogWriter(c *cli.Context) (_ *zerolog.Logger, e error) {
	var sylog *syslog.Writer
	if sylog, e = syslog.Dial(
		c.String("syslog-proto"),
		c.String("syslog-server"),
		syslog.LOG_INFO,
		c.String("syslog-tag"),
	); e != nil {
		return
	}

	log := zerolog.New(zerolog.MultiLevelWriter(
		zerolog.ConsoleWriter{Out: os.Stderr},
		sylog,
	)).With().Timestamp().Logger()

	return &log, e
}
