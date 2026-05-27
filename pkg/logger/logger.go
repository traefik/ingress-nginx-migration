package logger

import (
	"io"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Setup is configuring the logger to write to out.
// One-shot output modes pass os.Stderr so that stdout stays reserved for the
// machine- or human-readable report.
func Setup(level string, out io.Writer) {
	writer := zerolog.ConsoleWriter{
		Out:        out,
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
