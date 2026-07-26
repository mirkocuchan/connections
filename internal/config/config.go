package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	DBURL string
}