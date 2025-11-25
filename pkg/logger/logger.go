package logger

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Setup is configuring the logger.
func Setup(level string) {
	var w io.Writer = os.Stdout
	writer := zerolog.ConsoleWriter{
		Out:        w,
		TimeFormat: time.RFC3339,
	}

	log.Logger = zerolog.New(writer).With().Caller().Timestamp().Logger()
	zerolog.DefaultContextLogger = &log.Logger

	logLevel, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil {
		log.Debug().Err(err).
			Str("log_level", level).
			Msg("Unspecified or invalid log level, setting the level to default (INFO)...")

		logLevel = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(logLevel)

	log.Trace().Msgf("Log level set to %s.", logLevel)
}
