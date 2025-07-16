package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/surfaceyu/edge-tts-go/edgeTTS"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

const (
	appName = "readaloud"
)

type Config struct {
	Voice  string `yaml:"voice"`
	Rate   string `yaml:"rate"`
	Volume string `yaml:"volume"`
}

func main() {
	var (
		voiceFlag      string
		rateFlag       string
		volumeFlag     string
		textFilePath   string
		writeMediaFile string
	)

	pflag.StringVarP(&voiceFlag, "voice", "v", "", "Voice for TTS (e.g., en-US-AriaNeural)")
	pflag.StringVarP(&rateFlag, "rate", "r", "", "Set TTS rate (e.g., -10%)")
	pflag.StringVarP(&volumeFlag, "volume", "u", "", "Set TTS volume (e.g., 0%)")
	pflag.StringVarP(&textFilePath, "file", "f", "", "Path to a text file for TTS input")
	pflag.StringVarP(&writeMediaFile, "write-media", "w", "", "Write media output to file instead of playing (MP3)")

	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage of %s:
  %s [options] [text]
  %s --file <path> [options]
  echo "Hello world" | %s [options]

Options:
`, appName, appName, appName, appName)
		pflag.PrintDefaults()
	}

	pflag.Parse()

	// Load config from file
	config, err := loadConfig()
	if err != nil {
		log.Printf("Warning: Could not load config file: %v\n", err)
		config = &Config{} // Use empty config if loading fails
	}

	// Apply command-line overrides
	if voiceFlag != "" {
		config.Voice = voiceFlag
	}
	if rateFlag != "" {
		config.Rate = rateFlag
	}
	if volumeFlag != "" {
		config.Volume = volumeFlag
	}

	// Determine input text
	var inputText string
	stat, _ := os.Stdin.Stat()
	isStdin := (stat.Mode() & os.ModeCharDevice) == 0

	if textFilePath != "" && isStdin {
		pflag.Usage()
		log.Fatalf("Error: Cannot use both --file and stdin for input.")
	} else if textFilePath != "" {
		data, err := ioutil.ReadFile(textFilePath)
		if err != nil {
			log.Fatalf("Error reading file %s: %v", textFilePath, err)
		}
		inputText = string(data)
	} else if isStdin {
		reader := bufio.NewReader(os.Stdin)
		inputBytes, err := ioutil.ReadAll(reader)
		if err != nil {
			log.Fatalf("Error reading from stdin: %v", err)
		}
		inputText = string(inputBytes)
	} else if pflag.NArg() > 0 {
		inputText = strings.Join(pflag.Args(), " ")
	} else {
		pflag.Usage()
		log.Fatalf("Error: No input provided. Use --file, pipe to stdin, or provide text as arguments.")
	}

	if inputText == "" {
		pflag.Usage()
		log.Fatalf("Error: Input text is empty.")
	}

	// Determine the output file path
	outputFilePath := writeMediaFile
	var tempFile *os.File

	if outputFilePath == "" {
		tempFile, err = ioutil.TempFile("", "readaloud-*.mp3")
		if err != nil {
			log.Fatalf("Error creating temporary file: %v", err)
		}
		outputFilePath = tempFile.Name()
		defer os.Remove(outputFilePath) // Clean up the temporary file
		defer tempFile.Close()
	}

	// Prepare TTS arguments
	args := edgeTTS.Args{
		Text:       inputText,
		Voice:      config.Voice,
		Rate:       config.Rate,
		Volume:     config.Volume,
		WriteMedia: outputFilePath, // Always write to a file (temp or user-specified)
	}

	// Generate audio
	tts := edgeTTS.NewTTS(args)
	tts.AddText(args.Text, args.Voice, args.Rate, args.Volume)
	tts.Speak()

	if writeMediaFile == "" {
		// Play audio if not writing to a user-specified file
		playAudio(outputFilePath)
	}
}

func loadConfig() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not get user home directory: %w", err)
	}
	configPath := filepath.Join(homeDir, ".config", appName, appName+".yaml")

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("could not read config file %s: %w", configPath, err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("could not unmarshal config file %s: %w", configPath, err)
	}
	return &config, nil
}

func playAudio(filePath string) {
	var cmdName string
	var cmdArgs []string

	switch runtime.GOOS {
	case "darwin":
		cmdName = "afplay"
		cmdArgs = []string{filePath}
	case "linux":
		// Try mpg123 first, then play (sox)
		if _, err := os.Stat("/usr/bin/mpg123"); err == nil {
			cmdName = "mpg123"
			cmdArgs = []string{filePath}
		} else if _, err := os.Stat("/usr/bin/play"); err == nil {
			cmdName = "play"
			cmdArgs = []string{filePath}
		} else {
			log.Println("Warning: No suitable audio player found (mpg123 or play). Please install one to enable audio playback.")
			return
		}
	default:
		log.Printf("Warning: Audio playback not supported on %s.", runtime.GOOS)
		return
	}

	fmt.Printf("Playing audio with %s %s\n", cmdName, strings.Join(cmdArgs, " "))
	cmd := exec.Command(cmdName, cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("Error playing audio: %v", err)
	}
}