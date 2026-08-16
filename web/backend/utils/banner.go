package utils

import (
	"fmt"

	"github.com/Kantemba/clawy/pkg/config"
)

func Banner() string {
	return fmt.Sprintf("\r\nClawy %s\r\n", config.FormatVersion())
}
