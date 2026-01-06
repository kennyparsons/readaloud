package main

import (
	_ "embed"
)

//go:embed build/tts.bundle.js
var TTSBundle []byte
