//go:build windows

package utils

import (
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
)

func SetUpSyslogWriter(c *cli.Context) (_ *zerolog.Logger, e error) {
	return nil, nil
}
