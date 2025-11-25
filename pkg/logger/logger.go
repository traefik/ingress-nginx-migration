package logger

import (
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Setup is configuring the logger.
func Setup(level string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	log.Logger = zerolog.New(os.Stderr).With().Caller().Timestamp().Logger()
	zerolog.DefaultContextLogger = &log.Logger

	logLevel, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil {
		log.Debug().Err(err).
			Str("LOG_LEVEL", level).
			Msg("Unspecified or invalid log level, setting the level to default (INFO)...")

		logLevel = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(logLevel)

	log.Trace().Msgf("Log level set to %s.", logLevel)
}
