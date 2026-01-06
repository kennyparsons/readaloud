package main

import (
	_ "embed"
)

//go:embed dist/tts.bundle.js
var TTSBundle []byte
